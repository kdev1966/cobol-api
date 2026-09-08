package loan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// binaire rend le chemin du programme COBOL compile, ou saute le test s'il
// n'a pas ete construit. Les tests de parsing, eux, tournent sans lui.
func binaire(t *testing.T) string {
	t.Helper()
	if chemin := os.Getenv("COBOL_PROGRAM_PATH"); chemin != "" {
		return chemin
	}
	chemin, err := filepath.Abs("../../bin/loan_amortization")
	if err != nil {
		t.Fatalf("chemin : %v", err)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Skip("binaire COBOL absent : cobc -x -free cobol/loan-amortization.cbl -o bin/loan_amortization")
	}
	return chemin
}

// recap et echeance fabriquent les enregistrements a largeur fixe attendus,
// aux memes positions que celles produites par le programme COBOL. Ni
// assurance, ni frais, ni verification du taux excessif : les tests de lecture
// portent sur le decoupage, pas sur le calcul. Les montants sont en millimes.
func recap(echeances int, premiere, derniere, interets, total string) string {
	return fmt.Sprintf("R%04d%014s%014s%016s%016s%014s%016s%016s%04d%04d%04d%s%s",
		echeances, premiere, derniere, interets,
		"0000000000000000", "00000000000000", total, interets,
		850, 0, 0, "-", "+0000")
}

func echeance(n int, mensualite, interets, capital, solde string) string {
	return fmt.Sprintf("E%04d%014s%014s%014s%014s%014s%014s",
		n, mensualite, interets, capital, "00000000000000", mensualite, solde)
}

func TestLireSortieEcarteLesLignesParasites(t *testing.T) {
	// libcob peut ecrire un avertissement sur la sortie standard ; il ne
	// doit pas faire echouer le calcul.
	sortie := bytes.NewBufferString(strings.Join([]string{
		"libcob: warning: implicit CLOSE of SYSIN",
		recap(2, "00000000100000", "00000000105000", "0000000000005000", "0000000000205000"),
		echeance(1, "00000000100000", "00000000003000", "00000000097000", "00000000103000"),
		"",
		echeance(2, "00000000105000", "00000000002000", "00000000103000", "00000000000000"),
		"note de fin sans structure",
	}, "\n"))

	res, err := lireSortie(sortie)
	if err != nil {
		t.Fatalf("lireSortie : %v", err)
	}
	if len(res.Echeancier) != 2 {
		t.Fatalf("%d echeances, attendu 2", len(res.Echeancier))
	}
	if got := res.Recapitulatif.PremiereMensualite.String(); got != "100.000" {
		t.Errorf("premiere mensualite %q, attendu \"100.000\"", got)
	}
	if got := res.Recapitulatif.DerniereMensualite.String(); got != "105.000" {
		t.Errorf("derniere mensualite %q, attendu \"105.000\"", got)
	}
	if got := res.Echeancier[1].Solde.String(); got != "0.000" {
		t.Errorf("solde final %q, attendu \"0.000\"", got)
	}
}

func TestLireSortieRefuseUnComptageIncoherent(t *testing.T) {
	sortie := bytes.NewBufferString(strings.Join([]string{
		recap(5, "00000000100000", "00000000105000", "0000000000005000", "0000000000205000"),
		echeance(1, "00000000100000", "00000000003000", "00000000097000", "00000000000000"),
	}, "\n"))

	if _, err := lireSortie(sortie); err == nil {
		t.Fatal("un echeancier tronque aurait du etre refuse")
	}
}

func TestLireSortieRefuseUnRecapitulatifEnDouble(t *testing.T) {
	ligne := recap(1, "00000000100000", "00000000100000", "0000000000005000", "0000000000205000")
	sortie := bytes.NewBufferString(ligne + "\n" + ligne)

	if _, err := lireSortie(sortie); err == nil {
		t.Fatal("un recapitulatif en double aurait du etre refuse")
	}
}

func TestLireSortieRefuseUneSortieVide(t *testing.T) {
	if _, err := lireSortie(bytes.NewBufferString("")); err == nil {
		t.Fatal("une sortie vide aurait du etre refusee")
	}
}

