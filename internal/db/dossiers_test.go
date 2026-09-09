package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func dossierDeTest(t *testing.T, dd *Dossiers, agent *Agent) *Dossier {
	t.Helper()
	reference := "PRT-" + time.Now().Format("20060102150405.000000000")
	d, err := dd.Creer(context.Background(), agent, Dossier{
		Reference: reference, Capital: "250000.000", Taux: "8.500000",
		Mois: 240, Methode: "annuite_constante", TypeDiffere: "partiel",
	})
	if err != nil {
		t.Fatalf("Creer : %v", err)
	}
	return d
}

// Le cycle de vie est une machine a etats : un dossier ne saute pas
// l'instruction, et une decision ne se revient pas.
func TestLeCycleDeVieDUnDossier(t *testing.T) {
	permises := map[string][]string{
		"brouillon":      {"en_instruction", "annule"},
		"en_instruction": {"accorde", "refuse", "annule"},
		"accorde":        {},
		"refuse":         {},
		"annule":         {},
	}

	for de, vers := range permises {
		for _, statut := range StatutsAcceptes() {
			attendu := false
			for _, p := range vers {
				if p == statut {
					attendu = true
				}
			}
			if got := TransitionPermise(de, statut); got != attendu {
				t.Errorf("%s -> %s : %v, attendu %v", de, statut, got, attendu)
			}
		}
		// Un statut ne mene jamais a lui-meme.
		if TransitionPermise(de, de) {
			t.Errorf("%s -> %s : une transition vers soi-meme est permise", de, de)
		}
	}

	// Les trois etats terminaux ne mènent nulle part.
	for _, terminal := range []string{"accorde", "refuse", "annule"} {
		if suites := TransitionsDepuis(terminal); len(suites) != 0 {
			t.Errorf("%s mene a %v, attendu aucune suite", terminal, suites)
		}
	}
}

func TestCreerPuisLireUnDossier(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	cree := dossierDeTest(t, dossiers, agent)
	if cree.Statut != "brouillon" {
		t.Errorf("statut %q, attendu brouillon", cree.Statut)
	}
	if cree.Agence != agent.Agence {
		t.Errorf("agence %q, attendu %q", cree.Agence, agent.Agence)
	}

	lu, err := dossiers.Lire(ctx, agent, cree.ID)
	if err != nil {
		t.Fatalf("Lire : %v", err)
	}
	if lu.Reference != cree.Reference {
		t.Errorf("reference %q, attendu %q", lu.Reference, cree.Reference)
	}

	// La creation inscrit son evenement : l'historique commence a l'origine.
	hist, err := dossiers.Historique(ctx, agent, cree.ID)
	if err != nil {
		t.Fatalf("Historique : %v", err)
	}
	if len(hist) != 1 || hist[0].StatutApres != "brouillon" {
		t.Errorf("historique %+v, attendu un evenement vers brouillon", hist)
	}
	if hist[0].StatutAvant != nil {
		t.Errorf("statut avant %q, attendu nul a la creation", *hist[0].StatutAvant)
	}
}

// Un agent ne voit pas les dossiers d'une autre agence, et le message ne dit
// pas qu'ils existent.
func TestUnDossierNeSortPasDeSonAgence(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)

	_, _, agentA := agentDeTest(t, agents)
	_, _, agentB := agentDeTest(t, agents)
	// Deplacer le second dans une autre agence.
	// agentDeTest donne deja une agence propre a chacun : agentB ne doit rien
	// voir des dossiers d'agentA.
	if agentA.Agence == agentB.Agence {
		t.Fatal("les deux agents partagent une agence, le test ne prouve rien")
	}

	cree := dossierDeTest(t, dossiers, agentA)

	if _, err := dossiers.Lire(ctx, agentB, cree.ID); !errors.Is(err, ErrDossierAbsent) {
		t.Errorf("erreur %v, attendu ErrDossierAbsent", err)
	}
	if _, err := dossiers.ChangerLeStatut(ctx, agentB, cree.ID,
		"en_instruction", ""); !errors.Is(err, ErrDossierAbsent) {
		t.Errorf("changement de statut : erreur %v, attendu ErrDossierAbsent", err)
	}

	page, err := dossiers.Lister(ctx, agentB, Filtre{Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	for _, d := range page.Dossiers {
		if d.ID == cree.ID {
			t.Error("un dossier d'une autre agence figure dans la liste")
		}
	}
}

func TestLaReferenceEstUniqueDansLAgence(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	cree := dossierDeTest(t, dossiers, agent)
	_, err := dossiers.Creer(ctx, agent, Dossier{
		Reference: cree.Reference, Capital: "1000.000", Taux: "5.000000",
		Mois: 12, Methode: "annuite_constante", TypeDiffere: "partiel",
	})
	if !errors.Is(err, ErrReferencePrise) {
		t.Errorf("erreur %v, attendu ErrReferencePrise", err)
	}
}

// Une fois transmis a l'instruction, les parametres du pret ne bougent plus :
// la decision doit porter sur ce qui a ete instruit.
func TestUnDossierInstruitNEstPlusModifiable(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)
	cree := dossierDeTest(t, dossiers, agent)

	// En brouillon, la modification passe.
	modifie, err := dossiers.Modifier(ctx, agent, cree.ID, Dossier{
		Reference: cree.Reference, Capital: "300000.000", Taux: "9.000000",
		Mois: 180, Methode: "capital_constant", TypeDiffere: "partiel",
	})
	if err != nil {
		t.Fatalf("Modifier : %v", err)
	}
	if modifie.Capital != "300000.000" || modifie.Mois != 180 {
		t.Errorf("capital %s mois %d, modification non prise",
			modifie.Capital, modifie.Mois)
	}

	if _, err := dossiers.ChangerLeStatut(ctx, agent, cree.ID,
		"en_instruction", "transmis"); err != nil {
		t.Fatalf("ChangerLeStatut : %v", err)
	}

	if _, err := dossiers.Modifier(ctx, agent, cree.ID, Dossier{
		Reference: cree.Reference, Capital: "999.000", Taux: "1.000000",
		Mois: 12, Methode: "annuite_constante", TypeDiffere: "partiel",
	}); !errors.Is(err, ErrDossierFige) {
		t.Errorf("erreur %v, attendu ErrDossierFige", err)
	}
}

