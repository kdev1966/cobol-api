package db

import (
	"context"
	"os"
	"sync"
	"testing"
)

// urlBase rend l'adresse de la base de test, ou saute le test. Les tests qui
// ont besoin de PostgreSQL se sautent d'eux-memes en son absence ; la CI en
// fournit toujours une.
func urlBase(t *testing.T) string {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL absente : lancer un PostgreSQL de test")
	}
	return url
}

func ouvrir(t *testing.T) *Pool {
	t.Helper()
	pool, err := Ouvrir(context.Background(), urlBase(t))
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestOuvrirRefuseUneAdresseVide(t *testing.T) {
	if _, err := Ouvrir(context.Background(), ""); err == nil {
		t.Fatal("une adresse vide aurait du etre refusee")
	}
}

func TestOuvrirRefuseUneAdresseIllisible(t *testing.T) {
	if _, err := Ouvrir(context.Background(), "ceci n'est pas une url"); err == nil {
		t.Fatal("une adresse illisible aurait du etre refusee")
	}
}

func TestOuvrirRefuseUneBaseInjoignable(t *testing.T) {
	// Port ferme : la connexion doit echouer au demarrage, pas plus tard.
	_, err := Ouvrir(context.Background(),
		"postgres://postgres:x@127.0.0.1:1/absente?sslmode=disable&connect_timeout=2")
	if err == nil {
		t.Fatal("une base injoignable aurait du etre refusee")
	}
}

func TestMigrerEstIdempotent(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()

	if err := Migrer(ctx, pool); err != nil {
		t.Fatalf("premiere migration : %v", err)
	}

	var avant int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM taux_effectifs_moyens").Scan(&avant); err != nil {
		t.Fatalf("comptage : %v", err)
	}
	if avant != 8 {
		t.Fatalf("%d taux charges, attendu 8", avant)
	}

	// Rejouer ne doit ni echouer ni dupliquer.
	if err := Migrer(ctx, pool); err != nil {
		t.Fatalf("seconde migration : %v", err)
	}
	var apres int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM taux_effectifs_moyens").Scan(&apres); err != nil {
		t.Fatalf("comptage : %v", err)
	}
	if apres != avant {
		t.Errorf("%d taux apres rejeu, %d avant", apres, avant)
	}
}

// Plusieurs instances peuvent demarrer en meme temps. Le verrou consultatif
// doit serialiser l'application des migrations, sans quoi deux instances
// inseraient les memes lignes.
func TestMigrerSupporteDesDemarragesConcurrents(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()

	const instances = 6
	var attente sync.WaitGroup
	erreurs := make(chan error, instances)

	for i := 0; i < instances; i++ {
		attente.Add(1)
		go func() {
			defer attente.Done()
			if err := Migrer(ctx, pool); err != nil {
				erreurs <- err
			}
		}()
	}
	attente.Wait()
	close(erreurs)

	for err := range erreurs {
		t.Errorf("migration concurrente : %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM taux_effectifs_moyens").Scan(&n); err != nil {
		t.Fatalf("comptage : %v", err)
	}
	if n != 8 {
		t.Errorf("%d taux apres %d migrations concurrentes, attendu 8", n, instances)
	}
}

// Les taux charges doivent etre exactement ceux publies par l'arrete.
func TestLesTauxPubliesSontCharges(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	if err := Migrer(ctx, pool); err != nil {
		t.Fatalf("Migrer : %v", err)
	}

	publies := map[string]string{
		"leasing": "13.37", "decouverts": "12.29",
		"gestion_des_dettes": "11.78", "credits_consommation": "11.23",
		"credits_logement": "10.25", "credits_moyen_terme": "9.80",
		"credits_long_terme": "9.63", "credits_court_terme": "9.57",
	}

	for categorie, attendu := range publies {
		var tem string
		err := pool.QueryRow(ctx,
			"SELECT tem::text FROM taux_effectifs_moyens WHERE categorie = $1 AND semestre = $2",
			categorie, "2026S1").Scan(&tem)
		if err != nil {
			t.Errorf("%s : %v", categorie, err)
			continue
		}
		if tem != attendu {
			t.Errorf("%s : tem %s, publie %s", categorie, tem, attendu)
		}
	}
}

func TestListerMigrationsRendUnOrdreStable(t *testing.T) {
	noms, err := listerMigrations()
	if err != nil {
		t.Fatalf("listerMigrations : %v", err)
	}
	if len(noms) < 2 {
		t.Fatalf("%d migrations trouvees", len(noms))
	}
	for i := 1; i < len(noms); i++ {
		if noms[i-1] >= noms[i] {
			t.Errorf("ordre non croissant : %s puis %s", noms[i-1], noms[i])
		}
	}
}
