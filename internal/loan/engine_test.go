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
// binaireCapacite rend le programme du calcul inverse, ou une chaine vide :
// les tests qui n'en ont pas besoin s'en passent.
func binaireCapacite(t *testing.T) string {
	t.Helper()
	if chemin := os.Getenv("COBOL_CAPACITY_PATH"); chemin != "" {
		return chemin
	}
	chemin, err := filepath.Abs("../../bin/loan_capacity")
	if err != nil {
		return ""
	}
	if _, err := os.Stat(chemin); err != nil {
		return ""
	}
	return chemin
}

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
	return fmt.Sprintf("R%04d%014s%014s%016s%016s%014s%016s%016s%04d%04d%04d%s%s%016s",
		echeances, premiere, derniere, interets,
		"0000000000000000", "00000000000000", total, interets,
		850, 0, 0, "-", "+0000", "0000000000000000")
}

func echeance(n int, mensualite, interets, capital, solde string) string {
	return fmt.Sprintf("E%04d%014s%014s%014s%014s%014s%014s%014s",
		n, mensualite, interets, capital, "00000000000000", mensualite, solde,
		"00000000000000")
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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	var sommeAssurance, sommeMensualites, sommeCapitalise int64
	for _, e := range res.Echeancier {
		p := millimes(t, e.Echeance)
		i := millimes(t, e.Interets)
		k := millimes(t, e.Capital)
		c := millimes(t, e.Capitalise)

		// Invariant 4 : la part de credit s'equilibre. L'interet capitalise
		// n'est pas verse : il s'ajoute au capital au lieu d'entrer dans
		// l'echeance. Sous franchise partielle il est toujours nul, et
		// l'invariant retrouve sa forme simple.
		if p != i+k-c {
			t.Errorf("echeance %d : echeance %d != interets %d + capital %d "+
				"- capitalise %d", e.N, p, i, k, c)
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
		sommeCapitalise += c
		sommeMensualites += millimes(t, e.Mensualite)
	}

	// Invariant 1 : la somme des parts de capital egale le capital emprunte,
	// grossi des interets que la franchise totale y a ajoutes. Sans franchise
	// totale la capitalisation est nulle et l'egalite reste la plus simple.
	if sommeCapital != d.CapitalMillimes+sommeCapitalise {
		t.Errorf("somme des parts de capital %d, capital emprunte %d "+
			"plus capitalise %d", sommeCapital, d.CapitalMillimes,
			sommeCapitalise)
	}
	// Le recapitulatif doit rendre la meme capitalisation que le detail.
	if got := millimes(t, res.Recapitulatif.InteretsCapitalises); got != sommeCapitalise {
		t.Errorf("interets_capitalises %d, detail %d", got, sommeCapitalise)
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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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

	moteur := NewMoteur(chemin, "", 150*time.Millisecond)
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
		if got := NewMoteur("/x", "", delai).Delai; got != DelaiParDefaut {
			t.Errorf("NewMoteur(_, %s).Delai = %s, attendu %s",
				delai, got, DelaiParDefaut)
		}
	}
	if got := NewMoteur("/x", "", 2*time.Second).Delai; got != 2*time.Second {
		t.Errorf("un delai explicite doit etre conserve, recu %s", got)
	}
}

func TestCalculerAvecUnBinaireIntrouvable(t *testing.T) {
	moteur := NewMoteur("/inexistant/loan_amortization", "", 0)
	d, _ := ParseDemande(Parametres{Capital: "1000.00", Taux: "3.45", Mois: "12", Methode: ""})

	if _, err := moteur.Calculer(context.Background(), d); err == nil {
		t.Fatal("un binaire absent aurait du produire une erreur")
	}
}

// Les invariants doivent tenir avec des frais et une assurance, sur les trois
// methodes et les deux assiettes.
func TestInvariantsAvecFraisEtAssurance(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

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

// Le capital rendu doit etre le plus grand qui tienne dans le budget : un
// millime de plus doit le depasser. C'est la propriete que la dichotomie
// garantit, et elle se verifie contre l'echeancier reel.
func TestLaCapaciteEstMaximale(t *testing.T) {
	chemin := binaireCapacite(t)
	if chemin == "" {
		t.Skip("binaire de capacite absent")
	}
	moteur := NewMoteur(binaire(t), chemin, 0)
	ctx := context.Background()

	cas := []Parametres{
		{Mensualite: "2169.558", Taux: "8.5", Mois: "240"},
		{Mensualite: "1500.000", Taux: "13", Mois: "60",
			TauxAssurance: "1.5", Assiette: "capital_initial"},
		{Mensualite: "2000.000", Taux: "8.5", Mois: "240", Methode: "capital_constant"},
		{Mensualite: "2000.000", Taux: "8.5", Mois: "240", Methode: "in_fine"},
		{Mensualite: "833.333", Taux: "0", Mois: "12"},
		// Avec un differe, la contrainte porte sur la premiere echeance
		// amortissante et non sur la franchise.
		{Mensualite: "2000.000", Taux: "8.5", Mois: "240", Differe: "24"},
	}

	for _, p := range cas {
		nom := p.Mensualite + "@" + p.Taux + "/" + p.Methode
		t.Run(nom, func(t *testing.T) {
			budget, err := ParseDemandeCapacite(p)
			if err != nil {
				t.Fatalf("ParseDemandeCapacite : %v", err)
			}
			cap, err := moteur.Capaciter(ctx, budget)
			if err != nil {
				t.Fatalf("Capaciter : %v", err)
			}

			// La mensualite annoncee doit tenir dans le budget.
			if millimes(t, cap.Mensualite) > budget.BudgetMillimes {
				t.Fatalf("mensualite %s au-dessus du budget %s",
					cap.Mensualite, p.Mensualite)
			}

			// Elle doit correspondre a l'echeancier reellement produit.
			// Avec une franchise, c'est la premiere echeance amortissante
			// qui doit tenir dans le budget.
			premiere := func(capital string) int64 {
				t.Helper()
				p2 := p
				p2.Capital, p2.Mensualite = capital, ""
				d, err := ParseDemande(p2)
				if err != nil {
					t.Fatalf("ParseDemande(%s) : %v", capital, err)
				}
				res, err := moteur.Calculer(ctx, d)
				if err != nil {
					t.Fatalf("Calculer(%s) : %v", capital, err)
				}
				return millimes(t, res.Echeancier[d.DiffereMois].Mensualite)
			}

			capital := millimes(t, cap.Capital)
			if got := premiere(cap.Capital.String()); got != millimes(t, cap.Mensualite) {
				t.Errorf("echeancier reel : mensualite %d, capacite annoncee %s",
					got, cap.Mensualite)
			}
			// Un millime de plus doit depasser le budget : c'est ce qui fait
			// du resultat un maximum et non une approximation prudente.
			suivant := formaterEchelle(capital+1, DecimalesMonnaie)
			if got := premiere(suivant); got <= budget.BudgetMillimes {
				t.Errorf("capital %s tient encore dans le budget : la capacite n'est pas maximale",
					suivant)
			}
		})
	}
}

// Plus l'amortissement est lent, plus on peut emprunter a mensualite egale.
func TestLaCapaciteDependDeLaMethode(t *testing.T) {
	chemin := binaireCapacite(t)
	if chemin == "" {
		t.Skip("binaire de capacite absent")
	}
	moteur := NewMoteur(binaire(t), chemin, 0)

	capital := func(methode string) int64 {
		t.Helper()
		d, err := ParseDemandeCapacite(Parametres{
			Mensualite: "2000.000", Taux: "8.5", Mois: "240", Methode: methode,
		})
		if err != nil {
			t.Fatalf("ParseDemandeCapacite : %v", err)
		}
		c, err := moteur.Capaciter(context.Background(), d)
		if err != nil {
			t.Fatalf("Capaciter : %v", err)
		}
		return millimes(t, c.Capital)
	}

	lineaire, annuite, inFine := capital("capital_constant"), capital("annuite_constante"), capital("in_fine")
	if lineaire >= annuite || annuite >= inFine {
		t.Errorf("capacites attendues capital_constant < annuite < in_fine, recu %d %d %d",
			lineaire, annuite, inFine)
	}
}

func TestLireCapaciteRefuseUneSortieMalformee(t *testing.T) {
	for _, mauvaise := range []string{"", "C123", "X" + strings.Repeat("0", 32),
		"C" + strings.Repeat("x", 32)} {
		if _, err := lireCapacite(mauvaise); err == nil {
			t.Errorf("lireCapacite(%q) aurait du echouer", mauvaise)
		}
	}
}

func TestParseDemandeCapaciteValideLeBudget(t *testing.T) {
	for _, mauvais := range []string{"", "0", "abc", "-5", "100000000000000"} {
		_, err := ParseDemandeCapacite(Parametres{
			Mensualite: mauvais, Taux: "8.5", Mois: "240",
		})
		if err == nil {
			t.Errorf("mensualite %q : aurait du echouer", mauvais)
			continue
		}
		invalide, ok := err.(*ErreurValidation)
		if !ok || invalide.Champ != "mensualite" {
			t.Errorf("mensualite %q : erreur %v, champ attendu mensualite", mauvais, err)
		}
	}
	// Les autres parametres restent valides comme pour l'echeancier.
	if _, err := ParseDemandeCapacite(Parametres{
		Mensualite: "2000.000", Taux: "8.5", Mois: "999",
	}); err == nil {
		t.Error("une duree hors bornes aurait du etre refusee")
	}
}

// Pendant la franchise, seuls les interets et l'assurance sont dus : le
// capital reste intact, et les invariants tiennent malgre tout.
func TestDiffereDAmortissement(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

	d, err := ParseDemande(Parametres{
		Capital: "250000.000", Taux: "8.5", Mois: "240", Differe: "24",
	})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	// Franchise : aucun capital amorti, solde intact.
	for i := 0; i < 24; i++ {
		e := res.Echeancier[i]
		if millimes(t, e.Capital) != 0 {
			t.Fatalf("echeance %d : capital %s amorti pendant la franchise", e.N, e.Capital)
		}
		if millimes(t, e.Solde) != d.CapitalMillimes {
			t.Fatalf("echeance %d : solde %s, capital devrait rester intact", e.N, e.Solde)
		}
		// L'echeance se reduit aux interets.
		if e.Echeance.String() != e.Interets.String() {
			t.Fatalf("echeance %d : %s pour %s d'interets", e.N, e.Echeance, e.Interets)
		}
	}
	// L'amortissement commence juste apres.
	if millimes(t, res.Echeancier[24].Capital) == 0 {
		t.Error("l'amortissement devrait commencer a la 25e echeance")
	}

	// La mensualite d'amortissement est plus elevee : meme capital, moins
	// d'echeances pour l'amortir.
	sans, err := ParseDemande(Parametres{Capital: "250000.000", Taux: "8.5", Mois: "240"})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	resSans, err := moteur.Calculer(context.Background(), sans)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}
	if millimes(t, res.Echeancier[24].Mensualite) <= millimes(t, resSans.Echeancier[0].Mensualite) {
		t.Error("l'echeance apres franchise devrait depasser celle sans differe")
	}
	// Et le credit coute plus cher : on paie des interets sans rien amortir.
	if millimes(t, res.Recapitulatif.TotalInterets) <=
		millimes(t, resSans.Recapitulatif.TotalInterets) {
		t.Error("un differe devrait rencherir le credit")
	}
}

func TestLesInvariantsTiennentAvecUnDiffere(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

	cas := []Parametres{
		{Capital: "250000.000", Taux: "8.5", Mois: "240", Differe: "24"},
		{Capital: "120000.000", Taux: "11.23", Mois: "60", Differe: "1"},
		// Franchise maximale : une seule echeance pour amortir.
		{Capital: "60000.000", Taux: "13", Mois: "12", Differe: "11"},
		{Capital: "100000.000", Taux: "0", Mois: "36", Differe: "6"},
		{Capital: "250000.000", Taux: "8.5", Mois: "240", Differe: "24",
			FraisDossier: "1500.000", TauxAssurance: "0.36", Assiette: "capital_restant_du"},
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

func TestParseDemandeRefuseUnDiffereAberrant(t *testing.T) {
	cas := []struct{ nom, differe, mois string }{
		{"egal a la duree", "12", "12"},
		{"superieur a la duree", "13", "12"},
		{"negatif", "-1", "12"},
		{"non entier", "abc", "12"},
		{"hors bornes", "600", "600"},
	}
	for _, c := range cas {
		_, err := ParseDemande(Parametres{
			Capital: "1000.000", Taux: "8.5", Mois: c.mois, Differe: c.differe,
		})
		if err == nil {
			t.Errorf("differe %s : aurait du echouer", c.nom)
			continue
		}
		invalide, ok := err.(*ErreurValidation)
		if !ok || invalide.Champ != "differe" {
			t.Errorf("differe %s : erreur %v, champ attendu differe", c.nom, err)
		}
	}
}

// Un differe reduit la capacite : la mensualite amortissante doit rembourser
// le meme capital en moins d'echeances.
func TestLeDiffereReduitLaCapacite(t *testing.T) {
	chemin := binaireCapacite(t)
	if chemin == "" {
		t.Skip("binaire de capacite absent")
	}
	moteur := NewMoteur(binaire(t), chemin, 0)

	capital := func(differe string) int64 {
		t.Helper()
		d, err := ParseDemandeCapacite(Parametres{
			Mensualite: "2000.000", Taux: "8.5", Mois: "240", Differe: differe,
		})
		if err != nil {
			t.Fatalf("ParseDemandeCapacite : %v", err)
		}
		c, err := moteur.Capaciter(context.Background(), d)
		if err != nil {
			t.Fatalf("Capaciter : %v", err)
		}
		return millimes(t, c.Capital)
	}

	sans, avec := capital(""), capital("24")
	if avec >= sans {
		t.Errorf("capacite avec differe %d, sans differe %d : elle devrait diminuer",
			avec, sans)
	}
}

// Solder par anticipation economise les interets restants, moins l'indemnite.
func TestRemboursementAnticipe(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)
	ctx := context.Background()

	calculer := func(mois, indemnite string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240",
			MoisAnticipe: mois, Indemnite: indemnite,
		})
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(ctx, d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	// Sans demande, aucun bloc de remboursement anticipe.
	if calculer("", "").Anticipe != nil {
		t.Error("aucun remboursement anticipe ne devrait etre rendu sans demande")
	}

	res := calculer("120", "1")
	a := res.Anticipe
	if a == nil {
		t.Fatal("le remboursement anticipe devrait etre rendu")
	}
	if a.Mois != 120 {
		t.Errorf("mois %d, attendu 120", a.Mois)
	}
	// Le solde annonce doit etre celui de l'echeancier au meme mois.
	if a.SoldeRestant.String() != res.Echeancier[119].Solde.String() {
		t.Errorf("solde %s, echeancier %s", a.SoldeRestant, res.Echeancier[119].Solde)
	}
	// L'indemnite est un pourcentage du capital solde.
	solde, indem := millimes(t, a.SoldeRestant), millimes(t, a.Indemnite)
	if attendu := solde / 100; indem < attendu-1 || indem > attendu+1 {
		t.Errorf("indemnite %d, attendu environ %d (1 %% de %d)", indem, attendu, solde)
	}
	// Le total au terme doit etre celui du recapitulatif.
	if a.TotalTerme.String() != res.Recapitulatif.TotalVerse.String() {
		t.Errorf("total au terme %s, recapitulatif %s", a.TotalTerme, res.Recapitulatif.TotalVerse)
	}
	// L'economie est la difference entre les deux totaux.
	if got, veut := millimes(t, a.Economie),
		millimes(t, a.TotalTerme)-millimes(t, a.TotalAnticipe); got != veut {
		t.Errorf("economie %d, attendu %d", got, veut)
	}

	// Une indemnite plus forte reduit l'economie.
	sans, avec := calculer("120", "0").Anticipe, calculer("120", "1.5").Anticipe
	if millimes(t, avec.Economie) >= millimes(t, sans.Economie) {
		t.Error("une indemnite plus forte devrait reduire l'economie")
	}
	// Sans indemnite, l'economie egale les interets economises.
	if sans.Economie.String() != sans.InteretsEconomises.String() {
		t.Errorf("sans indemnite : economie %s, interets economises %s",
			sans.Economie, sans.InteretsEconomises)
	}
	// Plus on solde tot, plus on economise.
	if millimes(t, calculer("60", "1").Anticipe.Economie) <=
		millimes(t, calculer("180", "1").Anticipe.Economie) {
		t.Error("solder plus tot devrait economiser davantage")
	}
}

// Solder a la derniere echeance revient a aller au terme.
func TestSolderALaDerniereEcheanceEquivautAuTerme(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

	d, err := ParseDemande(Parametres{
		Capital: "250000.000", Taux: "8.5", Mois: "240", MoisAnticipe: "240",
	})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}
	a := res.Anticipe
	if a == nil {
		t.Fatal("bloc absent")
	}
	if millimes(t, a.SoldeRestant) != 0 {
		t.Errorf("solde %s, attendu nul a la derniere echeance", a.SoldeRestant)
	}
	if millimes(t, a.Economie) != 0 {
		t.Errorf("economie %s, attendue nulle", a.Economie)
	}
	if a.TotalAnticipe.String() != a.TotalTerme.String() {
		t.Errorf("total anticipe %s, total au terme %s", a.TotalAnticipe, a.TotalTerme)
	}
}

// Une indemnite assez forte peut rendre l'operation perdante : l'economie est
// alors nulle, jamais negative.
func TestUneIndemniteExcessiveAnnuleLEconomie(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

	d, err := ParseDemande(Parametres{
		Capital: "250000.000", Taux: "8.5", Mois: "240",
		MoisAnticipe: "230", Indemnite: "50",
	})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}
	if millimes(t, res.Anticipe.Economie) != 0 {
		t.Errorf("economie %s, attendue nulle quand l'indemnite depasse le gain",
			res.Anticipe.Economie)
	}
}