// Les montants traversent le service en json.Number : le texte produit par le
// COBOL doit ressortir a l'identique. Un float64 arrondirait ces valeurs.
func TestLesMontantsNeTransitentPasParUnFlottant(t *testing.T) {
	// 0.07 n'a pas de representation binaire exacte, et 8388608010 depasse la
	// precision d'un float32. Les deux doivent ressortir tels quels.
	sortie := bytes.NewBufferString(strings.Join([]string{
		recap(1, "00000000000070", "00000000000070", "0000000000000070", "0000000000000070"),
		echeance(1, "08388608010000", "00000000000070", "00000000000290", "00000000000000"),
	}, "\n"))

	res, err := lireSortie(sortie)
	if err != nil {
		t.Fatalf("lireSortie : %v", err)
	}
	if got := res.Recapitulatif.PremiereMensualite.String(); got != "0.070" {
		t.Errorf("premiere mensualite %q, attendu \"0.070\"", got)
	}
	if got := res.Echeancier[0].Mensualite.String(); got != "8388608010.000" {
		t.Errorf("mensualite %q, attendu \"8388608010.000\"", got)
	}
}

func TestNombreInsereLePointDecimal(t *testing.T) {
	cas := []struct {
		chiffres  string
		decimales int
		attendu   string
	}{
		{"00000002169558", 3, "2169.558"},
		{"00000000000070", 3, "0.070"},
		{"00000000000000", 3, "0.000"},
		{"99999999999999", 3, "99999999999.999"},
		{"0000", 3, "0.000"},
		// Le TEG est rendu a deux decimales, comme l'impose le decret.
		{"0850", 2, "8.50"},
		{"0000", 2, "0.00"},
		{"9999", 2, "99.99"},
	}
	for _, c := range cas {
		got, err := nombre(c.chiffres, c.decimales)
		if err != nil {
			t.Errorf("nombre(%q, %d) : %v", c.chiffres, c.decimales, err)
			continue
		}
		if got.String() != c.attendu {
			t.Errorf("nombre(%q, %d) = %q, attendu %q",
				c.chiffres, c.decimales, got, c.attendu)
		}
	}
	for _, mauvais := range []string{"", "12", "12x45", "  1234"} {
		if _, err := nombre(mauvais, 2); err == nil {
			t.Errorf("nombre(%q, 2) aurait du echouer", mauvais)
		}
	}
}

// millimes convertit un montant rendu par le COBOL en entier, pour totaliser
// sans flottant. Le dinar tunisien se divise en mille.
func millimes(t *testing.T, n interface{ String() string }) int64 {
	t.Helper()
	v, err := decimalVersEntier(n.String(), DecimalesMonnaie)
	if err != nil {
		t.Fatalf("montant %q illisible : %v", n.String(), err)
	}
	return v
}

func TestInvariantsDeLEcheancier(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	// Montants en dinars, taux plausibles sur le marche tunisien.
	cas := []struct{ capital, taux, mois string }{
		{"250000.000", "8.5", "240"},
		{"100000.000", "9.6", "120"},
		{"500000.000", "10.25", "300"},
		{"10000.000", "0", "12"},    // taux nul : la formule diviserait par zero
		{"1234.567", "11.23", "37"}, // montants et duree non ronds, au millime
		{"999999.999", "13.37", "360"},
		{"1000.000", "9.8", "1"},     // duree minimale
		{"50000.000", "0.01", "600"}, // duree maximale
		{"0.001", "8.5", "12"},       // capital minimal : un millime
	}

	// Les quatre invariants valent pour les trois methodes.
	for _, methode := range MethodesAcceptees() {
		for _, c := range cas {
			nom := methode + "/" + c.capital + "@" + c.taux + "%x" + c.mois
			t.Run(nom, func(t *testing.T) {
				verifierInvariants(t, moteur, Parametres{
					Capital: c.capital, Taux: c.taux, Mois: c.mois, Methode: methode,
				})
			})
		}
	}
}

