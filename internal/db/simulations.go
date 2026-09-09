package db

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
)

// Simulation est une ligne de la piste d'audit.
type Simulation struct {
	ID           int64           `json:"id"`
	RequeteID    string          `json:"requete_id"`
	EmpreinteCle string          `json:"empreinte_cle,omitempty"`
	Adresse      string          `json:"adresse,omitempty"`
	Demande      json.RawMessage `json:"demande"`

	Teg                string `json:"teg"`
	CoutCredit         string `json:"cout_credit"`
	PremiereMensualite string `json:"premiere_mensualite"`

	Tem       *string `json:"tem"`
	Seuil     *string `json:"seuil_excessif"`
	Conforme  *bool   `json:"conforme"`
	Categorie *string `json:"categorie"`
	Semestre  *string `json:"semestre"`
	Arrete    *string `json:"arrete"`

	CreeLe string `json:"cree_le"`
}

// Simulations tient la piste d'audit.
type Simulations struct {
	pool *Pool
}

func NewSimulations(pool *Pool) *Simulations {
	return &Simulations{pool: pool}
}

// Enregistrer inscrit une simulation. L'appel est synchrone et son echec doit
// faire echouer la requete : une piste d'audit a trous n'en est pas une.
func (s *Simulations) Enregistrer(ctx context.Context, sim Simulation) (int64, error) {
	var adresse any
	if sim.Adresse != "" {
		// Une adresse illisible ne doit pas empecher l'inscription : mieux
		// vaut une ligne sans adresse que pas de ligne du tout.
		if _, err := netip.ParseAddr(sim.Adresse); err == nil {
			adresse = sim.Adresse
		}
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO simulations (
			requete_id, empreinte_cle, adresse, demande,
			teg, cout_credit, premiere_mensualite,
			tem, seuil, conforme, categorie, semestre, arrete)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id`,
		sim.RequeteID, sim.EmpreinteCle, adresse, sim.Demande,
		sim.Teg, sim.CoutCredit, sim.PremiereMensualite,
		sim.Tem, sim.Seuil, sim.Conforme,
		sim.Categorie, sim.Semestre, sim.Arrete).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inscription de la simulation : %w", err)
	}
	return id, nil
}

const colonnesSimulation = `id, requete_id, coalesce(empreinte_cle, ''),
	coalesce(host(adresse), ''), demande,
	teg::text, cout_credit::text, premiere_mensualite::text,
	tem::text, seuil::text, conforme, categorie, semestre, arrete,
	to_char(cree_le, 'YYYY-MM-DD"T"HH24:MI:SSOF')`

// Lister rend les simulations les plus recentes. nonConformes restreint aux
// prets juges excessifs, ce que demande un controle.
func (s *Simulations) Lister(ctx context.Context, limite int, nonConformes bool) ([]Simulation, error) {
	requete := `SELECT ` + colonnesSimulation + ` FROM simulations`
	if nonConformes {
		requete += ` WHERE conforme IS FALSE`
	}
	requete += ` ORDER BY cree_le DESC, id DESC LIMIT $1`

	lignes, err := s.pool.Query(ctx, requete, limite)
	if err != nil {
		return nil, fmt.Errorf("lecture des simulations : %w", err)
	}
	defer lignes.Close()

	var toutes []Simulation
	for lignes.Next() {
		var sim Simulation
		if err := lignes.Scan(&sim.ID, &sim.RequeteID, &sim.EmpreinteCle,
			&sim.Adresse, &sim.Demande, &sim.Teg, &sim.CoutCredit,
			&sim.PremiereMensualite, &sim.Tem, &sim.Seuil, &sim.Conforme,
			&sim.Categorie, &sim.Semestre, &sim.Arrete, &sim.CreeLe); err != nil {
			return nil, fmt.Errorf("lecture d'une simulation : %w", err)
		}
		toutes = append(toutes, sim)
	}
	if err := lignes.Err(); err != nil {
		return nil, fmt.Errorf("parcours des simulations : %w", err)
	}
	return toutes, nil
}
