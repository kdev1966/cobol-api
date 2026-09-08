package loan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
// aux memes positions que celles produites par le programme COBOL.
func recap(echeances int, premiere, derniere, interets, total string) string {
	return fmt.Sprintf("R%04d%013s%013s%015s%015s%06d",
		echeances, premiere, derniere, interets, total, 35051)
}

func echeance(n int, paiement, interets, capital, solde string) string {
	return fmt.Sprintf("E%04d%013s%013s%013s%013s", n, paiement, interets, capital, solde)
}

func TestLireSortieEcarteLesLignesParasites(t *testing.T) {
	// libcob peut ecrire un avertissement sur la sortie standard ; il ne
	// doit pas faire echouer le calcul.
	sortie := bytes.NewBufferString(strings.Join([]string{
		"libcob: warning: implicit CLOSE of SYSIN",
		recap(2, "0000000010000", "0000000010500", "000000000000500", "000000000020500"),
		echeance(1, "0000000010000", "0000000000300", "0000000009700", "0000000010300"),
		"",
		echeance(2, "0000000010500", "0000000000200", "0000000010300", "0000000000000"),
		"note de fin sans structure",
	}, "\n"))

	res, err := lireSortie(sortie)
	if err != nil {
		t.Fatalf("lireSortie : %v", err)
	}
	if len(res.Echeancier) != 2 {
		t.Fatalf("%d echeances, attendu 2", len(res.Echeancier))
	}
	if got := res.Recapitulatif.PremiereEcheance.String(); got != "100.00" {
		t.Errorf("premiere echeance %q, attendu \"100.00\"", got)
	}
	if got := res.Recapitulatif.DerniereEcheance.String(); got != "105.00" {
		t.Errorf("derniere echeance %q, attendu \"105.00\"", got)
	}
	if got := res.Echeancier[1].Solde.String(); got != "0.00" {
		t.Errorf("solde final %q, attendu \"0.00\"", got)
	}
}

func TestLireSortieRefuseUnComptageIncoherent(t *testing.T) {
	sortie := bytes.NewBufferString(strings.Join([]string{
		recap(5, "0000000010000", "0000000010500", "000000000000500", "000000000020500"),
		echeance(1, "0000000010000", "0000000000300", "0000000009700", "0000000000000"),
	}, "\n"))

	if _, err := lireSortie(sortie); err == nil {
		t.Fatal("un echeancier tronque aurait du etre refuse")
	}
}

func TestLireSortieRefuseUnRecapitulatifEnDouble(t *testing.T) {
	ligne := recap(1, "0000000010000", "0000000010000", "000000000000500", "000000000020500")
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
	// 0.07 n'a pas de representation binaire exacte, et 8388608.01 depasse
	// la precision d'un float32. Les deux doivent ressortir tels quels.
	sortie := bytes.NewBufferString(strings.Join([]string{
		recap(1, "0000000000007", "0000000000007", "000000000000007", "000000000000007"),
		echeance(1, "0838860801000", "0000000000007", "0000000000029", "0000000000000"),
	}, "\n"))

	res, err := lireSortie(sortie)
	if err != nil {
		t.Fatalf("lireSortie : %v", err)
	}
	if got := res.Recapitulatif.PremiereEcheance.String(); got != "0.07" {
		t.Errorf("premiere echeance %q, attendu \"0.07\"", got)
	}
	if got := res.Echeancier[0].Paiement.String(); got != "8388608010.00" {
		t.Errorf("paiement %q, attendu \"8388608010.00\"", got)
	}
}

