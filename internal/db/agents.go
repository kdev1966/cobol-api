package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// ErrIdentifiantsRefuses couvre indistinctement l'identifiant inconnu, le mot
// de passe faux et le compte desactive. Les distinguer dirait a un attaquant
// quels identifiants existent.
var ErrIdentifiantsRefuses = errors.New("identifiants refuses")

// ErrIdentifiantPris signale un identifiant deja attribue.
var ErrIdentifiantPris = errors.New("identifiant deja attribue")

// ErrSessionInconnue signale un jeton absent, expire ou revoque.
var ErrSessionInconnue = errors.New("session inconnue ou expiree")

// DureeSession borne une session ouverte. Huit heures couvrent une journee de
// travail sans laisser une session ouverte indefiniment sur un poste partage.
const DureeSession = 8 * time.Hour

// octetsJeton donne au jeton de session 256 bits d'entropie.
const octetsJeton = 32

// Agent est un compte d'agent de credit. Le mot de passe n'y figure jamais.
type Agent struct {
	ID                int64   `json:"id"`
	Identifiant       string  `json:"identifiant"`
	Nom               string  `json:"nom"`
	Agence            string  `json:"agence"`
	Actif             bool    `json:"actif"`
	CreeLe            string  `json:"cree_le"`
	DerniereConnexion *string `json:"derniere_connexion"`
}

// Agents tient les comptes et leurs sessions.
type Agents struct {
	pool *Pool
}

func NewAgents(pool *Pool) *Agents {
	return &Agents{pool: pool}
}

// Creer inscrit un compte. Le mot de passe est hache avant d'atteindre la
// base : il ne transite jamais en clair dans une requete SQL.
func (a *Agents) Creer(ctx context.Context, identifiant, motDePasse, nom, agence string) (*Agent, error) {
	empreinte, err := HacherMotDePasse(motDePasse)
	if err != nil {
		return nil, err
	}

	var ag Agent
	var creeLe time.Time
	err = a.pool.QueryRow(ctx, `
		INSERT INTO agents (identifiant, mot_de_passe, nom, agence)
		VALUES ($1, $2, $3, $4)
		RETURNING id, identifiant, nom, agence, actif, cree_le`,
		strings.TrimSpace(identifiant), empreinte, nom, agence,
	).Scan(&ag.ID, &ag.Identifiant, &ag.Nom, &ag.Agence, &ag.Actif, &creeLe)
	if err != nil {
		if strings.Contains(err.Error(), "agents_identifiant") {
			return nil, ErrIdentifiantPris
		}
		return nil, fmt.Errorf("creation de l'agent : %w", err)
	}
	ag.CreeLe = creeLe.UTC().Format(time.RFC3339)
	return &ag, nil
}

