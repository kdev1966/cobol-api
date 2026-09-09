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
// voulue, sans passer par un flottant. "3.45" avec 6 decimales donne 3450000,
// et un montant en dinars devient un entier de millimes.
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

// AssietteParDefaut : sans assurance declaree, il n'y en a pas.
const AssietteParDefaut = "aucune"

// codesAssiette associe chaque libelle d'assiette a la lettre attendue par le
// programme COBOL.
var codesAssiette = map[string]byte{
	"aucune":             'N',
	"capital_initial":    'I',
	"capital_restant_du": 'R',
	"N":                  'N',
	"I":                  'I',
	"R":                  'R',
}

var libellesAssiette = map[byte]string{
	'N': "aucune",
	'I': "capital_initial",
	'R': "capital_restant_du",
}

// AssiettesAcceptees liste les libelles canoniques.
func AssiettesAcceptees() []string {
	return []string{"aucune", "capital_initial", "capital_restant_du"}
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

// ModeParDefaut : sans precision, un remboursement anticipe solde la totalite
// du capital restant du. C'est la forme la plus courante, et celle que le
// service offrait avant que le remboursement partiel n'existe.
const ModeParDefaut = "total"

// codesMode associe chaque suite donnee a un remboursement anticipe a la
// lettre attendue par le programme COBOL.
var codesMode = map[string]byte{
	"total":            'T',
	"duree_reduite":    'D',
	"echeance_reduite": 'M',
	"T":                'T',
	"D":                'D',
	"M":                'M',
}

var libellesMode = map[byte]string{
	'T': "total",
	'D': "duree_reduite",
	'M': "echeance_reduite",
}

// ModesAcceptees liste les libelles canoniques.
func ModesAcceptes() []string {
	return []string{"total", "duree_reduite", "echeance_reduite"}
}

// ParseDemande valide les parametres et rend une demande prete a calculer.
// Les bornes protegent a la fois les PIC du programme COBOL et la taille de
// la reponse.
// Parametres regroupe la saisie brute, telle qu'elle arrive de la requete.
type Parametres struct {
	Capital       string
	Taux          string
	Mois          string
	Methode       string
	FraisDossier  string
	FraisGarantie string
	TauxAssurance string
	Assiette      string
	// Mensualite est le budget mensuel du calcul inverse.
	Mensualite string
	// Differe est le nombre d'echeances en franchise partielle.
	Differe string
	// MoisAnticipe est l'echeance a laquelle simuler un remboursement
	// anticipe, et Indemnite le taux applique au capital rembourse.
	MoisAnticipe string
	Indemnite    string
	// MontantAnticipe est le capital rembourse par anticipation, et Mode la
	// suite donnee a l'operation : solder la totalite, raccourcir la duree,
	// ou alleger l'echeance.
	MontantAnticipe string
	Mode            string
	// Categorie designe la categorie de concours dont le taux effectif moyen
	// sera lu dans le bareme.
	Categorie string
	// Tem est le taux effectif moyen publie pour la categorie de concours.
	// Vide ou absent, aucune verification du taux excessif n'est demandee.
	Tem string
}

// montantOuZero convertit un montant facultatif : vide vaut zero.
func montantOuZero(champ, texte string, max int64) (int64, error) {
	if strings.TrimSpace(texte) == "" {
		return 0, nil
	}
	valeur, err := decimalVersEntier(texte, DecimalesMonnaie)
	if err != nil {
		return 0, &ErreurValidation{champ, err.Error()}
	}
	if valeur > max {
		return 0, &ErreurValidation{champ, "montant trop grand"}
	}
	return valeur, nil
}

func ParseDemande(p Parametres) (Demande, error) {
	capital, taux, mois, methode := p.Capital, p.Taux, p.Mois, p.Methode
	millimes, err := decimalVersEntier(capital, DecimalesMonnaie)
	if err != nil {
		return Demande{}, &ErreurValidation{"capital", err.Error()}
	}
	if millimes < CapitalMin || millimes > CapitalMax {
		return Demande{}, &ErreurValidation{"capital",
			"doit etre compris entre 0.001 et 99999999999.999 dinars"}
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

	fraisDossier, err := montantOuZero("frais_dossier", p.FraisDossier, FraisMax)
	if err != nil {
		return Demande{}, err
	}
	fraisGarantie, err := montantOuZero("frais_garantie", p.FraisGarantie, FraisMax)
	if err != nil {
		return Demande{}, err
	}
	// Les frais sont preleves sur ce que l'emprunteur percoit : ils ne
	// peuvent pas absorber le capital.
	if fraisDossier+fraisGarantie >= millimes {
		return Demande{}, &ErreurValidation{"frais_dossier",
			"les frais ne peuvent pas atteindre le capital emprunte"}
	}

	var tauxAssurance int64
	if brut := strings.TrimSpace(p.TauxAssurance); brut != "" {
		tauxAssurance, err = decimalVersEntier(brut, 6)
		if err != nil {
			return Demande{}, &ErreurValidation{"taux_assurance", err.Error()}
		}
		if tauxAssurance > TauxMax {
			return Demande{}, &ErreurValidation{"taux_assurance",
				"doit etre compris entre 0 et 99.999999"}
		}
	}

	assiette := strings.TrimSpace(p.Assiette)
	if assiette == "" {
		// Un taux d'assurance sans assiette porte sur le capital initial,
		// convention la plus repandue ; sans taux, il n'y a pas d'assurance.
		if tauxAssurance > 0 {
			assiette = "capital_initial"
		} else {
			assiette = AssietteParDefaut
		}
	}
	codeAssiette, connue := codesAssiette[assiette]
	if !connue {
		return Demande{}, &ErreurValidation{"assiette_assurance",
			"doit valoir " + strings.Join(AssiettesAcceptees(), ", ")}
	}

	var tem int64
	var temAffiche *string
	if brut := strings.TrimSpace(p.Tem); brut != "" {
		tem, err = decimalVersEntier(brut, 2)
		if err != nil {
			return Demande{}, &ErreurValidation{"tem", err.Error()}
		}
		// Le champ COBOL est un PIC 9(2)V99, et les taux publies par arrete
		// ont deux decimales.
		if tem > 9999 {
			return Demande{}, &ErreurValidation{"tem",
				"doit etre compris entre 0 et 99.99"}
		}
		if tem == 0 {
			return Demande{}, &ErreurValidation{"tem",
				"un taux effectif moyen nul n'a pas de sens ; omettre le parametre pour ne pas verifier"}
		}
		affiche := formaterEchelle(tem, 2)
		temAffiche = &affiche
	}

	var differe int
	if brut := strings.TrimSpace(p.Differe); brut != "" {
		differe, err = strconv.Atoi(brut)
		if err != nil || differe < 0 || differe > DiffereMax {
			return Demande{}, &ErreurValidation{"differe",
				fmt.Sprintf("doit etre un entier entre 0 et %d", DiffereMax)}
		}
		// Il doit rester au moins une echeance pour amortir le capital.
		if differe >= n {
			return Demande{}, &ErreurValidation{"differe",
				"doit laisser au moins une echeance amortissante"}
		}
	}

	// Le mode se lit meme sans remboursement anticipe : le preciser seul est
	// une erreur de saisie qu'il vaut mieux signaler que taire.
	mode := strings.TrimSpace(p.Mode)
	if mode == "" {
		mode = ModeParDefaut
	}
	codeMode, connu := codesMode[mode]
	if !connu {
		return Demande{}, &ErreurValidation{"mode_remboursement_anticipe",
			"doit valoir " + strings.Join(ModesAcceptes(), ", ")}
	}

	var moisAnticipe int
	var indemnite int64
	var montantAnticipe int64
	var indemniteAffichee, montantAffiche, modeAffiche *string
	if brut := strings.TrimSpace(p.MoisAnticipe); brut != "" {
		moisAnticipe, err = strconv.Atoi(brut)
		if err != nil || moisAnticipe < 1 || moisAnticipe > n {
			return Demande{}, &ErreurValidation{"mois_remboursement_anticipe",
				fmt.Sprintf("doit etre un entier entre 1 et %d", n)}
		}

		// L'indemnite n'est pas encadree par la loi tunisienne mais par le
		// contrat : son taux est un parametre, sans plafond impose ici.
		if brutIndem := strings.TrimSpace(p.Indemnite); brutIndem != "" {
			indemnite, err = decimalVersEntier(brutIndem, 4)
			if err != nil {
				return Demande{}, &ErreurValidation{"indemnite", err.Error()}
			}
			if indemnite > 999999 {
				return Demande{}, &ErreurValidation{"indemnite",
					"doit etre comprise entre 0 et 99.9999"}
			}
		}
		affiche := formaterEchelle(indemnite, 4)
		indemniteAffichee = &affiche

		if codeMode == 'T' {
			// Solder la totalite ne laisse rien a rembourser partiellement.
			if strings.TrimSpace(p.MontantAnticipe) != "" {
				return Demande{}, &ErreurValidation{"montant_remboursement_anticipe",
					"sans objet quand le mode est total"}
			}
		} else {
			montantAnticipe, err = decimalVersEntier(
				strings.TrimSpace(p.MontantAnticipe), DecimalesMonnaie)
			if err != nil {
				return Demande{}, &ErreurValidation{"montant_remboursement_anticipe",
					err.Error()}
			}
			if montantAnticipe < 1 || montantAnticipe > CapitalMax {
				return Demande{}, &ErreurValidation{"montant_remboursement_anticipe",
					fmt.Sprintf("doit etre compris entre %s et %s dinars",
						formaterEchelle(1, DecimalesMonnaie),
						formaterEchelle(CapitalMax, DecimalesMonnaie))}
			}
			// Il doit rester une echeance apres l'operation pour que
			// raccourcir la duree ou alleger l'echeance ait un sens.
			if moisAnticipe >= n {
				return Demande{}, &ErreurValidation{"mois_remboursement_anticipe",
					fmt.Sprintf("doit etre un entier entre 1 et %d "+
						"pour un remboursement partiel", n-1)}
			}
			// Sans amortissement avant le terme, il n'y a pas de duree a
			// raccourcir : le capital est du en une fois, a la fin.
			if codeMode == 'D' && code == 'I' {
				return Demande{}, &ErreurValidation{"mode_remboursement_anticipe",
					"duree_reduite est sans objet avec la methode in_fine"}
			}
			m := formaterEchelle(montantAnticipe, DecimalesMonnaie)
			montantAffiche = &m
		}
		l := libellesMode[codeMode]
		modeAffiche = &l
	} else {
		if strings.TrimSpace(p.Indemnite) != "" {
			return Demande{}, &ErreurValidation{"indemnite",
				"sans objet sans mois_remboursement_anticipe"}
		}
		if strings.TrimSpace(p.MontantAnticipe) != "" {
			return Demande{}, &ErreurValidation{"montant_remboursement_anticipe",
				"sans objet sans mois_remboursement_anticipe"}
		}
		if strings.TrimSpace(p.Mode) != "" {
			return Demande{}, &ErreurValidation{"mode_remboursement_anticipe",
				"sans objet sans mois_remboursement_anticipe"}
		}
		// Sans remboursement anticipe, le programme COBOL attend la lettre
		// du mode par defaut.
		codeMode = 'T'
	}

	return Demande{
		CapitalMillimes:          millimes,
		TauxMillioniemes:         millioniemes,
		CodeMethode:              code,
		CodeAssiette:             codeAssiette,
		FraisDossierMillimes:     fraisDossier,
		FraisGarantieMillimes:    fraisGarantie,
		TauxAssuranceMillion:     tauxAssurance,
		Mois:                     n,
		Capital:                  formaterEchelle(millimes, DecimalesMonnaie),
		Taux:                     formaterEchelle(millioniemes, 6),
		Methode:                  libellesMethode[code],
		FraisDossier:             formaterEchelle(fraisDossier, DecimalesMonnaie),
		FraisGarantie:            formaterEchelle(fraisGarantie, DecimalesMonnaie),
		TauxAssurance:            formaterEchelle(tauxAssurance, 6),
		AssietteAssur:            libellesAssiette[codeAssiette],
		DiffereMois:              differe,
		MoisAnticipe:             moisAnticipe,
		TauxIndemniteDixMillieme: indemnite,
		TauxIndemnite:            indemniteAffichee,
		MontantAnticipeMillimes:  montantAnticipe,
		CodeModeAnticipe:         codeMode,
		MontantAnticipe:          montantAffiche,
		ModeAnticipe:             modeAffiche,
		TemCentiemes:             tem,
		Tem:                      temAffiche,
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