func TestParseDemandeValideLeRemboursementAnticipe(t *testing.T) {
	cas := []struct{ nom, mois, indemnite, champ string }{
		{"mois nul", "0", "", "mois_remboursement_anticipe"},
		{"mois au-dela de la duree", "241", "", "mois_remboursement_anticipe"},
		{"mois non entier", "abc", "", "mois_remboursement_anticipe"},
		{"indemnite hors bornes", "120", "100", "indemnite"},
		{"indemnite non numerique", "120", "abc", "indemnite"},
		// Une indemnite sans mois n'a pas d'objet : mieux vaut le dire que
		// l'ignorer en silence.
		{"indemnite sans mois", "", "1", "indemnite"},
	}
	for _, c := range cas {
		_, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240",
			MoisAnticipe: c.mois, Indemnite: c.indemnite,
		})
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

// Rembourser une part du capital, puis raccourcir la duree ou alleger
// l'echeance. Les valeurs sont celles d'un modele decimal independant.
func TestRemboursementPartiel(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)
	ctx := context.Background()

	calculer := func(methode, mode, montant string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240", Methode: methode,
			MoisAnticipe: "120", Indemnite: "1",
			MontantAnticipe: montant, Mode: mode,
		})
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(ctx, d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		if res.Partiel == nil {
			t.Fatal("aucun bloc de remboursement partiel")
		}
		return res
	}

	cas := []struct {
		methode, mode    string
		duree            int
		echeanceSuivante string
		total            string
		totalTerme       string
		economie         string
		interetsEconomes string
	}{
		{"annuite_constante", "duree_reduite", 195, "2169.558",
			"472019.531", "520693.976", "48674.445", "49174.445"},
		{"annuite_constante", "echeance_reduite", 240, "1549.630",
			"496802.544", "520693.976", "23891.432", "24391.432"},
		{"capital_constant", "duree_reduite", 192, "1572.917",
			"429708.289", "463385.350", "33677.061", "34177.061"},
		{"capital_constant", "echeance_reduite", 240, "1156.250",
			"442458.282", "463385.350", "20927.068", "21427.068"},
		{"in_fine", "echeance_reduite", 240, "1416.667",
			"633000.000", "674999.920", "41999.920", "42499.920"},
	}

	for _, c := range cas {
		t.Run(c.methode+"/"+c.mode, func(t *testing.T) {
			res := calculer(c.methode, c.mode, "50000.000")
			p := res.Partiel

			if p.Mois != 120 {
				t.Errorf("mois %d, attendu 120", p.Mois)
			}
			if p.Mode != c.mode {
				t.Errorf("mode %q, attendu %q", p.Mode, c.mode)
			}
			if p.Montant.String() != "50000.000" {
				t.Errorf("montant %s, attendu 50000.000", p.Montant)
			}
			// L'indemnite porte sur le capital rembourse, non sur le solde.
			if p.Indemnite.String() != "500.000" {
				t.Errorf("indemnite %s, attendu 500.000", p.Indemnite)
			}
			if p.Duree != c.duree {
				t.Errorf("duree %d, attendu %d", p.Duree, c.duree)
			}
			if p.EcheanceSuivante.String() != c.echeanceSuivante {
				t.Errorf("echeance suivante %s, attendu %s",
					p.EcheanceSuivante, c.echeanceSuivante)
			}
			for _, v := range []struct {
				nom     string
				got     interface{ String() string }
				attendu string
			}{
				{"total", p.Total, c.total},
				{"total au terme", p.TotalTerme, c.totalTerme},
				{"economie", p.Economie, c.economie},
				{"interets economises", p.InteretsEconomises, c.interetsEconomes},
			} {
				if v.got.String() != v.attendu {
					t.Errorf("%s %s, attendu %s", v.nom, v.got, v.attendu)
				}
			}

			// L'economie est la difference des deux totaux.
			if got, veut := millimes(t, p.Economie),
				millimes(t, p.TotalTerme)-millimes(t, p.Total); got != veut {
				t.Errorf("economie %d, attendu %d", got, veut)
			}
			// Elle vaut les interets economises moins l'indemnite.
			if got, veut := millimes(t, p.Economie),
				millimes(t, p.InteretsEconomises)-
					millimes(t, p.Indemnite); got != veut {
				t.Errorf("economie %d, interets moins indemnite %d", got, veut)
			}

			// L'echeancier rendu reste celui du contrat : le remboursement
			// anticipe est simule, il ne le reecrit pas.
			if len(res.Echeancier) != 240 {
				t.Errorf("echeancier de %d lignes, attendu 240 : le contrat",
					len(res.Echeancier))
			}
			if res.Anticipe != nil {
				t.Error("un remboursement partiel n'est pas un solde total")
			}
		})
	}

	// Raccourcir la duree laisse l'echeance intacte ; l'alleger laisse la
	// duree intacte. Les deux modes s'opposent terme a terme.
	reduite := calculer("annuite_constante", "duree_reduite", "50000.000")
	allegee := calculer("annuite_constante", "echeance_reduite", "50000.000")
	if reduite.Partiel.Duree >= allegee.Partiel.Duree {
		t.Error("la duree reduite devrait raccourcir le pret")
	}
	if millimes(t, allegee.Partiel.EcheanceSuivante) >=
		millimes(t, reduite.Partiel.EcheanceSuivante) {
		t.Error("l'echeance reduite devrait alleger le versement")
	}
	// A montant egal, raccourcir la duree economise davantage : le capital
	// cesse plus tot de porter interet.
	if millimes(t, reduite.Partiel.Economie) <=
		millimes(t, allegee.Partiel.Economie) {
		t.Error("la duree reduite devrait economiser davantage")
	}
}