func verifierInvariants(t *testing.T, moteur *Moteur, p Parametres) {
	t.Helper()

	d, err := ParseDemande(p)
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}

	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	var sommeCapital, sommeInterets, sommeEcheances int64
	var sommeAssurance, sommeMensualites int64
	for _, e := range res.Echeancier {
		p := millimes(t, e.Echeance)
		i := millimes(t, e.Interets)
		k := millimes(t, e.Capital)

		// Invariant 4 : la part de credit s'equilibre.
		if p != i+k {
			t.Errorf("echeance %d : echeance %d != interets %d + capital %d", e.N, p, i, k)
		}
		// Invariant 5 : la mensualite est l'echeance plus l'assurance.
		a := millimes(t, e.Assurance)
		if m := millimes(t, e.Mensualite); m != p+a {
			t.Errorf("echeance %d : mensualite %d != echeance %d + assurance %d",
				e.N, m, p, a)
		}
		sommeCapital += k
		sommeInterets += i
		sommeEcheances += p
		sommeAssurance += a
		sommeMensualites += millimes(t, e.Mensualite)
	}

	// Invariant 1 : la somme des parts de capital egale le capital emprunte.
	if sommeCapital != d.CapitalMillimes {
		t.Errorf("somme des parts de capital %d, capital emprunte %d",
			sommeCapital, d.CapitalMillimes)
	}
	// Invariant 2 : les echeances valent le capital plus les interets.
	if sommeEcheances != d.CapitalMillimes+sommeInterets {
		t.Errorf("somme des echeances %d, attendu %d",
			sommeEcheances, d.CapitalMillimes+sommeInterets)
	}
	// Invariant 3 : le solde final est nul.
	if solde := millimes(t, res.Echeancier[len(res.Echeancier)-1].Solde); solde != 0 {
		t.Errorf("solde final %d, attendu 0", solde)
	}

	// Le recapitulatif doit concorder avec le detail.
	if len(res.Echeancier) != d.Mois {
		t.Errorf("%d echeances, attendu %d", len(res.Echeancier), d.Mois)
	}
	if got := millimes(t, res.Recapitulatif.TotalInterets); got != sommeInterets {
		t.Errorf("total_interets %d, detail %d", got, sommeInterets)
	}
	// Invariant 6 : le total verse est la somme des mensualites.
	if got := millimes(t, res.Recapitulatif.TotalVerse); got != sommeMensualites {
		t.Errorf("total_verse %d, detail %d", got, sommeMensualites)
	}
	if got := millimes(t, res.Recapitulatif.TotalAssurance); got != sommeAssurance {
		t.Errorf("total_assurance %d, detail %d", got, sommeAssurance)
	}
	// Le cout du credit agrege interets, assurance et frais.
	frais := d.FraisDossierMillimes + d.FraisGarantieMillimes
	if got := millimes(t, res.Recapitulatif.CoutCredit); got != sommeInterets+sommeAssurance+frais {
		t.Errorf("cout_credit %d, attendu %d", got, sommeInterets+sommeAssurance+frais)
	}
	if got := millimes(t, res.Recapitulatif.TotalFrais); got != frais {
		t.Errorf("total_frais %d, attendu %d", got, frais)
	}
	if got, veut := millimes(t, res.Recapitulatif.PremiereMensualite),
		millimes(t, res.Echeancier[0].Mensualite); got != veut {
		t.Errorf("premiere_mensualite %d, detail %d", got, veut)
	}
	if got, veut := millimes(t, res.Recapitulatif.DerniereMensualite),
		millimes(t, res.Echeancier[len(res.Echeancier)-1].Mensualite); got != veut {
		t.Errorf("derniere_mensualite %d, detail %d", got, veut)
	}
}

