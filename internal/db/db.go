// Package db ouvre la base PostgreSQL et applique ses migrations.
package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Pool est le type de connexion rendu par Ouvrir. L'alias evite aux appelants
// d'importer pgxpool pour une simple signature.
type Pool = pgxpool.Pool

// estAbsent distingue l'absence de ligne des autres erreurs de requete.
func estAbsent(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// verrouMigrations identifie le verrou consultatif pris pendant l'application
// des migrations. Plusieurs instances peuvent demarrer en meme temps ; sans
// lui, elles appliqueraient la meme migration deux fois.
const verrouMigrations int64 = 7314159265358979

// delaiOuverture borne la connexion initiale : mieux vaut un demarrage qui
// echoue vite qu'un service qui pend.
const delaiOuverture = 10 * time.Second

// Ouvrir etablit le pool et verifie que la base repond.
func Ouvrir(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, errors.New("DATABASE_URL est vide")
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL illisible : %w", err)
	}
	// Le service ne fait que des lectures courtes ; une poignee de connexions
	// suffit, et les fermer apres une demi-heure evite de garder ouvertes des
	// sessions que le serveur a peut-etre deja abandonnees.
	cfg.MaxConns = 8
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("ouverture du pool : %w", err)
	}

	ctxPing, annuler := context.WithTimeout(ctx, delaiOuverture)
	defer annuler()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("base injoignable : %w", err)
	}

	return pool, nil
}

// Migrer applique les migrations embarquees qui manquent, dans l'ordre de leur
// nom. Chacune est appliquee dans sa propre transaction, avec l'inscription de
// sa version : une migration echouee ne laisse rien derriere elle.
func Migrer(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquisition d'une connexion : %w", err)
	}
	defer conn.Release()

	// Le verrou est pris sur la connexion, pas sur la transaction : il couvre
	// toute la sequence et se relache a la liberation de la connexion.
	//
	// Il precede la creation de la table des migrations, et non l'inverse :
	// CREATE TABLE IF NOT EXISTS ne protege pas de la concurrence, deux
	// instances passant ensemble le test d'existence puis echouant l'une des
	// deux sur le catalogue. Le verrou ne porte que sur un entier, il n'a
	// besoin d'aucune table.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", verrouMigrations); err != nil {
		return fmt.Errorf("prise du verrou : %w", err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", verrouMigrations); err != nil {
			slog.Error("relachement du verrou de migration", "erreur", err)
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text        PRIMARY KEY,
			applique_le timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("table des migrations : %w", err)
	}

	appliquees, err := versionsAppliquees(ctx, conn)
	if err != nil {
		return err
	}

	fichiers, err := listerMigrations()
	if err != nil {
		return err
	}

	for _, nom := range fichiers {
		if appliquees[nom] {
			continue
		}
		sql, err := migrations.ReadFile("migrations/" + nom)
		if err != nil {
			return fmt.Errorf("lecture de %s : %w", nom, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("ouverture de transaction pour %s : %w", nom, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s : %w", nom, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", nom); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("inscription de %s : %w", nom, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("validation de %s : %w", nom, err)
		}

		slog.Info("migration appliquee", "version", nom)
	}

	return nil
}

func versionsAppliquees(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	lignes, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("lecture des migrations appliquees : %w", err)
	}
	defer lignes.Close()

	appliquees := make(map[string]bool)
	for lignes.Next() {
		var version string
		if err := lignes.Scan(&version); err != nil {
			return nil, fmt.Errorf("lecture d'une version : %w", err)
		}
		appliquees[version] = true
	}
	if err := lignes.Err(); err != nil {
		return nil, fmt.Errorf("parcours des migrations appliquees : %w", err)
	}
	return appliquees, nil
}

// listerMigrations rend les noms de fichiers tries. L'ordre lexicographique
// suffit tant que les migrations sont numerotees a largeur fixe.
func listerMigrations() ([]string, error) {
	entrees, err := migrations.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("lecture des migrations : %w", err)
	}
	noms := make([]string, 0, len(entrees))
	for _, e := range entrees {
		if !e.IsDir() {
			noms = append(noms, e.Name())
		}
	}
	sort.Strings(noms)
	return noms, nil
}