func TestNombreInsereLePointDecimal(t *testing.T) {
	cas := []struct {
		chiffres  string
		decimales int
		attendu   string
	}{
		{"0000000144348", 2, "1443.48"},
		{"0000000000007", 2, "0.07"},
		{"0000000000000", 2, "0.00"},
		{"9999999999999", 2, "99999999999.99"},
		{"000", 2, "0.00"},
		// Le TAEG est rendu a quatre decimales.
		{"035051", 4, "3.5051"},
		{"000000", 4, "0.0000"},
		{"999999", 4, "99.9999"},
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

// centimes convertit un montant rendu par le COBOL en entier, pour totaliser
// sans flottant.
func centimes(t *testing.T, n interface{ String() string }) int64 {
	t.Helper()
	v, err := decimalVersEntier(n.String(), 2)
	if err != nil {
		t.Fatalf("montant %q illisible : %v", n.String(), err)
	}
	return v
}

func TestInvariantsDeLEcheancier(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	cas := []struct{ capital, taux, mois string }{
		{"250000.00", "3.45", "240"},
		{"100000.00", "1", "120"},
		{"500000.00", "5.9", "300"},
		{"10000.00", "0", "12"},   // taux nul : la formule diviserait par zero
		{"1234.56", "7.25", "37"}, // montants et duree non ronds
		{"999999.99", "12.5", "360"},
		{"1000.00", "4", "1"},       // duree minimale
		{"50000.00", "0.01", "600"}, // duree maximale
		{"0.01", "3.45", "12"},      // capital minimal
	}

	// Les quatre invariants valent pour les trois methodes.
	for _, methode := range MethodesAcceptees() {
		for _, c := range cas {
			nom := methode + "/" + c.capital + "@" + c.taux + "%x" + c.mois
			t.Run(nom, func(t *testing.T) {
				verifierInvariants(t, moteur, c.capital, c.taux, c.mois, methode)
			})
		}
	}
}

func verifierInvariants(t *testing.T, moteur *Moteur, capital, taux, mois, methode string) {
	t.Helper()

	d, err := ParseDemande(capital, taux, mois, methode)
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}

	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	var sommeCapital, sommeInterets, sommeEcheances int64
	for _, e := range res.Echeancier {
		p := centimes(t, e.Paiement)
		i := centimes(t, e.Interets)
		k := centimes(t, e.Capital)

		// Invariant 4 : chaque ligne s'equilibre.
		if p != i+k {
			t.Errorf("echeance %d : paiement %d != interets %d + capital %d", e.N, p, i, k)
		}
		sommeCapital += k
		sommeInterets += i
		sommeEcheances += p
	}

	// Invariant 1 : la somme des parts de capital egale le capital emprunte.
	if sommeCapital != d.CapitalCentimes {
		t.Errorf("somme des parts de capital %d, capital emprunte %d",
			sommeCapital, d.CapitalCentimes)
	}
	// Invariant 2 : les echeances valent le capital plus les interets.
	if sommeEcheances != d.CapitalCentimes+sommeInterets {
		t.Errorf("somme des echeances %d, attendu %d",
			sommeEcheances, d.CapitalCentimes+sommeInterets)
	}
	// Invariant 3 : le solde final est nul.
	if solde := centimes(t, res.Echeancier[len(res.Echeancier)-1].Solde); solde != 0 {
		t.Errorf("solde final %d, attendu 0", solde)
	}

	// Le recapitulatif doit concorder avec le detail.
	if len(res.Echeancier) != d.Mois {
		t.Errorf("%d echeances, attendu %d", len(res.Echeancier), d.Mois)
	}
	if got := centimes(t, res.Recapitulatif.TotalInterets); got != sommeInterets {
		t.Errorf("total_interets %d, detail %d", got, sommeInterets)
	}
	if got := centimes(t, res.Recapitulatif.TotalDu); got != sommeEcheances {
		t.Errorf("total_du %d, detail %d", got, sommeEcheances)
	}
	if got, veut := centimes(t, res.Recapitulatif.PremiereEcheance),
		centimes(t, res.Echeancier[0].Paiement); got != veut {
		t.Errorf("premiere_echeance %d, detail %d", got, veut)
	}
	if got, veut := centimes(t, res.Recapitulatif.DerniereEcheance),
		centimes(t, res.Echeancier[len(res.Echeancier)-1].Paiement); got != veut {
		t.Errorf("derniere_echeance %d, detail %d", got, veut)
	}
}

// Chaque methode a une signature propre, verifiee en plus des invariants
// communs.
func TestSignatureDeChaqueMethode(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	calculer := func(methode string) *Echeancier {
		t.Helper()
		d, err := ParseDemande("120000.00", "4.2", "60", methode)
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
	ref := annuite.Echeancier[0].Paiement.String()
	for _, e := range annuite.Echeancier[:len(annuite.Echeancier)-1] {
		if e.Paiement.String() != ref {
			t.Fatalf("annuite constante : echeance %d vaut %s, attendu %s", e.N, e.Paiement, ref)
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
	if centimes(t, lineaire.Echeancier[0].Paiement) <=
		centimes(t, lineaire.Echeancier[len(lineaire.Echeancier)-1].Paiement) {
		t.Error("capital constant : l'echeance devrait decroitre")
	}

	// In fine : aucun capital rembourse avant la derniere echeance.
	inFine := calculer("in_fine")
	for _, e := range inFine.Echeancier[:len(inFine.Echeancier)-1] {
		if centimes(t, e.Capital) != 0 {
			t.Fatalf("in fine : capital %s rembourse a l'echeance %d", e.Capital, e.N)
		}
	}
	if got := inFine.Echeancier[len(inFine.Echeancier)-1].Capital.String(); got != "120000.00" {
		t.Errorf("in fine : derniere part de capital %s, attendu 120000.00", got)
	}

	// Plus l'amortissement est rapide, moins on paie d'interets.
	iLin := centimes(t, lineaire.Recapitulatif.TotalInterets)
	iAnn := centimes(t, annuite.Recapitulatif.TotalInterets)
	iFin := centimes(t, inFine.Recapitulatif.TotalInterets)
	if !(iLin < iAnn && iAnn < iFin) {
		t.Errorf("interets attendus capital_constant < annuite < in_fine, recu %d %d %d",
			iLin, iAnn, iFin)
	}
}

// Cas de reference verifie independamment avec le module decimal de Python.
func TestCasDeReference(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	d, err := ParseDemande("250000.00", "3.45", "240", "")
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	attendus := map[string]string{
		"mensualite":     res.Recapitulatif.PremiereEcheance.String(),
		"total_interets": res.Recapitulatif.TotalInterets.String(),
		"total_du":       res.Recapitulatif.TotalDu.String(),
	}
	attendus["taeg"] = res.Recapitulatif.Taeg.String()
	references := map[string]string{
		"mensualite":     "1443.48",
		"total_interets": "96436.65",
		"total_du":       "346436.65",
		// Dichotomie independante en Decimal Python sur les memes echeances :
		// 3.505078595213301... soit 3.5051 a quatre decimales.
		"taeg": "3.5051",
	}
	for cle, ref := range references {
		if attendus[cle] != ref {
			t.Errorf("%s = %s, reference %s", cle, attendus[cle], ref)
		}
	}

	premiere := res.Echeancier[0]
	if premiere.Interets.String() != "718.75" || premiere.Capital.String() != "724.73" {
		t.Errorf("premiere echeance : interets %s capital %s, attendu 718.75 / 724.73",
			premiere.Interets, premiere.Capital)
	}
	// La derniere echeance absorbe le residu d'arrondi : elle differe de la
	// mensualite nominale.
	derniere := res.Echeancier[len(res.Echeancier)-1]
	if derniere.Paiement.String() != "1444.93" {
		t.Errorf("derniere echeance %s, attendu 1444.93", derniere.Paiement)
	}
}

// Sans frais, le TAEG ne depend que du taux nominal : il capitalise le taux
// periodique sur douze mois, quelle que soit la maniere dont le capital est
// amorti. C'est une propriete du domaine, verifiee ici comme telle.
func TestLeTaegNeDependPasDeLaMethode(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	var reference string
	for _, methode := range MethodesAcceptees() {
		d, err := ParseDemande("120000.00", "4.2", "60", methode)
		if err != nil {
			t.Fatalf("ParseDemande : %v", err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}

		got := res.Recapitulatif.Taeg.String()
		if reference == "" {
			reference = got
			// (1 + 0.042/12)^12 - 1 = 4.2818 %
			if got != "4.2818" {
				t.Errorf("taeg %s, attendu 4.2818", got)
			}
			continue
		}
		if got != reference {
			t.Errorf("methode %s : taeg %s, attendu %s", methode, got, reference)
		}
	}
}

func TestLeTaegEstNulSansInterets(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	d, err := ParseDemande("10000.00", "0", "12", "")
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}
	if got := res.Recapitulatif.Taeg.String(); got != "0.0000" {
		t.Errorf("taeg %s, attendu 0.0000", got)
	}
}

func TestCalculerRemonteLEchecDuProgramme(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	// Capital nul : le programme COBOL doit refuser et sortir en erreur.
	_, err := moteur.Calculer(context.Background(), Demande{
		CapitalCentimes: 0, TauxMillioniemes: 3450000, Mois: 12, CodeMethode: 'A',
	})
	if err == nil {
		t.Fatal("un capital nul aurait du faire echouer le programme")
	}
}

func TestCalculerAvecUnBinaireIntrouvable(t *testing.T) {
	moteur := NewMoteur("/inexistant/loan_amortization")
	d, _ := ParseDemande("1000.00", "3.45", "12", "")

	if _, err := moteur.Calculer(context.Background(), d); err == nil {
		t.Fatal("un binaire absent aurait du produire une erreur")
	}
}