// Chaque methode a une signature propre, verifiee en plus des invariants
// communs.
func TestSignatureDeChaqueMethode(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	calculer := func(methode string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{Capital: "120000.000", Taux: "11.23", Mois: "60", Methode: methode})
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	// Annuite constante : toutes les echeances sauf la derniere sont egales.
	annuite := calculer("annuite_constante")
	ref := annuite.Echeancier[0].Mensualite.String()
	for _, e := range annuite.Echeancier[:len(annuite.Echeancier)-1] {
		if e.Mensualite.String() != ref {
			t.Fatalf("annuite constante : echeance %d vaut %s, attendu %s", e.N, e.Mensualite, ref)
		}
	}

	// Capital constant : la part de capital ne bouge pas avant la derniere,
	// et l'echeance decroit.
	lineaire := calculer("capital_constant")
	refCap := lineaire.Echeancier[0].Capital.String()
	for _, e := range lineaire.Echeancier[:len(lineaire.Echeancier)-1] {
		if e.Capital.String() != refCap {
			t.Fatalf("capital constant : part de capital %s a l'echeance %d, attendu %s",
				e.Capital, e.N, refCap)
		}
	}
	if millimes(t, lineaire.Echeancier[0].Mensualite) <=
		millimes(t, lineaire.Echeancier[len(lineaire.Echeancier)-1].Mensualite) {
		t.Error("capital constant : l'echeance devrait decroitre")
	}

	// In fine : aucun capital rembourse avant la derniere echeance.
	inFine := calculer("in_fine")
	for _, e := range inFine.Echeancier[:len(inFine.Echeancier)-1] {
		if millimes(t, e.Capital) != 0 {
			t.Fatalf("in fine : capital %s rembourse a l'echeance %d", e.Capital, e.N)
		}
	}
	if got := inFine.Echeancier[len(inFine.Echeancier)-1].Capital.String(); got != "120000.000" {
		t.Errorf("in fine : derniere part de capital %s, attendu 120000.000", got)
	}

	// Plus l'amortissement est rapide, moins on paie d'interets.
	iLin := millimes(t, lineaire.Recapitulatif.TotalInterets)
	iAnn := millimes(t, annuite.Recapitulatif.TotalInterets)
	iFin := millimes(t, inFine.Recapitulatif.TotalInterets)
	if iLin >= iAnn || iAnn >= iFin {
		t.Errorf("interets attendus capital_constant < annuite < in_fine, recu %d %d %d",
			iLin, iAnn, iFin)
	}
}

// Cas de reference verifie independamment avec le module decimal de Python.
func TestCasDeReference(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	// Credit logement de 250 000 dinars a 8,5 % sur vingt ans. Toutes les
	// valeurs sont recoupees avec une resolution independante en Decimal
	// Python, au millime.
	d, err := ParseDemande(Parametres{Capital: "250000.000", Taux: "8.5", Mois: "240"})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	references := map[string][2]string{
		"mensualite":     {res.Recapitulatif.PremiereMensualite.String(), "2169.558"},
		"total_interets": {res.Recapitulatif.TotalInterets.String(), "270693.976"},
		"total_verse":    {res.Recapitulatif.TotalVerse.String(), "520693.976"},
		// Le decret n° 2000-462 annualise proportionnellement : sans frais ni
		// assurance, le TEG egale exactement le taux nominal.
		"teg": {res.Recapitulatif.Teg.String(), "8.50"},
	}
	for cle, v := range references {
		if v[0] != v[1] {
			t.Errorf("%s = %s, reference %s", cle, v[0], v[1])
		}
	}

	premiere := res.Echeancier[0]
	if premiere.Interets.String() != "1770.833" || premiere.Capital.String() != "398.725" {
		t.Errorf("premiere echeance : interets %s capital %s, attendu 1770.833 / 398.725",
			premiere.Interets, premiere.Capital)
	}
	// La derniere echeance absorbe le residu d'arrondi : elle differe de la
	// mensualite nominale.
	derniere := res.Echeancier[len(res.Echeancier)-1]
	if derniere.Mensualite.String() != "2169.614" {
		t.Errorf("derniere echeance %s, attendu 2169.614", derniere.Mensualite)
	}
}

// Sans frais ni assurance, le TEG egale le taux nominal, quelle que soit la
// maniere dont le capital est amorti : le decret annualise proportionnellement
// le taux de periode. C'est une propriete du domaine, verifiee ici comme telle.
func TestLeTegNeDependPasDeLaMethode(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	var reference string
	for _, methode := range MethodesAcceptees() {
		d, err := ParseDemande(Parametres{Capital: "120000.000", Taux: "11.23", Mois: "60", Methode: methode})
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}

		got := res.Recapitulatif.Teg.String()
		if reference == "" {
			reference = got
			// Annualisation proportionnelle : le TEG rejoint le nominal.
			if got != "11.23" {
				t.Errorf("teg %s, attendu 11.23", got)
			}
			continue
		}
		if got != reference {
			t.Errorf("methode %s : teg %s, attendu %s", methode, got, reference)
		}
	}
}