func TestUneTransitionInterditeEstRefusee(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)
	cree := dossierDeTest(t, dossiers, agent)

	// Un brouillon ne s'accorde pas sans passer par l'instruction.
	if _, err := dossiers.ChangerLeStatut(ctx, agent, cree.ID,
		"accorde", ""); !errors.Is(err, ErrTransitionRefusee) {
		t.Errorf("erreur %v, attendu ErrTransitionRefusee", err)
	}

	for _, statut := range []string{"en_instruction", "accorde"} {
		if _, err := dossiers.ChangerLeStatut(ctx, agent, cree.ID,
			statut, "note"); err != nil {
			t.Fatalf("passage a %s : %v", statut, err)
		}
	}

	// Accorde est terminal : rien n'en repart.
	if _, err := dossiers.ChangerLeStatut(ctx, agent, cree.ID,
		"annule", ""); !errors.Is(err, ErrTransitionRefusee) {
		t.Errorf("erreur %v, attendu ErrTransitionRefusee depuis accorde", err)
	}

	// L'historique retient chaque decision et son auteur.
	hist, err := dossiers.Historique(ctx, agent, cree.ID)
	if err != nil {
		t.Fatalf("Historique : %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("%d evenements, attendu 3", len(hist))
	}
	for _, e := range hist {
		if e.AgentID != agent.ID {
			t.Errorf("evenement attribue a l'agent %d, attendu %d",
				e.AgentID, agent.ID)
		}
	}
	if hist[2].StatutAvant == nil || *hist[2].StatutAvant != "en_instruction" {
		t.Error("le dernier evenement ne retient pas le statut precedent")
	}
}

