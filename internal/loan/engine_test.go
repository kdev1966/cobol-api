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
func recap(echeances int, mensualite, interets, total string) string {
	return fmt.Sprintf("R%04d%013s%015s%015s", echeances, mensualite, interets, total)
}

func echeance(n int, paiement, interets, capital, solde string) string {
	return fmt.Sprintf("E%04d%013s%013s%013s%013s", n, paiement, interets, capital, solde)
}

func TestLireSortieEcarteLesLignesParasites(t *testing.T) {
	// libcob peut ecrire un avertissement sur la sortie standard ; il ne
	// doit pas faire echouer le calcul.
	sortie := bytes.NewBufferString(strings.Join([]string{
		"libcob: warning: implicit CLOSE of SYSIN",
		recap(2, "0000000010000", "000000000000500", "000000000020500"),
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
	if got := res.Recapitulatif.Mensualite.String(); got != "100.00" {
		t.Errorf("mensualite %q, attendu \"100.00\"", got)
	}
	if got := res.Echeancier[1].Solde.String(); got != "0.00" {
		t.Errorf("solde final %q, attendu \"0.00\"", got)
	}
}

func TestLireSortieRefuseUnComptageIncoherent(t *testing.T) {
	sortie := bytes.NewBufferString(strings.Join([]string{
		recap(5, "0000000010000", "000000000000500", "000000000020500"),
		echeance(1, "0000000010000", "0000000000300", "0000000009700", "0000000000000"),
	}, "\n"))

	if _, err := lireSortie(sortie); err == nil {
		t.Fatal("un echeancier tronque aurait du etre refuse")
	}
}

func TestLireSortieRefuseUnRecapitulatifEnDouble(t *testing.T) {
	ligne := recap(1, "0000000010000", "000000000000500", "000000000020500")
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
		recap(1, "0000000000007", "000000000000007", "000000000000007"),
		echeance(1, "0838860801000", "0000000000007", "0000000000029", "0000000000000"),
	}, "\n"))

	res, err := lireSortie(sortie)
	if err != nil {
		t.Fatalf("lireSortie : %v", err)
	}
	if got := res.Recapitulatif.Mensualite.String(); got != "0.07" {
		t.Errorf("mensualite %q, attendu \"0.07\"", got)
	}
	if got := res.Echeancier[0].Paiement.String(); got != "8388608010.00" {
		t.Errorf("paiement %q, attendu \"8388608010.00\"", got)
	}
}

func TestMontantInsereLePointDecimal(t *testing.T) {
	cas := []struct{ chiffres, attendu string }{
		{"0000000144348", "1443.48"},
		{"0000000000007", "0.07"},
		{"0000000000000", "0.00"},
		{"9999999999999", "99999999999.99"},
		{"000", "0.00"},
	}
	for _, c := range cas {
		got, err := montant(c.chiffres)
		if err != nil {
			t.Errorf("montant(%q) : %v", c.chiffres, err)
			continue
		}
		if got.String() != c.attendu {
			t.Errorf("montant(%q) = %q, attendu %q", c.chiffres, got, c.attendu)
		}
	}
	for _, mauvais := range []string{"", "12", "12x45", "  1234"} {
		if _, err := montant(mauvais); err == nil {
			t.Errorf("montant(%q) aurait du echouer", mauvais)
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

	for _, c := range cas {
		t.Run(c.capital+"@"+c.taux+"%x"+c.mois, func(t *testing.T) {
			d, err := ParseDemande(c.capital, c.taux, c.mois)
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
					t.Errorf("echeance %d : paiement %d != interets %d + capital %d",
						e.N, p, i, k)
				}
				sommeCapital += k
				sommeInterets += i
				sommeEcheances += p
			}

			// Invariant 1 : la somme des parts de capital egale le capital.
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
		})
	}
}

// Cas de reference verifie independamment avec le module decimal de Python.
func TestCasDeReference(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	d, err := ParseDemande("250000.00", "3.45", "240")
	if err != nil {
		t.Fatalf("ParseDemande : %v", err)
	}
	res, err := moteur.Calculer(context.Background(), d)
	if err != nil {
		t.Fatalf("Calculer : %v", err)
	}

	attendus := map[string]string{
		"mensualite":     res.Recapitulatif.Mensualite.String(),
		"total_interets": res.Recapitulatif.TotalInterets.String(),
		"total_du":       res.Recapitulatif.TotalDu.String(),
	}
	references := map[string]string{
		"mensualite":     "1443.48",
		"total_interets": "96436.65",
		"total_du":       "346436.65",
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

func TestCalculerRemonteLEchecDuProgramme(t *testing.T) {
	moteur := NewMoteur(binaire(t))

	// Capital nul : le programme COBOL doit refuser et sortir en erreur.
	_, err := moteur.Calculer(context.Background(), Demande{
		CapitalCentimes: 0, TauxMillioniemes: 3450000, Mois: 12,
	})
	if err == nil {
		t.Fatal("un capital nul aurait du faire echouer le programme")
	}
}

func TestCalculerAvecUnBinaireIntrouvable(t *testing.T) {
	moteur := NewMoteur("/inexistant/loan_amortization")
	d, _ := ParseDemande("1000.00", "3.45", "12")

	if _, err := moteur.Calculer(context.Background(), d); err == nil {
		t.Fatal("un binaire absent aurait du produire une erreur")
	}
}
