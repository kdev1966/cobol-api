package loan

import (
	"strings"
	"testing"
)

func TestDecimalVersEntierNePasseParUnFlottant(t *testing.T) {
	cas := []struct {
		texte     string
		decimales int
		attendu   int64
	}{
		{"250000.00", 2, 25000000},
		{"3.45", 6, 3450000},
		{"0.07", 2, 7},
		{"1234.5", 2, 123450},
		{"99999999999.99", 2, 9999999999999},
		{".5", 2, 50},
		{"7", 6, 7000000},
		// 0.07 n'est pas representable en binaire ; la conversion doit
		// rester exacte quoi qu'il arrive.
		{"0.070000", 6, 70000},
	}

	for _, c := range cas {
		got, err := decimalVersEntier(c.texte, c.decimales)
		if err != nil {
			t.Errorf("decimalVersEntier(%q, %d) : %v", c.texte, c.decimales, err)
			continue
		}
		if got != c.attendu {
			t.Errorf("decimalVersEntier(%q, %d) = %d, attendu %d",
				c.texte, c.decimales, got, c.attendu)
		}
	}
}

func TestDecimalVersEntierRefuseCeQuiNEnEstPas(t *testing.T) {
	cas := []struct {
		texte     string
		decimales int
	}{
		{"", 2}, {"abc", 2}, {"-5", 2}, {"+5", 2},
		{"1.234", 2}, // trop de decimales
		{"1e5", 2}, {"1 000", 2}, {"1,5", 2},
	}

	for _, c := range cas {
		if _, err := decimalVersEntier(c.texte, c.decimales); err == nil {
			t.Errorf("decimalVersEntier(%q, %d) aurait du echouer", c.texte, c.decimales)
		}
	}
}

func TestParseDemandeAccepteLesBornes(t *testing.T) {
	cas := []struct{ capital, taux, mois string }{
		{"0.01", "0", "1"},
		{"99999999999.99", "99.999999", "600"},
		{"250000.00", "3.45", "240"},
	}

	for _, c := range cas {
		if _, err := ParseDemande(Parametres{Capital: c.capital, Taux: c.taux, Mois: c.mois, Methode: ""}); err != nil {
			t.Errorf("ParseDemande(%q,%q,%q) : %v", c.capital, c.taux, c.mois, err)
		}
	}
}

func TestParseDemandeRefuseHorsBornes(t *testing.T) {
	cas := []struct {
		nom                 string
		capital, taux, mois string
		champ               string
	}{
		{"capital nul", "0", "3.45", "240", "capital"},
		{"capital trop grand", "100000000000.00", "3.45", "240", "capital"},
		{"capital non numerique", "abc", "3.45", "240", "capital"},
		{"taux trop grand", "1000.00", "100", "240", "taux"},
		{"mois nul", "1000.00", "3.45", "0", "mois"},
		{"mois trop grand", "1000.00", "3.45", "601", "mois"},
		{"mois non entier", "1000.00", "3.45", "12.5", "mois"},
		{"mois negatif", "1000.00", "3.45", "-12", "mois"},
	}

	for _, c := range cas {
		_, err := ParseDemande(Parametres{Capital: c.capital, Taux: c.taux, Mois: c.mois, Methode: ""})
		if err == nil {
			t.Errorf("%s : aurait du echouer", c.nom)
			continue
		}
		invalide, ok := err.(*ErreurValidation)
		if !ok {
			t.Errorf("%s : type d'erreur %T inattendu", c.nom, err)
			continue
		}
		if invalide.Champ != c.champ {
			t.Errorf("%s : champ %q, attendu %q", c.nom, invalide.Champ, c.champ)
		}
	}
}

// La disposition de la ligne d'entree est un contrat avec le PIC X(92) du
// programme COBOL. Les positions sont derivees d'une table de largeurs plutot
// que comptees a la main : ajouter un champ n'oblige alors qu'a une ligne de
// plus, sans recalculer tous les decalages.
func TestLigneEntreeRespecteLesPositionsCobol(t *testing.T) {
	champs := []struct {
		nom     string
		largeur int
		attendu string
	}{
		{"capital", 14, "00000250000000"},
		{"taux", 8, "08500000"},
		{"duree", 4, "0240"},
		{"frais_dossier", 12, "000001500000"},
		{"frais_garantie", 12, "000000900500"},
		{"taux_assurance", 8, "00360000"},
		{"tem", 4, "1025"},
		{"differe", 3, "024"},
		{"mois_anticipe", 4, "0120"},
		{"taux_indemnite", 6, "010000"},
		{"montant_anticipe", 14, "00000050000000"},
		{"methode", 1, "A"},
		{"assiette", 1, "R"},
		{"mode_anticipe", 1, "D"},
	}

	d, err := ParseDemande(Parametres{
		Capital: "250000.000", Taux: "8.5", Mois: "240",
		FraisDossier: "1500.000", FraisGarantie: "900.500",
		TauxAssurance: "0.36", Assiette: "capital_restant_du",
		Tem: "10.25", Differe: "24", MoisAnticipe: "120", Indemnite: "1",
		MontantAnticipe: "50000.000", Mode: "duree_reduite",
	})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}

	ligne := strings.TrimSuffix(ligneEntree(d), "\n")

	total := 0
	for _, c := range champs {
		total += c.largeur
	}
	if len(ligne) != total {
		t.Fatalf("longueur %d, attendu %d", len(ligne), total)
	}

	position := 0
	for _, c := range champs {
		got := ligne[position : position+c.largeur]
		if got != c.attendu {
			t.Errorf("%s en position %d : %q, attendu %q",
				c.nom, position+1, got, c.attendu)
		}
		position += c.largeur
	}

	// Les champs facultatifs restent a zero quand ils ne sont pas demandes,
	// et les lettres prennent leurs valeurs par defaut.
	nu, err := ParseDemande(Parametres{Capital: "250000.000", Taux: "8.5", Mois: "240"})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	ligneNue := strings.TrimSuffix(ligneEntree(nu), "\n")
	if got := ligneNue[26 : total-3]; strings.Trim(got, "0") != "" {
		t.Errorf("champs facultatifs non nuls : %q", got)
	}
	if got := ligneNue[total-3:]; got != "ANT" {
		t.Errorf("lettres par defaut : %q, attendu \"ANT\"", got)
	}

	// Chaque methode doit poser sa lettre.
	for saisie, lettre := range map[string]byte{
		"annuite_constante": 'A',
		"capital_constant":  'C',
		"in_fine":           'I',
	} {
		d, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240", Methode: saisie,
		})
		if err != nil {
			t.Fatalf("ParseDemande(%q) : %v", saisie, err)
		}
		if got := ligneEntree(d)[total-3]; got != lettre {
			t.Errorf("methode %q : lettre %q, attendu %q", saisie, got, lettre)
		}
	}
}

