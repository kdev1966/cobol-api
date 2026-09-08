package loan

import "testing"

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

func TestLigneEntreeRespecteLesPositionsCobol(t *testing.T) {
	d, err := ParseDemande(Parametres{Capital: "250000.00", Taux: "3.45", Mois: "240", Methode: ""})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}

	ligne := ligneEntree(d)
	// 13 chiffres de capital, 8 de taux, 4 de duree, 11 de frais de dossier,
	// 11 de frais de garantie, 8 de taux d'assurance, puis les lettres de la
	// methode et de l'assiette. La longueur est un contrat avec le PIC X(57)
	// du COBOL.
	want := "0000025000000034500000240" + "00000000000" + "00000000000" +
		"00000000" + "AN\n"
	if ligne != want {
		t.Errorf("ligneEntree = %q, attendu %q", ligne, want)
	}
	if len(ligne)-1 != 57 {
		t.Errorf("longueur %d, attendu 57", len(ligne)-1)
	}

	// Les frais et l'assurance doivent se retrouver a leurs positions.
	avecFrais, err := ParseDemande(Parametres{
		Capital: "250000.00", Taux: "3.45", Mois: "240",
		FraisDossier: "1500.00", FraisGarantie: "900.50",
		TauxAssurance: "0.36", Assiette: "capital_restant_du",
	})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	l := ligneEntree(avecFrais)
	if got := l[25:36]; got != "00000150000" {
		t.Errorf("frais de dossier a la position 26 : %q", got)
	}
	if got := l[36:47]; got != "00000090050" {
		t.Errorf("frais de garantie a la position 37 : %q", got)
	}
	if got := l[47:55]; got != "00360000" {
		t.Errorf("taux d'assurance a la position 48 : %q", got)
	}
	if got := l[55:57]; got != "AR" {
		t.Errorf("lettres de methode et d'assiette : %q", got)
	}

	// Chaque methode doit poser sa lettre.
	for saisie, lettre := range map[string]byte{
		"annuite_constante": 'A',
		"capital_constant":  'C',
		"in_fine":           'I',
	} {
		d, err := ParseDemande(Parametres{Capital: "250000.00", Taux: "3.45", Mois: "240", Methode: saisie})
		if err != nil {
			t.Fatalf("ParseDemande(%q) : %v", saisie, err)
		}
		if got := ligneEntree(d)[55]; got != lettre {
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