func TestLeTegEstNulSansInterets(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	d, err := ParseDemande(Parametres{Capital: "10000.000", Taux: "0", Mois: "12"})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}
	if got := res.Recapitulatif.Teg.String(); got != "0.00" {
		t.Errorf("teg %s, attendu 0.00", got)
	}
}

func TestCalculerRemonteLEchecDuProgramme(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	// Capital nul : le programme COBOL doit refuser et sortir en erreur.
	_, err := moteur.Calculer(context.Background(), Demande{
		CapitalMillimes: 0, TauxMillioniemes: 3450000, Mois: 12,
		CodeMethode: 'A', CodeAssiette: 'N',
	})
	if err == nil {
		t.Fatal("un capital nul aurait du faire echouer le programme")
	}
}

func TestCalculerAbandonneApresLeDelai(t *testing.T) {
	chemin, err := filepath.Abs("testdata/lent.sh")
	if err != nil {
		t.Fatalf("chemin : %v", err)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Skip("fixture testdata/lent.sh absente")
	}

	moteur := NewMoteur(chemin, 150*time.Millisecond)
	d, err := ParseDemande(Parametres{Capital: "1000.00", Taux: "3.45", Mois: "12", Methode: ""})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}

	debut := time.Now()
	_, err = moteur.Calculer(context.Background(), d)
	ecoule := time.Since(debut)

	if !errors.Is(err, ErrDelaiDepasse) {
		t.Fatalf("erreur %v, attendu ErrDelaiDepasse", err)
	}
	// Sans WaitDelay, l'appel resterait bloque les trente secondes du script.
	if ecoule > 3*time.Second {
		t.Errorf("abandon apres %s, bien au-dela du delai demande", ecoule)
	}
}

func TestNewMoteurRetombeSurLeDelaiParDefaut(t *testing.T) {
	for _, delai := range []time.Duration{0, -time.Second} {
		if got := NewMoteur("/x", delai).Delai; got != DelaiParDefaut {
			t.Errorf("NewMoteur(_, %s).Delai = %s, attendu %s",
				delai, got, DelaiParDefaut)
		}
	}
	if got := NewMoteur("/x", 2*time.Second).Delai; got != 2*time.Second {
		t.Errorf("un delai explicite doit etre conserve, recu %s", got)
	}
}

func TestCalculerAvecUnBinaireIntrouvable(t *testing.T) {
	moteur := NewMoteur("/inexistant/loan_amortization", 0)
	d, _ := ParseDemande(Parametres{Capital: "1000.00", Taux: "3.45", Mois: "12", Methode: ""})

	if _, err := moteur.Calculer(context.Background(), d); err == nil {
		t.Fatal("un binaire absent aurait du produire une erreur")
	}
}

// Les invariants doivent tenir avec des frais et une assurance, sur les trois
// methodes et les deux assiettes.
func TestInvariantsAvecFraisEtAssurance(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	cas := []Parametres{
		{Capital: "250000.000", Taux: "8.5", Mois: "240", FraisDossier: "1500.000"},
		{Capital: "250000.000", Taux: "8.5", Mois: "240",
			FraisDossier: "1500.000", FraisGarantie: "900.000"},
		{Capital: "250000.000", Taux: "8.5", Mois: "240",
			TauxAssurance: "0.36", Assiette: "capital_initial"},
		{Capital: "250000.000", Taux: "8.5", Mois: "240",
			TauxAssurance: "0.36", Assiette: "capital_restant_du"},
		{Capital: "120000.000", Taux: "11.23", Mois: "60",
			FraisDossier: "800.000", TauxAssurance: "0.5", Assiette: "capital_restant_du"},
		// Taux nul mais assurance : les flux ne sont pas triviaux pour autant.
		{Capital: "10000.000", Taux: "0", Mois: "12",
			TauxAssurance: "0.4", Assiette: "capital_initial"},
	}

	for _, methode := range MethodesAcceptees() {
		for i, p := range cas {
			p.Methode = methode
			t.Run(fmt.Sprintf("%s/%d", methode, i), func(t *testing.T) {
				verifierInvariants(t, moteur, p)
			})
		}
	}
}

