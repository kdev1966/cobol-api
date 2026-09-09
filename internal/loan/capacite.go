package loan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Format de l'enregistrement rendu par le programme de capacite.
const (
	tagCapacite  = 'C'
	longCapacite = 33
)

// DemandeCapacite est une demande de capacite d'emprunt validee.
type DemandeCapacite struct {
	BudgetMillimes       int64 `json:"-"`
	TauxMillioniemes     int64 `json:"-"`
	TauxAssuranceMillion int64 `json:"-"`
	CodeMethode          byte  `json:"-"`
	CodeAssiette         byte  `json:"-"`

	Mois          int    `json:"mois"`
	Mensualite    string `json:"mensualite_max"`
	Taux          string `json:"taux"`
	Methode       string `json:"methode"`
	TauxAssurance string `json:"taux_assurance"`
	AssietteAssur string `json:"assiette_assurance"`
}

// Capacite est le resultat du calcul inverse.
type Capacite struct {
	// Capital est le plus grand montant dont la premiere mensualite tient
	// dans le budget, arrondis compris.
	Capital json.Number `json:"capital"`
	// Mensualite est celle qu'il produit reellement, toujours inferieure ou
	// egale au budget demande.
	Mensualite json.Number `json:"mensualite"`
	// MargeMillimes est la part du budget non employee.
	MargeMillimes int `json:"marge_millimes"`
}

// ligneCapacite rend les 36 caracteres attendus : mensualite 9(11)V999, taux
// 9(2)V9(6), duree 9(4), taux d'assurance 9(2)V9(6), puis les lettres de la
// methode et de l'assiette.
func ligneCapacite(d DemandeCapacite) string {
	return fmt.Sprintf("%014d%08d%04d%08d%c%c\n",
		d.BudgetMillimes, d.TauxMillioniemes, d.Mois,
		d.TauxAssuranceMillion, d.CodeMethode, d.CodeAssiette)
}

// Capaciter rend le capital maximal empruntable pour le budget demande.
func (m *Moteur) Capaciter(ctx context.Context, d DemandeCapacite) (*Capacite, error) {
	if m.CheminCapacite == "" {
		return nil, fmt.Errorf("%w : programme de capacite non configure", ErrProgramme)
	}

	ctx, annuler := context.WithTimeout(ctx, m.Delai)
	defer annuler()

	cmd := exec.CommandContext(ctx, m.CheminCapacite)
	cmd.WaitDelay = delaiAvantArret
	cmd.Stdin = strings.NewReader(ligneCapacite(d))

	var sortie, erreurs bytes.Buffer
	cmd.Stdout = &sortie
	cmd.Stderr = &erreurs

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w apres %s", ErrDelaiDepasse, m.Delai)
		}
		detail := strings.TrimSpace(erreurs.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%w : %s", ErrProgramme, detail)
	}

	return lireCapacite(sortie.String())
}

func lireCapacite(sortie string) (*Capacite, error) {
	for _, brute := range strings.Split(sortie, "\n") {
		ligne := strings.TrimRight(brute, " \r")
		if len(ligne) != longCapacite || ligne[0] != tagCapacite {
			continue
		}

		capital, err := montant(ligne[1:15])
		if err != nil {
			return nil, fmt.Errorf("capacite illisible : %w", err)
		}
		mensualite, err := montant(ligne[15:29])
		if err != nil {
			return nil, fmt.Errorf("capacite illisible : %w", err)
		}
		marge, err := entier(ligne[29:33])
		if err != nil {
			return nil, fmt.Errorf("capacite illisible : %w", err)
		}
		return &Capacite{Capital: capital, Mensualite: mensualite, MargeMillimes: marge}, nil
	}
	return nil, fmt.Errorf("%w : aucune capacite dans la sortie", ErrProgramme)
}

// ParseDemandeCapacite valide les parametres du calcul inverse.
func ParseDemandeCapacite(p Parametres) (DemandeCapacite, error) {
	budget, err := decimalVersEntier(p.Mensualite, DecimalesMonnaie)
	if err != nil {
		return DemandeCapacite{}, &ErreurValidation{"mensualite", err.Error()}
	}
	if budget < CapitalMin || budget > CapitalMax {
		return DemandeCapacite{}, &ErreurValidation{"mensualite",
			"doit etre comprise entre 0.001 et 99999999999.999 dinars"}
	}

	// Le taux, la duree, la methode et l'assurance obeissent aux memes regles
	// que pour l'echeancier : on reutilise la validation, en donnant un
	// capital fictif que le calcul inverse remplacera.
	commun, err := ParseDemande(Parametres{
		Capital: "1", Taux: p.Taux, Mois: p.Mois, Methode: p.Methode,
		TauxAssurance: p.TauxAssurance, Assiette: p.Assiette,
	})
	if err != nil {
		return DemandeCapacite{}, err
	}

	return DemandeCapacite{
		BudgetMillimes:       budget,
		TauxMillioniemes:     commun.TauxMillioniemes,
		TauxAssuranceMillion: commun.TauxAssuranceMillion,
		CodeMethode:          commun.CodeMethode,
		CodeAssiette:         commun.CodeAssiette,
		Mois:                 commun.Mois,
		Mensualite:           formaterEchelle(budget, DecimalesMonnaie),
		Taux:                 commun.Taux,
		Methode:              commun.Methode,
		TauxAssurance:        commun.TauxAssurance,
		AssietteAssur:        commun.AssietteAssur,
	}, nil
}