// Rembourser par anticipation la totalite du capital restant du revient au
// solde total : les deux chemins doivent donner le meme cout.
func TestRembourserToutLeSoldeEquivautAuSoldeTotal(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)
	ctx := context.Background()

	calculer := func(p Parametres) *Echeancier {
		t.Helper()
		d, err := ParseDemande(p)
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(ctx, d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	base := Parametres{Capital: "250000.000", Taux: "8.5", Mois: "240",
		MoisAnticipe: "120"}

	total := calculer(base).Anticipe
	if total == nil {
		t.Fatal("aucun bloc de solde total")
	}

	partiel := calculer(Parametres{Capital: "250000.000", Taux: "8.5",
		Mois: "240", MoisAnticipe: "120", Mode: "duree_reduite",
		MontantAnticipe: total.SoldeRestant.String()}).Partiel
	if partiel == nil {
		t.Fatal("aucun bloc de remboursement partiel")
	}

	if partiel.Total.String() != total.TotalAnticipe.String() {
		t.Errorf("total partiel %s, total du solde %s",
			partiel.Total, total.TotalAnticipe)
	}
	if partiel.Economie.String() != total.Economie.String() {
		t.Errorf("economie partielle %s, economie du solde %s",
			partiel.Economie, total.Economie)
	}
	// Le pret s'arrete au mois du remboursement, et rien n'est du ensuite.
	if partiel.Duree != 120 {
		t.Errorf("duree %d, attendu 120", partiel.Duree)
	}
	if millimes(t, partiel.EcheanceSuivante) != 0 {
		t.Errorf("echeance suivante %s, attendue nulle", partiel.EcheanceSuivante)
	}
}

func TestParseDemandeValideLeRemboursementPartiel(t *testing.T) {
	cas := []struct {
		nom, methode, mois, mode, montant, champ string
	}{
		// Solder la totalite ne laisse rien a rembourser partiellement,
		// et reciproquement : les deux operations s'excluent.
		{"montant avec le mode total", "", "120", "total", "50000.000",
			"montant_remboursement_anticipe"},
		{"mode partiel sans montant", "", "120", "duree_reduite", "",
			"montant_remboursement_anticipe"},
		{"montant nul", "", "120", "duree_reduite", "0",
			"montant_remboursement_anticipe"},
		{"montant non numerique", "", "120", "duree_reduite", "abc",
			"montant_remboursement_anticipe"},
		// Il doit rester une echeance apres l'operation.
		{"partiel a la derniere echeance", "", "240", "duree_reduite",
			"1000.000", "mois_remboursement_anticipe"},
		// Sans amortissement avant le terme, il n'y a pas de duree a
		// raccourcir.
		{"duree reduite in fine", "in_fine", "120", "duree_reduite",
			"50000.000", "mode_remboursement_anticipe"},
		{"mode inconnu", "", "120", "moitie_moitie", "50000.000",
			"mode_remboursement_anticipe"},
		// Preciser un mode ou un montant sans mois est une erreur de saisie
		// qu'il vaut mieux signaler que taire.
		{"mode sans mois", "", "", "duree_reduite", "",
			"mode_remboursement_anticipe"},
		{"montant sans mois", "", "", "", "50000.000",
			"montant_remboursement_anticipe"},
	}
	for _, c := range cas {
		_, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240",
			Methode: c.methode, MoisAnticipe: c.mois,
			Mode: c.mode, MontantAnticipe: c.montant,
		})
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

// Le programme COBOL refuse lui aussi les combinaisons incoherentes : la
// validation Go n'est pas sa seule protection.
func TestLeProgrammeRefuseUnMontantSuperieurAuSolde(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)

	d, err := ParseDemande(Parametres{
		Capital: "250000.000", Taux: "8.5", Mois: "240",
		MoisAnticipe: "120", Mode: "duree_reduite",
		MontantAnticipe: "200000.000",
	})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	// Le solde au mois 120 vaut 174984.575 : en rembourser 200000 n'a pas
	// de sens, et seul le deroulement de l'echeancier peut le savoir.
	if _, err := moteur.Calculer(context.Background(), d); err == nil {
		t.Error("un montant superieur au capital restant aurait du echouer")
	}
}

