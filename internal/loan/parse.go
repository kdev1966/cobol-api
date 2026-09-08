package loan

import (
	"fmt"
	"strconv"
	"strings"
)

// ErreurValidation porte un message destine au client.
type ErreurValidation struct {
	Champ   string
	Message string
}

func (e *ErreurValidation) Error() string {
	return fmt.Sprintf("%s : %s", e.Champ, e.Message)
}

// decimalVersEntier convertit une ecriture decimale en entier a l'echelle
// voulue, sans passer par un flottant. "3.45" avec 6 decimales donne 3450000.
func decimalVersEntier(texte string, decimales int) (int64, error) {
	texte = strings.TrimSpace(texte)
	if texte == "" {
		return 0, fmt.Errorf("valeur vide")
	}
	if strings.HasPrefix(texte, "+") || strings.HasPrefix(texte, "-") {
		return 0, fmt.Errorf("le signe n'est pas accepte")
	}

	entiere, fraction, _ := strings.Cut(texte, ".")
	if entiere == "" {
		entiere = "0"
	}
	if len(fraction) > decimales {
		return 0, fmt.Errorf("au plus %d decimales", decimales)
	}
	fraction += strings.Repeat("0", decimales-len(fraction))

	for _, partie := range []string{entiere, fraction} {
		for _, r := range partie {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("chiffres et point decimal uniquement")
			}
		}
	}

	valeur, err := strconv.ParseInt(entiere+fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("valeur trop grande")
	}
	return valeur, nil
}

// MethodeParDefaut est retenue quand le parametre est absent.
const MethodeParDefaut = "annuite_constante"

// codesMethode associe chaque libelle accepte a la lettre attendue par le
// programme COBOL. Les lettres seules sont acceptees comme alias.
var codesMethode = map[string]byte{
	"annuite_constante": 'A',
	"capital_constant":  'C',
	"in_fine":           'I',
	"A":                 'A',
	"C":                 'C',
	"I":                 'I',
}

// libellesMethode rend la forme canonique renvoyee dans la reponse.
var libellesMethode = map[byte]string{
	'A': "annuite_constante",
	'C': "capital_constant",
	'I': "in_fine",
}

// MethodesAcceptees liste les libelles canoniques, pour la documentation et
// les messages d'erreur.
func MethodesAcceptees() []string {
	return []string{"annuite_constante", "capital_constant", "in_fine"}
}

// ParseDemande valide les parametres et rend une demande prete a calculer.
// Les bornes protegent a la fois les PIC du programme COBOL et la taille de
// la reponse.
func ParseDemande(capital, taux, mois, methode string) (Demande, error) {
	centimes, err := decimalVersEntier(capital, 2)
	if err != nil {
		return Demande{}, &ErreurValidation{"capital", err.Error()}
	}
	if centimes < CapitalMin || centimes > CapitalMax {
		return Demande{}, &ErreurValidation{"capital",
			"doit etre compris entre 0.01 et 99999999999.99"}
	}

	millioniemes, err := decimalVersEntier(taux, 6)
	if err != nil {
		return Demande{}, &ErreurValidation{"taux", err.Error()}
	}
	if millioniemes > TauxMax {
		return Demande{}, &ErreurValidation{"taux",
			"doit etre compris entre 0 et 99.999999"}
	}

	n, err := strconv.Atoi(strings.TrimSpace(mois))
	if err != nil {
		return Demande{}, &ErreurValidation{"mois", "doit etre un entier"}
	}
	if n < MoisMin || n > MoisMax {
		return Demande{}, &ErreurValidation{"mois",
			fmt.Sprintf("doit etre compris entre %d et %d", MoisMin, MoisMax)}
	}

	methode = strings.TrimSpace(methode)
	if methode == "" {
		methode = MethodeParDefaut
	}
	code, connue := codesMethode[methode]
	if !connue {
		return Demande{}, &ErreurValidation{"methode",
			"doit valoir " + strings.Join(MethodesAcceptees(), ", ")}
	}

	return Demande{
		CapitalCentimes:  centimes,
		TauxMillioniemes: millioniemes,
		CodeMethode:      code,
		Mois:             n,
		Capital:          formaterEchelle(centimes, 2),
		Taux:             formaterEchelle(millioniemes, 6),
		Methode:          libellesMethode[code],
	}, nil
}

// formaterEchelle rend la forme decimale d'un entier a l'echelle donnee.
func formaterEchelle(valeur int64, decimales int) string {
	chiffres := strconv.FormatInt(valeur, 10)
	if len(chiffres) <= decimales {
		chiffres = strings.Repeat("0", decimales-len(chiffres)+1) + chiffres
	}
	coupe := len(chiffres) - decimales
	return chiffres[:coupe] + "." + chiffres[coupe:]
}
