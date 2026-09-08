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
		if _, err := ParseDemande(c.capital, c.taux, c.mois); err != nil {
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
		_, err := ParseDemande(c.capital, c.taux, c.mois)
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
	d, err := ParseDemande("250000.00", "3.45", "240")
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}

	ligne := ligneEntree(d)
	if got, want := ligne, "0000025000000034500000240\n"; got != want {
		t.Errorf("ligneEntree = %q, attendu %q", got, want)
	}
	// Le programme COBOL lit un PIC X(25) : la longueur est un contrat.
	if len(ligne)-1 != 25 {
		t.Errorf("longueur %d, attendu 25", len(ligne)-1)
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