// Le code de retour distingue un refus de saisie d'un echec technique. Le
// confondre rendrait un 500 la ou le client a simplement mal saisi, ou pire,
// un 400 rassurant sur un moteur en panne.
func TestSeulLeCodeDeRefusDonneUneErreurDeSaisie(t *testing.T) {
	cas := []struct {
		code   int
		motif  string
		refuse bool
	}{
		{codeRefus, "le montant rembourse depasse le capital restant", true},
		{2, "entree malformee", false},
		{3, "capital et duree doivent etre non nuls", false},
		{1, "", false},
	}

	for _, c := range cas {
		script := filepath.Join(t.TempDir(), "faux.sh")
		contenu := fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q >&2\nexit %d\n",
			c.motif, c.code)
		if err := os.WriteFile(script, []byte(contenu), 0o700); err != nil {
			t.Fatalf("ecriture du script : %v", err)
		}

		d, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240"})
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}

		_, err = NewMoteur(script, "", 0).Calculer(context.Background(), d)
		if err == nil {
			t.Errorf("code %d : aurait du echouer", c.code)
			continue
		}

		var refus *ErreurRefus
		estRefus := errors.As(err, &refus)
		if estRefus != c.refuse {
			t.Errorf("code %d : refus de saisie %v, attendu %v (erreur %v)",
				c.code, estRefus, c.refuse, err)
			continue
		}
		if c.refuse {
			if refus.Motif != c.motif {
				t.Errorf("code %d : motif %q, attendu %q",
					c.code, refus.Motif, c.motif)
			}
			// Le refus doit rester reconnaissable par errors.Is.
			if !errors.Is(err, ErrDemandeRefusee) {
				t.Errorf("code %d : errors.Is(ErrDemandeRefusee) est faux", c.code)
			}
		} else if !errors.Is(err, ErrProgramme) {
			t.Errorf("code %d : erreur %v, attendu ErrProgramme", c.code, err)
		}
	}
}

