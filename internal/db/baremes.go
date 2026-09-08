package db

import (
	"context"
	"errors"
	"fmt"
)

// ErrCategorieInconnue signale une categorie absente du bareme.
var ErrCategorieInconnue = errors.New("categorie absente du bareme")

// TauxEffectif est un taux effectif moyen tel que publie par arrete.
//
// Tem est une chaine et non un flottant : la valeur traverse le service sous
// forme de texte, de la colonne numeric jusqu'au programme COBOL, sans jamais
// passer par une representation binaire.
type TauxEffectif struct {
	Categorie string `json:"categorie"`
	Semestre  string `json:"semestre"`
	Tem       string `json:"tem"`
	Arrete    string `json:"arrete"`
	PublieLe  string `json:"publie_le"`
}

// Baremes lit les taux effectifs moyens publies.
type Baremes struct {
	pool *Pool
}

// NewBaremes rend un accesseur aux baremes.
func NewBaremes(pool *Pool) *Baremes {
	return &Baremes{pool: pool}
}

const colonnes = `categorie, semestre, tem::text, arrete, to_char(publie_le, 'YYYY-MM-DD')`

// TauxEffectifMoyen rend le taux en vigueur pour une categorie, c'est-a-dire
// celui du semestre le plus recent. Le format du semestre, 2026S1, rend
// l'ordre lexicographique et l'ordre chronologique identiques.
func (b *Baremes) TauxEffectifMoyen(ctx context.Context, categorie string) (TauxEffectif, error) {
	var t TauxEffectif
	err := b.pool.QueryRow(ctx, `
		SELECT `+colonnes+`
		FROM taux_effectifs_moyens
		WHERE categorie = $1
		ORDER BY semestre DESC
		LIMIT 1`, categorie).
		Scan(&t.Categorie, &t.Semestre, &t.Tem, &t.Arrete, &t.PublieLe)
	if err != nil {
		if estAbsent(err) {
			return TauxEffectif{}, fmt.Errorf("%w : %s", ErrCategorieInconnue, categorie)
		}
		return TauxEffectif{}, fmt.Errorf("lecture du bareme : %w", err)
	}
	return t, nil
}

// Lister rend le bareme en vigueur, une ligne par categorie, du taux le plus
// eleve au plus faible.
func (b *Baremes) Lister(ctx context.Context) ([]TauxEffectif, error) {
	lignes, err := b.pool.Query(ctx, `
		SELECT DISTINCT ON (categorie) `+colonnes+`
		FROM taux_effectifs_moyens
		ORDER BY categorie, semestre DESC`)
	if err != nil {
		return nil, fmt.Errorf("lecture du bareme : %w", err)
	}
	defer lignes.Close()

	var tous []TauxEffectif
	for lignes.Next() {
		var t TauxEffectif
		if err := lignes.Scan(&t.Categorie, &t.Semestre, &t.Tem, &t.Arrete, &t.PublieLe); err != nil {
			return nil, fmt.Errorf("lecture d'une ligne de bareme : %w", err)
		}
		tous = append(tous, t)
	}
	if err := lignes.Err(); err != nil {
		return nil, fmt.Errorf("parcours du bareme : %w", err)
	}
	return tous, nil
}

// Categories rend les libelles connus, pour les messages d'erreur et la
// documentation.
func (b *Baremes) Categories(ctx context.Context) ([]string, error) {
	tous, err := b.Lister(ctx)
	if err != nil {
		return nil, err
	}
	noms := make([]string, 0, len(tous))
	for _, t := range tous {
		noms = append(noms, t.Categorie)
	}
	return noms, nil
}
