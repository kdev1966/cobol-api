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
// aux memes positions que celles produites par le programme COBOL.
func recap(echeances int, premiere, derniere, interets, total string) string {
	// assurance, frais et cout sont a zero : les tests de lecture portent sur
	// le decoupage, pas sur le calcul.
	// Ni assurance, ni frais, ni verification d'usure : les tests de lecture
	// portent sur le decoupage, pas sur le calcul.
	return fmt.Sprintf("R%04d%013s%013s%015s%015s%013s%015s%015s%06d%06d%s%s",
		echeances, premiere, derniere, interets,
		"000000000000000", "0000000000000", total, interets, 35051,
		0, "-", "+000000")
}

func echeance(n int, mensualite, interets, capital, solde string) string {
	return fmt.Sprintf("E%04d%013s%013s%013s%013s%013s%013s",
		n, mensualite, interets, capital, "0000000000000", mensualite, solde)
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
	if got := res.Recapitulatif.PremiereMensualite.String(); got != "100.00" {
		t.Errorf("premiere mensualite %q, attendu \"100.00\"", got)
	}
	if got := res.Recapitulatif.DerniereMensualite.String(); got != "105.00" {
		t.Errorf("derniere mensualite %q, attendu \"105.00\"", got)
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
	if got := res.Recapitulatif.PremiereMensualite.String(); got != "0.07" {
		t.Errorf("premiere mensualite %q, attendu \"0.07\"", got)
	}
	if got := res.Echeancier[0].Mensualite.String(); got != "8388608010.00" {
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
	moteur := NewMoteur(binaire(t), 0)

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
		p := centimes(t, e.Echeance)
		i := centimes(t, e.Interets)
		k := centimes(t, e.Capital)

		// Invariant 4 : la part de credit s'equilibre.
		if p != i+k {
			t.Errorf("echeance %d : echeance %d != interets %d + capital %d", e.N, p, i, k)
		}
		// Invariant 5 : la mensualite est l'echeance plus l'assurance.
		a := centimes(t, e.Assurance)
		if m := centimes(t, e.Mensualite); m != p+a {
			t.Errorf("echeance %d : mensualite %d != echeance %d + assurance %d",
				e.N, m, p, a)
		}
		sommeCapital += k
		sommeInterets += i
		sommeEcheances += p
		sommeAssurance += a
		sommeMensualites += centimes(t, e.Mensualite)
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
	// Invariant 6 : le total verse est la somme des mensualites.
	if got := centimes(t, res.Recapitulatif.TotalVerse); got != sommeMensualites {
		t.Errorf("total_verse %d, detail %d", got, sommeMensualites)
	}
	if got := centimes(t, res.Recapitulatif.TotalAssurance); got != sommeAssurance {
		t.Errorf("total_assurance %d, detail %d", got, sommeAssurance)
	}
	// Le cout du credit agrege interets, assurance et frais.
	frais := d.FraisDossierCentimes + d.FraisGarantieCentimes
	if got := centimes(t, res.Recapitulatif.CoutCredit); got != sommeInterets+sommeAssurance+frais {
		t.Errorf("cout_credit %d, attendu %d", got, sommeInterets+sommeAssurance+frais)
	}
	if got := centimes(t, res.Recapitulatif.TotalFrais); got != frais {
		t.Errorf("total_frais %d, attendu %d", got, frais)
	}
	if got, veut := centimes(t, res.Recapitulatif.PremiereMensualite),
		centimes(t, res.Echeancier[0].Mensualite); got != veut {
		t.Errorf("premiere_mensualite %d, detail %d", got, veut)
	}
	if got, veut := centimes(t, res.Recapitulatif.DerniereMensualite),
		centimes(t, res.Echeancier[len(res.Echeancier)-1].Mensualite); got != veut {
		t.Errorf("derniere_mensualite %d, detail %d", got, veut)
	}
}

// Chaque methode a une signature propre, verifiee en plus des invariants
// communs.
func TestSignatureDeChaqueMethode(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	calculer := func(methode string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{Capital: "120000.00", Taux: "4.2", Mois: "60", Methode: methode})
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
	if centimes(t, lineaire.Echeancier[0].Mensualite) <=
		centimes(t, lineaire.Echeancier[len(lineaire.Echeancier)-1].Mensualite) {
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
	if iLin >= iAnn || iAnn >= iFin {
		t.Errorf("interets attendus capital_constant < annuite < in_fine, recu %d %d %d",
			iLin, iAnn, iFin)
	}
}

// Cas de reference verifie independamment avec le module decimal de Python.
func TestCasDeReference(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	d, err := ParseDemande(Parametres{Capital: "250000.00", Taux: "3.45", Mois: "240", Methode: ""})
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	attendus := map[string]string{
		"mensualite":     res.Recapitulatif.PremiereMensualite.String(),
		"total_interets": res.Recapitulatif.TotalInterets.String(),
		"total_verse":    res.Recapitulatif.TotalVerse.String(),
	}
	attendus["taeg"] = res.Recapitulatif.Taeg.String()
	references := map[string]string{
		"mensualite":     "1443.48",
		"total_interets": "96436.65",
		"total_verse":    "346436.65",
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
	if derniere.Mensualite.String() != "1444.93" {
		t.Errorf("derniere echeance %s, attendu 1444.93", derniere.Mensualite)
	}
}

// Sans frais, le TAEG ne depend que du taux nominal : il capitalise le taux
// periodique sur douze mois, quelle que soit la maniere dont le capital est
// amorti. C'est une propriete du domaine, verifiee ici comme telle.
func TestLeTaegNeDependPasDeLaMethode(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	var reference string
	for _, methode := range MethodesAcceptees() {
		d, err := ParseDemande(Parametres{Capital: "120000.00", Taux: "4.2", Mois: "60", Methode: methode})
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
	moteur := NewMoteur(binaire(t), 0)

	d, err := ParseDemande(Parametres{Capital: "10000.00", Taux: "0", Mois: "12", Methode: ""})
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
	moteur := NewMoteur(binaire(t), 0)

	// Capital nul : le programme COBOL doit refuser et sortir en erreur.
	_, err := moteur.Calculer(context.Background(), Demande{
		CapitalCentimes: 0, TauxMillioniemes: 3450000, Mois: 12,
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
		{Capital: "250000.00", Taux: "3.45", Mois: "240", FraisDossier: "1500.00"},
		{Capital: "250000.00", Taux: "3.45", Mois: "240",
			FraisDossier: "1500.00", FraisGarantie: "900.00"},
		{Capital: "250000.00", Taux: "3.45", Mois: "240",
			TauxAssurance: "0.36", Assiette: "capital_initial"},
		{Capital: "250000.00", Taux: "3.45", Mois: "240",
			TauxAssurance: "0.36", Assiette: "capital_restant_du"},
		{Capital: "120000.00", Taux: "4.2", Mois: "60",
			FraisDossier: "800.00", TauxAssurance: "0.5", Assiette: "capital_restant_du"},
		// Taux nul mais assurance : les flux ne sont pas triviaux pour autant.
		{Capital: "10000.00", Taux: "0", Mois: "12",
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
func TestLesFraisEtLAssuranceRencherissentLeTaeg(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	calculer := func(p Parametres) *Echeancier {
		t.Helper()
		p.Capital, p.Taux, p.Mois = "250000.00", "3.45", "240"
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
	avecFrais := calculer(Parametres{FraisDossier: "1500.00"})
	surInitial := calculer(Parametres{TauxAssurance: "0.36", Assiette: "capital_initial"})
	surRestant := calculer(Parametres{TauxAssurance: "0.36", Assiette: "capital_restant_du"})

	taeg := func(e *Echeancier) int64 {
		v, err := decimalVersEntier(e.Recapitulatif.Taeg.String(), 4)
		if err != nil {
			t.Fatalf("taeg illisible : %v", err)
		}
		return v
	}

	// Valeurs recoupees avec une dichotomie independante en Decimal Python.
	if got := nu.Recapitulatif.Taeg.String(); got != "3.5051" {
		t.Errorf("sans frais ni assurance : taeg %s, attendu 3.5051", got)
	}
	if got := avecFrais.Recapitulatif.Taeg.String(); got != "3.5752" {
		t.Errorf("avec 1500 de frais : taeg %s, attendu 3.5752", got)
	}
	if got := surInitial.Recapitulatif.Taeg.String(); got != "4.1020" {
		t.Errorf("assurance sur capital initial : taeg %s, attendu 4.1020", got)
	}

	if taeg(avecFrais) <= taeg(nu) {
		t.Error("les frais devraient faire monter le TAEG")
	}
	if taeg(surInitial) <= taeg(nu) {
		t.Error("l'assurance devrait faire monter le TAEG")
	}
	// Une prime assise sur le capital restant du decroit : elle coute moins.
	if taeg(surRestant) >= taeg(surInitial) {
		t.Error("l'assurance sur capital restant du devrait couter moins que sur capital initial")
	}
	if centimes(t, surRestant.Recapitulatif.TotalAssurance) >=
		centimes(t, surInitial.Recapitulatif.TotalAssurance) {
		t.Error("le total d'assurance sur capital restant du devrait etre inferieur")
	}

	// Sans assurance, la colonne reste a zero partout.
	for _, e := range nu.Echeancier {
		if e.Assurance.String() != "0.00" {
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

func TestVerdictDUsure(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	calculer := func(plafond string) *Echeancier {
		t.Helper()
		d, err := ParseDemande(Parametres{
			Capital: "250000.00", Taux: "3.45", Mois: "240", TauxUsure: plafond,
		})
		if err != nil {
			t.Fatalf("ParseDemande(%q) : %v", plafond, err)
		}
		res, err := moteur.Calculer(context.Background(), d)
		if err != nil {
			t.Fatalf("Calculer : %v", err)
		}
		return res
	}

	// Sans plafond, le TAEG est rendu sans jugement.
	sans := calculer("").Recapitulatif
	if sans.Conforme != nil || sans.TauxUsure != nil || sans.MargeUsure != nil {
		t.Errorf("sans plafond, le verdict devrait etre nul : %+v", sans)
	}
	if sans.Taeg.String() != "3.5051" {
		t.Errorf("taeg %s, attendu 3.5051", sans.Taeg)
	}

	cas := []struct {
		nom      string
		plafond  string
		conforme bool
		marge    string
	}{
		{"largement conforme", "5.88", true, "2.3749"},
		// Un TAEG egal au plafond reste licite : le depassement est strict.
		{"egal au plafond", "3.5051", true, "0.0000"},
		{"depassement d'un point de base", "3.5", false, "-0.0051"},
		{"plafond derisoire", "1", false, "-2.5051"},
	}

	for _, c := range cas {
		r := calculer(c.plafond).Recapitulatif
		if r.Conforme == nil {
			t.Errorf("%s : verdict absent", c.nom)
			continue
		}
		if *r.Conforme != c.conforme {
			t.Errorf("%s : conforme=%v, attendu %v", c.nom, *r.Conforme, c.conforme)
		}
		if r.MargeUsure.String() != c.marge {
			t.Errorf("%s : marge %s, attendu %s", c.nom, r.MargeUsure, c.marge)
		}
		if r.TauxUsure == nil {
			t.Errorf("%s : le plafond devrait etre rendu", c.nom)
		}
	}
}

// Les frais et l'assurance font monter le TAEG : un pret licite nu peut
// devenir usuraire une fois tous les couts integres. C'est precisement ce que
// la reglementation vise, et le service doit le voir.
func TestUnPretLiciteNuPeutDevenirUsuraire(t *testing.T) {
	moteur := NewMoteur(binaire(t), 0)

	verdict := func(p Parametres) (bool, string) {
		t.Helper()
		p.Capital, p.Taux, p.Mois, p.TauxUsure = "250000.00", "3.45", "240", "3.9"
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
		return *res.Recapitulatif.Conforme, res.Recapitulatif.Taeg.String()
	}

	if ok, taeg := verdict(Parametres{}); !ok {
		t.Errorf("nu : taeg %s devrait passer sous un plafond de 3.9", taeg)
	}
	if ok, taeg := verdict(Parametres{
		TauxAssurance: "0.36", Assiette: "capital_initial",
	}); ok {
		t.Errorf("avec assurance : taeg %s devrait depasser le plafond de 3.9", taeg)
	}
}

func TestNombreSigne(t *testing.T) {
	cas := []struct{ champ, attendu string }{
		{"+023749", "2.3749"},
		{"-000051", "-0.0051"},
		{"+000000", "0.0000"},
		// Le zero negatif ne doit pas ressortir avec un signe.
		{"-000000", "0.0000"},
	}
	for _, c := range cas {
		got, err := nombreSigne(c.champ, 4)
		if err != nil {
			t.Errorf("nombreSigne(%q) : %v", c.champ, err)
			continue
		}
		if got.String() != c.attendu {
			t.Errorf("nombreSigne(%q) = %q, attendu %q", c.champ, got, c.attendu)
		}
	}
	for _, mauvais := range []string{"", "+", "x000000", "0000000"} {
		if _, err := nombreSigne(mauvais, 4); err == nil {
			t.Errorf("nombreSigne(%q) aurait du echouer", mauvais)
		}
	}
}