// Sous franchise totale, rien n'est verse pendant le differe et les interets
// grossissent le capital. C'est la difference avec la franchise partielle, ou
// les interets sont dus chaque mois et le capital reste intact.
func TestDiffereTotal(t *testing.T) {
	moteur := NewMoteur(binaire(t), binaireCapacite(t), 0)
	ctx := context.Background()

	calculer := func(typeDiffere string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240",
			Differe: "24", TypeDiffere: typeDiffere,
		})
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(ctx, d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	total := calculer("total")

	// Valeurs d'un modele decimal independant.
	if got := total.Recapitulatif.InteretsCapitalises.String(); got != "46148.691" {
		t.Errorf("interets capitalises %s, attendu 46148.691", got)
	}
	if got := total.Recapitulatif.TotalInterets.String(); got != "329204.195" {
		t.Errorf("total interets %s, attendu 329204.195", got)
	}
	if got := total.Recapitulatif.DerniereMensualite.String(); got != "2681.695" {
		t.Errorf("derniere mensualite %s, attendu 2681.695", got)
	}

	// Pendant la franchise totale, rien n'est du et le solde grossit.
	for _, n := range []int{1, 12, 24} {
		e := total.Echeancier[n-1]
		if millimes(t, e.Mensualite) != 0 {
			t.Errorf("mois %d : mensualite %s, attendue nulle", n, e.Mensualite)
		}
		if millimes(t, e.Capitalise) != millimes(t, e.Interets) {
			t.Errorf("mois %d : capitalise %s, interets %s : tout l'interet "+
				"devrait etre capitalise", n, e.Capitalise, e.Interets)
		}
	}
	if millimes(t, total.Echeancier[23].Solde) <=
		millimes(t, total.Echeancier[0].Solde) {
		t.Error("le solde devrait grossir pendant la franchise totale")
	}
	// Le capital amorti est le capital emprunte grossi des interets.
	if got, veut := millimes(t, total.Echeancier[23].Solde),
		int64(250000000)+millimes(t, total.Recapitulatif.InteretsCapitalises); got != veut {
		t.Errorf("solde en fin de franchise %d, attendu %d", got, veut)
	}
	// Passe la franchise, plus rien n'est capitalise.
	for _, e := range total.Echeancier[24:] {
		if millimes(t, e.Capitalise) != 0 {
			t.Errorf("mois %d : capitalise %s hors franchise", e.N, e.Capitalise)
		}
	}

	// La franchise partielle ne capitalise rien, et coute donc moins cher.
	partiel := calculer("partiel")
	if got := millimes(t, partiel.Recapitulatif.InteretsCapitalises); got != 0 {
		t.Errorf("franchise partielle : capitalise %d, attendu 0", got)
	}
	if millimes(t, partiel.Recapitulatif.TotalInterets) >=
		millimes(t, total.Recapitulatif.TotalInterets) {
		t.Error("la franchise totale devrait couter plus cher en interets")
	}
	// Sous franchise partielle, les interets sont dus des le premier mois.
	if millimes(t, partiel.Echeancier[0].Mensualite) == 0 {
		t.Error("la franchise partielle devrait laisser les interets dus")
	}

	// Les invariants tiennent sous les deux franchises, et sur les trois
	// methodes : l'invariant 1 tient compte de la capitalisation.
	for _, methode := range MethodesAcceptees() {
		for _, typ := range TypesDiffereAcceptes() {
			t.Run(methode+"/"+typ, func(t *testing.T) {
				verifierInvariants(t, moteur, Parametres{
					Capital: "250000.000", Taux: "8.5", Mois: "240",
					Methode: methode, Differe: "24", TypeDiffere: typ,
					TauxAssurance: "0.36", Assiette: "capital_restant_du",
				})
			})
		}
	}
}