// Authentifier verifie un couple identifiant / mot de passe et ouvre une
// session. Le jeton rendu n'est pas conserve : seule son empreinte l'est.
func (a *Agents) Authentifier(ctx context.Context,
	identifiant, motDePasse, adresse, navigateur string) (string, *Agent, error) {

	var ag Agent
	var empreinte string
	var creeLe time.Time
	err := a.pool.QueryRow(ctx, `
		SELECT id, identifiant, mot_de_passe, nom, agence, actif, cree_le
		FROM agents WHERE lower(identifiant) = lower($1)`,
		strings.TrimSpace(identifiant),
	).Scan(&ag.ID, &ag.Identifiant, &empreinte, &ag.Nom, &ag.Agence,
		&ag.Actif, &creeLe)

	if err != nil {
		if estAbsent(err) {
			// Le mot de passe est tout de meme verifie contre une empreinte
			// factice : sans cela, la reponse serait plus rapide pour un
			// identifiant inconnu que pour un mot de passe faux, ce qui
			// permettrait d'enumerer les comptes au chronometre.
			_, _ = VerifierMotDePasse(motDePasse, empreinteFactice)
			return "", nil, ErrIdentifiantsRefuses
		}
		return "", nil, fmt.Errorf("lecture de l'agent : %w", err)
	}

	bon, err := VerifierMotDePasse(motDePasse, empreinte)
	if err != nil {
		return "", nil, fmt.Errorf("verification du mot de passe : %w", err)
	}
	if !bon || !ag.Actif {
		return "", nil, ErrIdentifiantsRefuses
	}
	ag.CreeLe = creeLe.UTC().Format(time.RFC3339)

	jeton, err := nouveauJeton()
	if err != nil {
		return "", nil, err
	}
	somme := sha256.Sum256([]byte(jeton))

	var ip any
	if _, err := netip.ParseAddr(adresse); err == nil {
		ip = adresse
	}

	_, err = a.pool.Exec(ctx, `
		INSERT INTO sessions (empreinte, agent_id, expire_le, adresse,
			agent_utilisateur)
		VALUES ($1, $2, now() + $3::interval, $4, $5)`,
		somme[:], ag.ID, DureeSession.String(), ip, navigateur)
	if err != nil {
		return "", nil, fmt.Errorf("ouverture de la session : %w", err)
	}

	if _, err := a.pool.Exec(ctx,
		`UPDATE agents SET derniere_connexion = now() WHERE id = $1`,
		ag.ID); err != nil {
		return "", nil, fmt.Errorf("horodatage de la connexion : %w", err)
	}

	return jeton, &ag, nil
}

// empreinteFactice sert uniquement a egaliser le temps de reponse quand
// l'identifiant est inconnu. Elle ne correspond a aucun mot de passe utile.
var empreinteFactice = func() string {
	e, err := HacherMotDePasse("mot de passe sans usage")
	if err != nil {
		// Au demarrage seulement, et sans acces reseau : un echec ici
		// signalerait un generateur aleatoire defaillant.
		panic("argon2id indisponible : " + err.Error())
	}
	return e
}()

// AgentDeSession rend le titulaire d'un jeton, ou ErrSessionInconnue.
func (a *Agents) AgentDeSession(ctx context.Context, jeton string) (*Agent, error) {
	somme := sha256.Sum256([]byte(jeton))

	var ag Agent
	var creeLe time.Time
	err := a.pool.QueryRow(ctx, `
		SELECT a.id, a.identifiant, a.nom, a.agence, a.actif, a.cree_le
		FROM sessions s JOIN agents a ON a.id = s.agent_id
		WHERE s.empreinte = $1 AND s.expire_le > now() AND a.actif`,
		somme[:],
	).Scan(&ag.ID, &ag.Identifiant, &ag.Nom, &ag.Agence, &ag.Actif, &creeLe)

	if err != nil {
		if estAbsent(err) {
			return nil, ErrSessionInconnue
		}
		return nil, fmt.Errorf("lecture de la session : %w", err)
	}
	ag.CreeLe = creeLe.UTC().Format(time.RFC3339)
	return &ag, nil
}

// Deconnecter revoque une session. Revoquer un jeton absent n'est pas une
// erreur : le resultat voulu est atteint.
func (a *Agents) Deconnecter(ctx context.Context, jeton string) error {
	somme := sha256.Sum256([]byte(jeton))
	if _, err := a.pool.Exec(ctx,
		`DELETE FROM sessions WHERE empreinte = $1`, somme[:]); err != nil {
		return fmt.Errorf("revocation de la session : %w", err)
	}
	return nil
}

// PurgerLesSessions supprime les sessions echues et rend leur nombre.
func (a *Agents) PurgerLesSessions(ctx context.Context) (int64, error) {
	etiquette, err := a.pool.Exec(ctx,
		`DELETE FROM sessions WHERE expire_le <= now()`)
	if err != nil {
		return 0, fmt.Errorf("purge des sessions : %w", err)
	}
	return etiquette.RowsAffected(), nil
}

// nouveauJeton tire un jeton de session imprevisible.
func nouveauJeton() (string, error) {
	brut := make([]byte, octetsJeton)
	if _, err := rand.Read(brut); err != nil {
		return "", fmt.Errorf("tirage du jeton : %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(brut), nil
}
