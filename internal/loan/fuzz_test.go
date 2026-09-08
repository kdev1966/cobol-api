package loan

import (
	"bytes"
	"testing"
)

// FuzzLireSortie eprouve le lecteur d'enregistrements sur des entrees
// arbitraires. Ce code interprete la sortie d'un sous-processus par decoupage
// positionnel : c'est exactement le genre d'endroit ou une longueur mal
// verifiee produit une panique que les tests ecrits a la main n'atteignent
// pas. Le contrat est simple — lireSortie rend une erreur ou un resultat,
// jamais une panique.
func FuzzLireSortie(f *testing.F) {
	f.Add("")
	f.Add("R02400000000144348000000014449300000000964366500000003464366503505 1")
	f.Add(recap(1, "0000000010000", "0000000010000", "000000000000500", "000000000010500") +
		"\n" + echeance(1, "0000000010500", "0000000000500", "0000000010000", "0000000000000"))
	f.Add("libcob: warning: quelque chose\nR\nE\n")
	f.Add("RRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRR")
	f.Add("E000100000001443480000000071875000000007247300000249275270000")
	// Des chiffres a la bonne longueur mais tronques en plein champ.
	f.Add("R024000000001443480000000144493")

	f.Fuzz(func(t *testing.T, entree string) {
		res, err := lireSortie(bytes.NewBufferString(entree))

		// Un succes doit etre coherent : le nombre d'echeances annonce est
		// celui qui a ete lu, et chaque montant est une decimale valide.
		if err == nil {
			if len(res.Echeancier) != res.Recapitulatif.Echeances {
				t.Fatalf("succes incoherent : %d annoncees, %d lues",
					res.Recapitulatif.Echeances, len(res.Echeancier))
			}
			for _, e := range res.Echeancier {
				for nom, v := range map[string]string{
					"paiement": e.Paiement.String(),
					"interets": e.Interets.String(),
					"capital":  e.Capital.String(),
					"solde":    e.Solde.String(),
				} {
					if _, err := decimalVersEntier(v, 2); err != nil {
						t.Fatalf("%s illisible en sortie : %q", nom, v)
					}
				}
			}
		}
	})
}

// FuzzDecimalVersEntier eprouve la conversion des parametres de requete, qui
// recoit directement du texte fourni par le client.
func FuzzDecimalVersEntier(f *testing.F) {
	f.Add("250000.00", 2)
	f.Add("3.45", 6)
	f.Add("", 2)
	f.Add(".", 2)
	f.Add("...", 2)
	f.Add("99999999999999999999.99", 2)
	f.Add("-0", 2)

	f.Fuzz(func(t *testing.T, texte string, decimales int) {
		// Les seules echelles employees par le service.
		if decimales != 2 && decimales != 6 {
			t.Skip()
		}
		valeur, err := decimalVersEntier(texte, decimales)
		if err == nil && valeur < 0 {
			t.Fatalf("valeur negative %d rendue pour %q sans erreur", valeur, texte)
		}
	})
}
