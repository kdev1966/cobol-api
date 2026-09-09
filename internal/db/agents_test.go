package db

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// agentDeTest cree un compte jetable et rend son identifiant et son mot de
// passe. Le nom porte l'instant pour que deux tests ne se disputent pas le
// meme identifiant.
func agentDeTest(t *testing.T, a *Agents) (string, string, *Agent) {
	t.Helper()
	marque := time.Now().Format("20060102150405.000000000")
	identifiant := "agent-" + marque
	motDePasse := "un mot de passe assez long"

	// Une agence propre a chaque agent de test : la visibilite des dossiers
	// est par agence, deux tests partageant la meme se verraient mutuellement.
	ag, err := a.Creer(context.Background(), identifiant, motDePasse,
		"Agent de test", "Agence "+marque)
	if err != nil {
		t.Fatalf("Creer : %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		// Les dossiers referencent l'agent sans cascade : en production on
		// desactive un compte plutot que de le supprimer, justement pour que
		// ses dossiers restent rattachables. Le menage de test doit donc
		// retirer les dossiers avant le compte.
		if _, err := a.pool.Exec(ctx,
			`DELETE FROM dossiers WHERE agent_id = $1`, ag.ID); err != nil {
			t.Errorf("menage des dossiers : %v", err)
		}
		if _, err := a.pool.Exec(ctx,
			`DELETE FROM agents WHERE id = $1`, ag.ID); err != nil {
			t.Errorf("menage de l'agent : %v", err)
		}
	})
	return identifiant, motDePasse, ag
}

func TestHacherPuisVerifierUnMotDePasse(t *testing.T) {
	empreinte, err := HacherMotDePasse("correct cheval batterie agrafe")
	if err != nil {
		t.Fatalf("HacherMotDePasse : %v", err)
	}

	// L'empreinte porte ses parametres : les durcir plus tard n'invalidera
	// pas celles deja calculees.
	if !strings.HasPrefix(empreinte, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("empreinte %q, format argon2id attendu", empreinte)
	}
	// Le mot de passe ne doit apparaitre nulle part dans l'empreinte.
	if strings.Contains(empreinte, "cheval") {
		t.Error("l'empreinte laisse transparaitre le mot de passe")
	}

	bon, err := VerifierMotDePasse("correct cheval batterie agrafe", empreinte)
	if err != nil {
		t.Fatalf("VerifierMotDePasse : %v", err)
	}
	if !bon {
		t.Error("le bon mot de passe a ete refuse")
	}

	mauvais, err := VerifierMotDePasse("correct cheval batterie agrafF", empreinte)
	if err != nil {
		t.Fatalf("VerifierMotDePasse : %v", err)
	}
	if mauvais {
		t.Error("un mot de passe faux a ete accepte")
	}
}

// Deux hachages du meme mot de passe doivent differer : sans sel, une table
// arc-en-ciel les casserait tous d'un coup.
func TestLeSelRendChaqueEmpreinteUnique(t *testing.T) {
	a, err := HacherMotDePasse("le meme mot de passe")
	if err != nil {
		t.Fatalf("HacherMotDePasse : %v", err)
	}
	b, err := HacherMotDePasse("le meme mot de passe")
	if err != nil {
		t.Fatalf("HacherMotDePasse : %v", err)
	}
	if a == b {
		t.Error("deux empreintes identiques : le sel ne varie pas")
	}
}

func TestVerifierRefuseUneEmpreinteIllisible(t *testing.T) {
	cas := []string{
		"", "pas une empreinte", "$argon2i$v=19$m=65536,t=3,p=4$c2Vs$Y2xl",
		"$argon2id$v=1$m=65536,t=3,p=4$c2Vs$Y2xl",
		"$argon2id$v=19$m=65536,t=3,p=4$???$Y2xl",
	}
	for _, empreinte := range cas {
		if _, err := VerifierMotDePasse("x", empreinte); err == nil {
			t.Errorf("empreinte %q : aurait du etre refusee", empreinte)
		}
	}
}