func TestFormaterEchelle(t *testing.T) {
	cas := []struct {
		valeur    int64
		decimales int
		attendu   string
	}{
		{25000000, 2, "250000.00"},
		{3450000, 6, "3.450000"},
		{1, 2, "0.01"},
		{0, 6, "0.000000"},
	}

	for _, c := range cas {
		if got := formaterEchelle(c.valeur, c.decimales); got != c.attendu {
			t.Errorf("formaterEchelle(%d, %d) = %q, attendu %q",
				c.valeur, c.decimales, got, c.attendu)
		}
	}
}

func TestAssietteDAssurance(t *testing.T) {
	cas := []struct {
		nom, taux, assiette string
		code                byte
		libelle             string
	}{
		// Sans taux ni assiette declares, il n'y a pas d'assurance.
		{"rien", "", "", 'N', "aucune"},
		// Un taux sans assiette porte sur le capital initial, convention la
		// plus repandue.
		{"taux seul", "0.36", "", 'I', "capital_initial"},
		{"assiette explicite", "0.36", "capital_restant_du", 'R', "capital_restant_du"},
		{"aucune malgre le taux", "0.36", "aucune", 'N', "aucune"},
		{"alias", "0.36", "R", 'R', "capital_restant_du"},
	}

	for _, c := range cas {
		d, err := ParseDemande(Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12",
			TauxAssurance: c.taux, Assiette: c.assiette,
		})
		if err != nil {
			t.Errorf("%s : %v", c.nom, err)
			continue
		}
		if d.CodeAssiette != c.code {
			t.Errorf("%s : code %q, attendu %q", c.nom, d.CodeAssiette, c.code)
		}
		if d.AssietteAssur != c.libelle {
			t.Errorf("%s : libelle %q, attendu %q", c.nom, d.AssietteAssur, c.libelle)
		}
	}
}

func TestParseDemandeRefuseLesFraisAberrants(t *testing.T) {
	cas := []struct {
		nom   string
		p     Parametres
		champ string
	}{
		{"frais superieurs au capital", Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12", FraisDossier: "1000.00",
		}, "frais_dossier"},
		{"frais cumules egaux au capital", Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12",
			FraisDossier: "600.00", FraisGarantie: "400.00",
		}, "frais_dossier"},
		{"frais negatifs", Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12", FraisDossier: "-5",
		}, "frais_dossier"},
		{"taux d'assurance hors bornes", Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12", TauxAssurance: "100",
		}, "taux_assurance"},
		{"assiette inconnue", Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12", Assiette: "forfaitaire",
		}, "assiette_assurance"},
	}

	for _, c := range cas {
		_, err := ParseDemande(c.p)
		if err == nil {
			t.Errorf("%s : aurait du echouer", c.nom)
			continue
		}
		invalide, ok := err.(*ErreurValidation)
		if !ok || invalide.Champ != c.champ {
			t.Errorf("%s : erreur %v, champ attendu %q", c.nom, err, c.champ)
		}
	}
}

func TestParseDemandeRefuseUnTemAberrant(t *testing.T) {
	cas := []struct{ nom, plafond string }{
		{"nul", "0"},
		{"hors bornes", "100"},
		{"non numerique", "abc"},
		{"negatif", "-5"},
	}
	for _, c := range cas {
		_, err := ParseDemande(Parametres{
			Capital: "1000.00", Taux: "3.45", Mois: "12", Tem: c.plafond,
		})
		if err == nil {
			t.Errorf("TEM %s : aurait du echouer", c.nom)
			continue
		}
		invalide, ok := err.(*ErreurValidation)
		if !ok || invalide.Champ != "tem" {
			t.Errorf("TEM %s : erreur %v, champ attendu tem", c.nom, err)
		}
	}
}