func TestParseDemandeValideLeTypeDeDiffere(t *testing.T) {
	cas := []struct{ nom, differe, typ, champ string }{
		{"type inconnu", "24", "moitie", "type_differe"},
		// Qualifier une franchise qui n'existe pas est une erreur de saisie.
		{"type sans differe", "", "total", "type_differe"},
		{"type avec un differe nul", "0", "total", "type_differe"},
	}
	for _, c := range cas {
		_, err := ParseDemande(Parametres{
			Capital: "250000.000", Taux: "8.5", Mois: "240",
			Differe: c.differe, TypeDiffere: c.typ,
		})
		if err == nil {
			t.Errorf("%s : aurait du echouer", c.nom)
			continue
		}
		invalide, ok := err.(*ErreurValidation)
		if !ok || invalide.Champ != c.champ {
			t.Errorf("%s : erreur %v, champ attendu %q", c.nom, err, c.champ)
		}
	}

	// Sans differe, le type reste absent de la reponse plutot que d'annoncer
	// une franchise qui n'existe pas.
	d, err := ParseDemande(Parametres{Capital: "250000.000", Taux: "8.5", Mois: "240"})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	if d.TypeDiffere != nil {
		t.Errorf("type_differe %q rendu sans differe", *d.TypeDiffere)
	}
	if d.CodeTypeDiffere != 'P' {
		t.Errorf("lettre %q, attendu 'P' : le programme COBOL en exige une",
			d.CodeTypeDiffere)
	}
}