// Les frais et l'assurance doivent faire monter le TAEG, et l'assurance sur le
// capital restant du couter moins que sur le capital initial.
func TestLesFraisEtLAssuranceRencherissentLeTeg(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	calculer := func(p Parametres) *Echeancier {
		t.Helper()
		p.Capital, p.Taux, p.Mois = "250000.000", "8.5", "240"
		d, err := ParseDemande(p)
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	nu := calculer(Parametres{})
	avecFrais := calculer(Parametres{FraisDossier: "1500.000"})
	surInitial := calculer(Parametres{TauxAssurance: "0.36", Assiette: "capital_initial"})
	surRestant := calculer(Parametres{TauxAssurance: "0.36", Assiette: "capital_restant_du"})

	teg := func(e *Echeancier) int64 {
		v, err := decimalVersEntier(e.Recapitulatif.Teg.String(), 2)
		if err != nil {
			t.Fatalf("teg illisible : %v", err)
		}
		return v
	}

	// Sans frais ni assurance, le TEG rejoint le taux nominal.
	if got := nu.Recapitulatif.Teg.String(); got != "8.50" {
		t.Errorf("sans frais ni assurance : teg %s, attendu 8.50", got)
	}

	if teg(avecFrais) <= teg(nu) {
		t.Error("les frais devraient faire monter le TEG")
	}
	if teg(surInitial) <= teg(nu) {
		t.Error("l'assurance devrait faire monter le TEG")
	}
	// Une prime assise sur le capital restant du decroit : elle coute moins.
	if teg(surRestant) >= teg(surInitial) {
		t.Error("l'assurance sur capital restant du devrait couter moins que sur capital initial")
	}
	if millimes(t, surRestant.Recapitulatif.TotalAssurance) >=
		millimes(t, surInitial.Recapitulatif.TotalAssurance) {
		t.Error("le total d'assurance sur capital restant du devrait etre inferieur")
	}

	// Sans assurance, la colonne reste a zero partout.
	for _, e := range nu.Echeancier {
		if e.Assurance.String() != "0.000" {
			t.Fatalf("assurance %s a l'echeance %d alors qu'aucune n'est souscrite",
				e.Assurance, e.N)
		}
	}
	// Sur capital initial, la prime est constante.
	ref := surInitial.Echeancier[0].Assurance.String()
	for _, e := range surInitial.Echeancier {
		if e.Assurance.String() != ref {
			t.Fatalf("prime %s a l'echeance %d, attendu %s constante", e.Assurance, e.N, ref)
		}
	}
}

// Loi n° 99-64 : est excessif tout pret dont le TEG excede de plus du
// cinquieme le taux effectif moyen de la categorie.
func TestVerdictDeTauxExcessif(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	calculer := func(tem string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240", Tem: tem,
		})
		if err != nil {
			t.Fatalf("ParseDemande(%q) : %v", tem, err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	// Sans taux effectif moyen, le TEG est rendu sans jugement.
	sans := calculer("").Recapitulatif
	if sans.Conforme != nil || sans.Tem != nil || sans.Seuil != nil || sans.Marge != nil {
		t.Errorf("sans TEM, le verdict devrait etre nul : %+v", sans)
	}
	if sans.Teg.String() != "8.50" {
		t.Errorf("teg %s, attendu 8.50", sans.Teg)
	}

	cas := []struct {
		nom      string
		tem      string
		seuil    string
		conforme bool
		marge    string
	}{
		// Credit logement, TEM publie a 10,25 : seuil 12,30.
		{"credit logement", "10.25", "12.30", true, "3.80"},
		// Le seuil est le TEM majore d'un cinquieme, arrondi a deux
		// decimales comme les taux publies par arrete.
		{"court terme", "9.57", "11.48", true, "2.98"},
		{"consommation", "11.23", "13.48", true, "4.98"},
		// Un TEG egal au seuil reste licite : le depassement est strict.
		{"egal au seuil", "7.09", "8.51", true, "0.01"},
		{"depassement", "7", "8.40", false, "-0.10"},
	}

	for _, c := range cas {
		r := calculer(c.tem).Recapitulatif
		if r.Conforme == nil || r.Seuil == nil || r.Tem == nil || r.Marge == nil {
			t.Errorf("%s : verdict incomplet", c.nom)
			continue
		}
		if r.Seuil.String() != c.seuil {
			t.Errorf("%s : seuil %s, attendu %s (TEM %s majore d'un cinquieme)",
				c.nom, r.Seuil, c.seuil, c.tem)
		}
		if *r.Conforme != c.conforme {
			t.Errorf("%s : conforme=%v, attendu %v", c.nom, *r.Conforme, c.conforme)
		}
		if r.Marge.String() != c.marge {
			t.Errorf("%s : marge %s, attendu %s", c.nom, r.Marge, c.marge)
		}
	}
}

// Les seuils publies par arrete doivent se retrouver a partir des taux
// effectifs moyens : c'est la regle du cinquieme, arrondie a deux decimales.
func TestLeSeuilRedonneLesValeursPubliees(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	// Arrete du 28 juillet 2026, taux effectifs moyens et seuils
	// correspondants publies par la Banque Centrale de Tunisie.
	publies := []struct{ categorie, tem, seuil string }{
		{"leasing", "13.37", "16.04"},
		{"decouverts", "12.29", "14.75"},
		{"gestion des dettes", "11.78", "14.14"},
		{"credits a la consommation", "11.23", "13.48"},
		{"credits logement", "10.25", "12.30"},
		{"credits a moyen terme", "9.80", "11.76"},
		{"credits a long terme", "9.63", "11.56"},
		{"credits a court terme", "9.57", "11.48"},
	}

	for _, c := range publies {
		d, err := ParseDemande(Parametres{
			Capital: "100000.000", Taux: "5", Mois: "60", Tem: c.tem,
		})
		if err != nil {
			t.Fatalf("%s : %v", c.categorie, err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("%s : %v", c.categorie, err)
		}
		if res.Recapitulatif.Seuil == nil {
			t.Errorf("%s : seuil absent", c.categorie)
			continue
		}
		if got := res.Recapitulatif.Seuil.String(); got != c.seuil {
			t.Errorf("%s : seuil %s, publie %s", c.categorie, got, c.seuil)
		}
	}
}

// Les frais et l'assurance font monter le TAEG : un pret licite nu peut
// devenir usuraire une fois tous les couts integres. C'est precisement ce que
// la reglementation vise, et le service doit le voir.
func TestUnPretLiciteNuPeutDevenirExcessif(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	verdict := func(p Parametres) (bool, string) {
		t.Helper()
		// Credit a la consommation : TEM 11,23, donc seuil 13,48.
		p.Capital, p.Taux, p.Mois, p.Tem = "120000.000", "11.23", "60", "11.23"
		d, err := ParseDemande(p)
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		if res.Recapitulatif.Conforme == nil {
			t.Fatal("verdict absent")
		}
		return *res.Recapitulatif.Conforme, res.Recapitulatif.Teg.String()
	}

	if ok, teg := verdict(Parametres{}); !ok {
		t.Errorf("nu : teg %s devrait passer sous le seuil de 13.48", teg)
	}
	if ok, teg := verdict(Parametres{
		TauxAssurance: "2.5", Assiette: "capital_initial",
	}); ok {
		t.Errorf("avec assurance : teg %s devrait depasser le seuil de 13.48", teg)
	}
}

func TestNombreSigne(t *testing.T) {
	cas := []struct{ champ, attendu string }{
		{"+0380", "3.80"},
		{"-0051", "-0.51"},
		{"+0000", "0.00"},
		// Le zero negatif ne doit pas ressortir avec un signe.
		{"-0000", "0.00"},
	}
	for _, c := range cas {
		got, err := nombreSigne(c.champ, 2)
		if err != nil {
			t.Errorf("nombreSigne(%q) : %v", c.champ, err)
			continue
		}
		if got.String() != c.attendu {
			t.Errorf("nombreSigne(%q) = %q, attendu %q", c.champ, got, c.attendu)
		}
	}
	for _, mauvais := range []string{"", "+", "x0000", "00000"} {
		if _, err := nombreSigne(mauvais, 2); err == nil {
			t.Errorf("nombreSigne(%q) aurait du echouer", mauvais)
		}
	}
}