func TestListerFiltreParStatut(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	garde := dossierDeTest(t, dossiers, agent)
	transmis := dossierDeTest(t, dossiers, agent)
	if _, err := dossiers.ChangerLeStatut(ctx, agent, transmis.ID,
		"en_instruction", ""); err != nil {
		t.Fatalf("ChangerLeStatut : %v", err)
	}

	page, err := dossiers.Lister(ctx, agent, Filtre{Statut: "en_instruction", Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	vus := map[int64]bool{}
	for _, d := range page.Dossiers {
		vus[d.ID] = true
		if d.Statut != "en_instruction" {
			t.Errorf("dossier %d de statut %q dans un filtre en_instruction",
				d.ID, d.Statut)
		}
	}
	if !vus[transmis.ID] {
		t.Error("le dossier transmis ne figure pas dans le filtre")
	}
	if vus[garde.ID] {
		t.Error("un brouillon figure dans le filtre en_instruction")
	}
}

// La pagination par curseur doit parcourir tous les dossiers, sans en sauter
// ni en rendre deux fois — ce qu'un OFFSET ne garantit pas quand des dossiers
// sont crees pendant le parcours.
func TestLaPaginationParcourtToutSansDoublon(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	const combien = 25
	attendus := map[int64]bool{}
	for i := 0; i < combien; i++ {
		attendus[dossierDeTest(t, dossiers, agent).ID] = true
	}

	vus := map[int64]bool{}
	curseur := ""
	pages := 0
	for {
		page, err := dossiers.Lister(ctx, agent,
			Filtre{Limite: 7, Curseur: curseur})
		if err != nil {
			t.Fatalf("Lister : %v", err)
		}
		pages++
		if pages > 20 {
			t.Fatal("la pagination ne s'arrete pas")
		}
		for _, d := range page.Dossiers {
			if vus[d.ID] {
				t.Errorf("dossier %d rendu deux fois", d.ID)
			}
			vus[d.ID] = true
		}
		if page.CurseurSuivant == "" {
			break
		}
		curseur = page.CurseurSuivant
	}

	for id := range attendus {
		if !vus[id] {
			t.Errorf("dossier %d jamais rendu par la pagination", id)
		}
	}
	// 25 dossiers par pages de 7 : quatre pages.
	if pages != 4 {
		t.Errorf("%d pages, attendu 4", pages)
	}
}

// La derniere page ne rend pas de curseur : c'est ainsi que le client sait
// qu'il a tout vu.
func TestLaDernierePageNeRendPasDeCurseur(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	for i := 0; i < 3; i++ {
		dossierDeTest(t, dossiers, agent)
	}

	page, err := dossiers.Lister(ctx, agent, Filtre{Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	if page.CurseurSuivant != "" {
		t.Error("un curseur est rendu alors que tout tient sur une page")
	}
	if len(page.Dossiers) != 3 {
		t.Errorf("%d dossiers, attendu 3", len(page.Dossiers))
	}
}

func TestUnCurseurIllisibleEstRefuse(t *testing.T) {
	pool := ouvrir(t)
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	for _, curseur := range []string{
		"pas du base64 !!", "Y2VjaQ", // « ceci », sans le separateur
		"MjAyNi0wMS0wMXwx", // date non conforme au RFC 3339
	} {
		if _, err := dossiers.Lister(context.Background(), agent,
			Filtre{Curseur: curseur}); !errors.Is(err, ErrCurseurInvalide) {
			t.Errorf("curseur %q : erreur %v, attendu ErrCurseurInvalide",
				curseur, err)
		}
	}
}

// La recherche porte sur un prefixe de reference, sans egard a la casse.
func TestLaRechercheParPrefixeDeReference(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	marque := time.Now().Format("150405.000000000")
	for _, suffixe := range []string{"A", "B", "C"} {
		if _, err := dossiers.Creer(ctx, agent, Dossier{
			Reference: "REF-" + marque + "-" + suffixe,
			Capital:   "1000.000", Taux: "5.000000", Mois: 12,
			Methode: "annuite_constante", TypeDiffere: "partiel",
		}); err != nil {
			t.Fatalf("Creer : %v", err)
		}
	}
	// Un dossier hors de la recherche.
	dossierDeTest(t, dossiers, agent)

	page, err := dossiers.Lister(ctx, agent,
		Filtre{Recherche: "ref-" + marque, Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	if len(page.Dossiers) != 3 {
		t.Fatalf("%d dossiers trouves, attendu 3", len(page.Dossiers))
	}
	for _, d := range page.Dossiers {
		if !strings.HasPrefix(strings.ToLower(d.Reference), "ref-"+marque) {
			t.Errorf("reference %q hors du prefixe cherche", d.Reference)
		}
	}

	// La casse ne compte pas.
	majuscules, err := dossiers.Lister(ctx, agent,
		Filtre{Recherche: "REF-" + marque, Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	if len(majuscules.Dossiers) != 3 {
		t.Errorf("%d dossiers en majuscules, attendu 3", len(majuscules.Dossiers))
	}
}

// Les caracteres generiques de LIKE doivent etre pris au pied de la lettre :
// un souligne saisi par l'agent vaut un souligne, non « un caractere
// quelconque ». Les references en portent couramment.
func TestLaRechercheNeLaissePasPasserLesJokers(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	dossiers := NewDossiers(pool)
	_, _, agent := agentDeTest(t, agents)

	marque := time.Now().Format("150405.000000000")
	if _, err := dossiers.Creer(ctx, agent, Dossier{
		Reference: "JOK-" + marque + "-XY",
		Capital:   "1000.000", Taux: "5.000000", Mois: 12,
		Methode: "annuite_constante", TypeDiffere: "partiel",
	}); err != nil {
		t.Fatalf("Creer : %v", err)
	}

	// « JOK-<marque>-_Y » ne doit rien trouver : le souligne est litteral.
	page, err := dossiers.Lister(ctx, agent,
		Filtre{Recherche: "JOK-" + marque + "-_Y", Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	if len(page.Dossiers) != 0 {
		t.Errorf("%d dossiers trouves, le souligne a servi de joker",
			len(page.Dossiers))
	}

	// Et un pourcent seul ne doit pas tout rendre.
	tout, err := dossiers.Lister(ctx, agent, Filtre{Recherche: "%", Limite: 100})
	if err != nil {
		t.Fatalf("Lister : %v", err)
	}
	if len(tout.Dossiers) != 0 {
		t.Errorf("%d dossiers rendus pour la recherche « %% »", len(tout.Dossiers))
	}
}