func TestAuthentifierOuvreUneSession(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	identifiant, motDePasse, cree := agentDeTest(t, agents)

	jeton, ag, err := agents.Authentifier(ctx, identifiant, motDePasse,
		"192.0.2.10", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("Authentifier : %v", err)
	}
	if ag.ID != cree.ID {
		t.Errorf("agent %d, attendu %d", ag.ID, cree.ID)
	}
	if len(jeton) < 40 {
		t.Errorf("jeton de %d caracteres, trop court", len(jeton))
	}

	// Le jeton ne doit pas etre conserve en clair : la base ne garde que son
	// empreinte, et une base derobee ne permet pas de reprendre la session.
	var enClair int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sessions WHERE encode(empreinte, 'escape') = $1`,
		jeton).Scan(&enClair); err != nil {
		t.Fatalf("comptage : %v", err)
	}
	if enClair != 0 {
		t.Error("le jeton est conserve en clair dans la base")
	}

	// La session vaut jusqu'a sa revocation.
	trouve, err := agents.AgentDeSession(ctx, jeton)
	if err != nil {
		t.Fatalf("AgentDeSession : %v", err)
	}
	if trouve.ID != cree.ID {
		t.Errorf("session rendue a l'agent %d, attendu %d", trouve.ID, cree.ID)
	}

	if err := agents.Deconnecter(ctx, jeton); err != nil {
		t.Fatalf("Deconnecter : %v", err)
	}
	if _, err := agents.AgentDeSession(ctx, jeton); !errors.Is(err, ErrSessionInconnue) {
		t.Errorf("erreur %v, attendu ErrSessionInconnue apres deconnexion", err)
	}
}

// L'identifiant inconnu et le mot de passe faux doivent etre indiscernables :
// les distinguer dirait a un attaquant quels comptes existent.
func TestAuthentifierNeDistinguePasLesEchecs(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	identifiant, _, _ := agentDeTest(t, agents)

	_, _, errInconnu := agents.Authentifier(ctx,
		"personne-de-ce-nom", "peu importe", "", "")
	_, _, errFaux := agents.Authentifier(ctx,
		identifiant, "mauvais mot de passe", "", "")

	if !errors.Is(errInconnu, ErrIdentifiantsRefuses) {
		t.Errorf("identifiant inconnu : erreur %v", errInconnu)
	}
	if !errors.Is(errFaux, ErrIdentifiantsRefuses) {
		t.Errorf("mot de passe faux : erreur %v", errFaux)
	}
	if errInconnu.Error() != errFaux.Error() {
		t.Errorf("messages distincts : %q et %q", errInconnu, errFaux)
	}
}

// Un compte desactive ne doit plus ouvrir de session, sans que le message ne
// revele que le compte existe.
func TestUnCompteDesactiveNeSeConnectePlus(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	identifiant, motDePasse, cree := agentDeTest(t, agents)

	if _, err := pool.Exec(ctx,
		`UPDATE agents SET actif = false WHERE id = $1`, cree.ID); err != nil {
		t.Fatalf("desactivation : %v", err)
	}

	if _, _, err := agents.Authentifier(ctx, identifiant, motDePasse, "", ""); !errors.Is(err, ErrIdentifiantsRefuses) {
		t.Errorf("erreur %v, attendu ErrIdentifiantsRefuses", err)
	}
}

// L'identifiant est unique quelle que soit la casse : sans cela, deux comptes
// voisins se creeraient sans que personne ne les distingue a l'oeil.
func TestLIdentifiantEstUniqueSansEgardALaCasse(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	identifiant, motDePasse, _ := agentDeTest(t, agents)

	_, err := agents.Creer(ctx, strings.ToUpper(identifiant), "autre", "X", "Y")
	if !errors.Is(err, ErrIdentifiantPris) {
		t.Errorf("erreur %v, attendu ErrIdentifiantPris", err)
	}

	// Et la connexion accepte n'importe quelle casse.
	if _, _, err := agents.Authentifier(ctx,
		strings.ToUpper(identifiant), motDePasse, "", ""); err != nil {
		t.Errorf("connexion en majuscules refusee : %v", err)
	}
}

func TestUneSessionExpireeNEstPlusValable(t *testing.T) {
	pool := ouvrir(t)
	ctx := context.Background()
	agents := NewAgents(pool)
	identifiant, motDePasse, _ := agentDeTest(t, agents)

	jeton, _, err := agents.Authentifier(ctx, identifiant, motDePasse, "", "")
	if err != nil {
		t.Fatalf("Authentifier : %v", err)
	}

	// Vieillir la session plutot que d'attendre huit heures.
	if _, err := pool.Exec(ctx,
		`UPDATE sessions SET expire_le = now() - interval '1 minute'
		 WHERE agent_id = (SELECT id FROM agents WHERE lower(identifiant) = lower($1))`,
		identifiant); err != nil {
		t.Fatalf("vieillissement : %v", err)
	}

	if _, err := agents.AgentDeSession(ctx, jeton); !errors.Is(err, ErrSessionInconnue) {
		t.Errorf("erreur %v, attendu ErrSessionInconnue", err)
	}

	// Et la purge la retire.
	n, err := agents.PurgerLesSessions(ctx)
	if err != nil {
		t.Fatalf("PurgerLesSessions : %v", err)
	}
	if n < 1 {
		t.Errorf("%d session purgee, au moins une attendue", n)
	}
}

func TestAgentDeSessionRefuseUnJetonInvente(t *testing.T) {
	agents := NewAgents(ouvrir(t))
	if _, err := agents.AgentDeSession(context.Background(),
		"jeton-entierement-invente"); !errors.Is(err, ErrSessionInconnue) {
		t.Errorf("erreur %v, attendu ErrSessionInconnue", err)
	}
}

// Le calcul argon2id reserve 64 Mio a chaque appel. Sans borne, une rafale de
// connexions les cumule et fait passer le service de quelques dizaines de
// megaoctets a plus d'un gigaoctet. Ce test verifie que la borne tient sous
// une rafale bien plus large qu'elle.
func TestLesVerificationsSimultaneesSontBornees(t *testing.T) {
	empreinte, err := HacherMotDePasse("un mot de passe quelconque")
	if err != nil {
		t.Fatalf("HacherMotDePasse : %v", err)
	}

	var maximum int64
	var mu sync.Mutex
	var groupe sync.WaitGroup

	// Un observateur echantillonne l'occupation du canal de jetons pendant
	// la rafale : sa longueur est le nombre de calculs en cours.
	arret := make(chan struct{})
	go func() {
		for {
			select {
			case <-arret:
				return
			default:
				mu.Lock()
				if n := int64(len(jetons)); n > maximum {
					maximum = n
				}
				mu.Unlock()
			}
		}
	}()

	for i := 0; i < 40; i++ {
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			if _, err := VerifierMotDePasse("un mot de passe quelconque",
				empreinte); err != nil {
				t.Errorf("VerifierMotDePasse : %v", err)
			}
		}()
	}
	groupe.Wait()
	close(arret)

	mu.Lock()
	vu := maximum
	mu.Unlock()

	if vu > VerificationsSimultanees {
		t.Errorf("%d calculs simultanes observes, borne a %d",
			vu, VerificationsSimultanees)
	}
	// La borne doit avoir mordu : quarante appels d'un coup contre quatre
	// places, sans quoi le test ne prouverait rien.
	if vu < 2 {
		t.Errorf("occupation maximale observee %d : la rafale n'a pas eu lieu", vu)
	}
}
