package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kdev1966/cobol-api/internal/db"
)

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

func serveurDeTest(t *testing.T, ajuster func(*Config)) http.Handler {
	t.Helper()
	cfg := Config{
		Port:              "0",
		CheminProgramme:   binaire(t),
		CleAPI:            "cle-de-test",
		RequetesParMinute: 1000,
	}
	if ajuster != nil {
		ajuster(&cfg)
	}
	s, err := NewServeur(cfg, nil)
	if err != nil {
		t.Fatalf("NewServeur : %v", err)
	}
	return s.Handler()
}

func appeler(h http.Handler, methode, cible, cle string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(methode, cible, nil)
	if cle != "" {
		r.Header.Set("X-API-Key", cle)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestProductionExigeUneCle(t *testing.T) {
	_, err := NewServeur(Config{
		CheminProgramme: binaire(t),
		CleAPI:          "",
		Production:      true,
	}, nil)
	if err == nil {
		t.Fatal("le serveur aurait du refuser de demarrer sans API_KEY en production")
	}
}

func TestBinaireIntrouvableEmpecheLeDemarrage(t *testing.T) {
	_, err := NewServeur(Config{
		CheminProgramme: "/inexistant/loan_amortization",
		CleAPI:          "cle",
	}, nil)
	if err == nil {
		t.Fatal("le serveur aurait du refuser de demarrer sans binaire COBOL")
	}
}

func TestAuthentification(t *testing.T) {
	h := serveurDeTest(t, nil)
	cible := "/v1/loans/schedule?capital=1000.000&taux=8.5&mois=12"

	cas := []struct {
		nom  string
		cle  string
		code int
	}{
		{"sans cle", "", http.StatusUnauthorized},
		{"cle erronee", "mauvaise", http.StatusUnauthorized},
		{"prefixe de la bonne cle", "cle-de", http.StatusUnauthorized},
		{"bonne cle suivie d'un caractere", "cle-de-testx", http.StatusUnauthorized},
		{"bonne cle", "cle-de-test", http.StatusOK},
	}

	for _, c := range cas {
		if got := appeler(h, "GET", cible, c.cle).Code; got != c.code {
			t.Errorf("%s : HTTP %d, attendu %d", c.nom, got, c.code)
		}
	}
}

func TestRoutesOuvertes(t *testing.T) {
	h := serveurDeTest(t, nil)

	// L'index et la sante restent joignables sans cle : le HEALTHCHECK du
	// conteneur s'appuie sur /health.
	for _, cible := range []string{"/", "/health", "/openapi.json"} {
		if got := appeler(h, "GET", cible, "").Code; got != http.StatusOK {
			t.Errorf("%s sans cle : HTTP %d, attendu 200", cible, got)
		}
	}
	if got := appeler(h, "GET", "/inconnu", "cle-de-test").Code; got != http.StatusNotFound {
		t.Errorf("/inconnu : HTTP %d, attendu 404", got)
	}
	if got := appeler(h, "POST", "/v1/loans/schedule", "cle-de-test").Code; got != http.StatusNotFound {
		t.Errorf("POST /v1/loans/schedule : HTTP %d, attendu 404", got)
	}
	// L'ancien chemin non versionne ne doit plus repondre.
	if got := appeler(h, "GET", "/loans/schedule?capital=1000&taux=8.5&mois=12",
		"cle-de-test").Code; got != http.StatusNotFound {
		t.Errorf("chemin non versionne : HTTP %d, attendu 404", got)
	}
}

func TestValidationDesParametres(t *testing.T) {
	h := serveurDeTest(t, nil)

	cas := []struct {
		cible string
		champ string
	}{
		{"/v1/loans/schedule?capital=0&taux=8.5&mois=12", "capital"},
		{"/v1/loans/schedule?capital=abc&taux=8.5&mois=12", "capital"},
		{"/v1/loans/schedule?capital=-1000&taux=8.5&mois=12", "capital"},
		{"/v1/loans/schedule?capital=1000.000&taux=100&mois=12", "taux"},
		{"/v1/loans/schedule?capital=1000.000&taux=8.5&mois=0", "mois"},
		{"/v1/loans/schedule?capital=1000.000&taux=8.5&mois=601", "mois"},
		{"/v1/loans/schedule?capital=1000.000&taux=8.5&mois=abc", "mois"},
		{"/v1/loans/schedule", "capital"},
		{"/v1/loans/schedule?capital=1000&taux=8.5&mois=12&methode=lineaire", "methode"},
	}

	for _, c := range cas {
		w := appeler(h, "GET", c.cible, "cle-de-test")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s : HTTP %d, attendu 400", c.cible, w.Code)
			continue
		}
		var corps struct {
			Champ string `json:"champ"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
			t.Errorf("%s : reponse illisible : %v", c.cible, err)
			continue
		}
		if corps.Champ != c.champ {
			t.Errorf("%s : champ %q, attendu %q", c.cible, corps.Champ, c.champ)
		}
	}
}

func TestEcheancierNominal(t *testing.T) {
	h := serveurDeTest(t, nil)

	w := appeler(h, "GET",
		"/v1/loans/schedule?capital=250000.000&taux=8.5&mois=240", "cle-de-test")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d : %s", w.Code, w.Body.String())
	}

	var corps struct {
		Status  string `json:"status"`
		Demande struct {
			Capital string `json:"capital"`
			Taux    string `json:"taux"`
			Mois    int    `json:"mois"`
			Methode string `json:"methode"`
		} `json:"demande"`
		Recapitulatif struct {
			Echeances          int         `json:"echeances"`
			PremiereMensualite json.Number `json:"premiere_mensualite"`
			DerniereMensualite json.Number `json:"derniere_mensualite"`
			TotalAssurance     json.Number `json:"total_assurance"`
			TotalFrais         json.Number `json:"total_frais"`
			Teg                json.Number `json:"teg"`
		} `json:"recapitulatif"`
		Echeancier []struct {
			N          int         `json:"n"`
			Mensualite json.Number `json:"mensualite"`
			Assurance  json.Number `json:"assurance"`
		} `json:"echeancier"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
		t.Fatalf("reponse illisible : %v", err)
	}

	if corps.Status != "success" {
		t.Errorf("status %q", corps.Status)
	}
	if corps.Demande.Capital != "250000.000" || corps.Demande.Taux != "8.500000" {
		t.Errorf("demande relayee : %+v", corps.Demande)
	}
	if len(corps.Echeancier) != 240 || corps.Recapitulatif.Echeances != 240 {
		t.Errorf("%d echeances, recapitulatif %d", len(corps.Echeancier), corps.Recapitulatif.Echeances)
	}
	if corps.Demande.Methode != "annuite_constante" {
		t.Errorf("methode par defaut %q", corps.Demande.Methode)
	}
	// Les montants doivent ressortir tels que le COBOL les a ecrits, au
	// millime.
	if got := corps.Recapitulatif.PremiereMensualite.String(); got != "2169.558" {
		t.Errorf("premiere mensualite %q, attendu \"2169.558\"", got)
	}
	if got := corps.Recapitulatif.DerniereMensualite.String(); got != "2169.614" {
		t.Errorf("derniere mensualite %q, attendu \"2169.614\"", got)
	}
	if got := corps.Echeancier[239].Mensualite.String(); got != "2169.614" {
		t.Errorf("derniere mensualite %q, attendu \"2169.614\"", got)
	}
	// Annualisation proportionnelle : sans frais, le TEG egale le nominal.
	if got := corps.Recapitulatif.Teg.String(); got != "8.50" {
		t.Errorf("teg %q, attendu \"8.50\"", got)
	}
}

func TestEnTetesDeSecurite(t *testing.T) {
	h := serveurDeTest(t, nil)
	w := appeler(h, "GET", "/health", "")

	attendus := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-origin",
	}
	for cle, valeur := range attendus {
		if got := w.Header().Get(cle); got != valeur {
			t.Errorf("%s = %q, attendu %q", cle, got, valeur)
		}
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy absent")
	}
}

func TestCORSFermeParDefaut(t *testing.T) {
	h := serveurDeTest(t, nil)

	r := httptest.NewRequest("GET", "/health", nil)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("origine inconnue autorisee : %q", got)
	}
}

func TestCORSAutoriseUneOrigineListee(t *testing.T) {
	h := serveurDeTest(t, func(c *Config) {
		c.OriginesCORS = []string{"https://boutique.example"}
	})

	r := httptest.NewRequest("GET", "/health", nil)
	r.Header.Set("Origin", "https://boutique.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://boutique.example" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestLimitationDeDebit(t *testing.T) {
	h := serveurDeTest(t, func(c *Config) { c.RequetesParMinute = 3 })
	cible := "/v1/loans/schedule?capital=1000.000&taux=8.5&mois=12"

	var ok, trop int
	for i := 0; i < 6; i++ {
		switch appeler(h, "GET", cible, "cle-de-test").Code {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			trop++
		}
	}
	if ok != 3 || trop != 3 {
		t.Errorf("%d acceptees et %d refusees, attendu 3 et 3", ok, trop)
	}

	// /health doit rester joignable malgre le plafond atteint.
	if got := appeler(h, "GET", "/health", "").Code; got != http.StatusOK {
		t.Errorf("/health sous plafond : HTTP %d, attendu 200", got)
	}
}

func TestSansCleLesEndpointsSontOuverts(t *testing.T) {
	h := serveurDeTest(t, func(c *Config) { c.CleAPI = "" })

	got := appeler(h, "GET", "/v1/loans/schedule?capital=1000.000&taux=8.5&mois=12", "").Code
	if got != http.StatusOK {
		t.Errorf("HTTP %d, attendu 200 en mode ouvert", got)
	}
}

func TestSpecificationServieEtCoherente(t *testing.T) {
	h := serveurDeTest(t, nil)

	w := appeler(h, "GET", "/openapi.json", "")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d", w.Code)
	}

	var spec struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("specification illisible : %v", err)
	}
	if spec.OpenAPI == "" {
		t.Error("champ openapi absent")
	}

	// Chaque chemin decrit doit repondre autre chose qu'un 404 : la
	// specification et les routes ne peuvent pas diverger en silence.
	for chemin := range spec.Paths {
		cible := chemin
		if chemin == "/v1/loans/schedule" {
			cible += "?capital=1000&taux=8.5&mois=12"
		}
		if got := appeler(h, "GET", cible, "cle-de-test").Code; got == http.StatusNotFound {
			t.Errorf("%s est decrit dans la specification mais rend 404", chemin)
		}
	}
}

func TestIdentifiantDeCorrelation(t *testing.T) {
	h := serveurDeTest(t, nil)

	premiere := appeler(h, "GET", "/health", "")
	seconde := appeler(h, "GET", "/health", "")

	id := premiere.Header().Get("X-Request-Id")
	if id == "" {
		t.Fatal("X-Request-Id absent de la reponse")
	}
	if autre := seconde.Header().Get("X-Request-Id"); autre == id {
		t.Errorf("deux requetes portent le meme identifiant : %s", id)
	}
}

func TestDelaiDeCalculDepasse(t *testing.T) {
	// Un programme qui ne rend jamais la main doit produire un 504, pas un
	// 500 ni une requete qui pend.
	lent, err := filepath.Abs("../loan/testdata/lent.sh")
	if err != nil {
		t.Fatalf("chemin : %v", err)
	}
	if _, err := os.Stat(lent); err != nil {
		t.Skip("fixture lent.sh absente")
	}

	s, err := NewServeur(Config{
		CheminProgramme:   lent,
		CleAPI:            "cle-de-test",
		RequetesParMinute: 100,
		DelaiCalcul:       150 * time.Millisecond,
	}, nil)
	if err != nil {
		t.Fatalf("NewServeur : %v", err)
	}

	w := appeler(s.Handler(), "GET",
		"/v1/loans/schedule?capital=1000&taux=8.5&mois=12", "cle-de-test")
	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("HTTP %d, attendu 504 : %s", w.Code, w.Body.String())
	}
}

// Sans base fournie, /health ne sonde pas la base et n'en parle pas.
func TestSanteSansBase(t *testing.T) {
	h := serveurDeTest(t, nil)

	w := appeler(h, "GET", "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d", w.Code)
	}
	var corps map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
		t.Fatalf("reponse illisible : %v", err)
	}
	if _, present := corps["databaseReachable"]; present {
		t.Error("databaseReachable ne devrait pas figurer sans base")
	}
	if corps["status"] != "OK" {
		t.Errorf("status %v", corps["status"])
	}
}

// Avec une base, /health la sonde et le dit.
func TestSanteAvecBase(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL absente")
	}
	pool, err := db.Ouvrir(context.Background(), url)
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	defer pool.Close()

	s, err := NewServeur(Config{
		CheminProgramme:   binaire(t),
		CleAPI:            "cle-de-test",
		RequetesParMinute: 100,
	}, pool)
	if err != nil {
		t.Fatalf("NewServeur : %v", err)
	}

	w := appeler(s.Handler(), "GET", "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d : %s", w.Code, w.Body.String())
	}
	var corps map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
		t.Fatalf("reponse illisible : %v", err)
	}
	if corps["databaseReachable"] != true {
		t.Errorf("databaseReachable = %v, attendu true", corps["databaseReachable"])
	}
}

// Une base fermee doit rendre 503 : le HEALTHCHECK du conteneur doit voir la
// panne, pas un OK de facade.
func TestSanteSignaleUneBaseInjoignable(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL absente")
	}
	pool, err := db.Ouvrir(context.Background(), url)
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}

	s, err := NewServeur(Config{
		CheminProgramme:   binaire(t),
		CleAPI:            "cle-de-test",
		RequetesParMinute: 100,
	}, pool)
	if err != nil {
		t.Fatalf("NewServeur : %v", err)
	}
	pool.Close()

	w := appeler(s.Handler(), "GET", "/health", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("HTTP %d, attendu 503 : %s", w.Code, w.Body.String())
	}
}

// serveurAvecBase monte un serveur relie a PostgreSQL, ou saute le test.
func serveurAvecBase(t *testing.T) http.Handler {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL absente")
	}
	pool, err := db.Ouvrir(context.Background(), url)
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrer(context.Background(), pool); err != nil {
		t.Fatalf("Migrer : %v", err)
	}

	s, err := NewServeur(Config{
		CheminProgramme:   binaire(t),
		CleAPI:            "cle-de-test",
		RequetesParMinute: 1000,
	}, pool)
	if err != nil {
		t.Fatalf("NewServeur : %v", err)
	}
	return s.Handler()
}

func TestBaremeListe(t *testing.T) {
	h := serveurAvecBase(t)

	w := appeler(h, "GET", "/v1/baremes", "cle-de-test")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d : %s", w.Code, w.Body.String())
	}
	var corps struct {
		Count   int `json:"count"`
		Baremes []struct {
			Categorie string `json:"categorie"`
			Tem       string `json:"tem"`
			Arrete    string `json:"arrete"`
		} `json:"baremes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
		t.Fatalf("reponse illisible : %v", err)
	}
	if corps.Count != 8 || len(corps.Baremes) != 8 {
		t.Fatalf("count=%d, %d lignes, attendu 8", corps.Count, len(corps.Baremes))
	}
	for _, b := range corps.Baremes {
		if b.Arrete == "" {
			t.Errorf("%s : arrete absent", b.Categorie)
		}
	}

	if got := appeler(h, "GET", "/v1/baremes", "").Code; got != http.StatusUnauthorized {
		t.Errorf("sans cle : HTTP %d, attendu 401", got)
	}
}

// La categorie doit resoudre le taux dans le bareme et le verdict citer
// l'arrete applique.
func TestCategorieResoutLeTauxEtCiteLArrete(t *testing.T) {
	h := serveurAvecBase(t)

	w := appeler(h, "GET",
		"/v1/loans/schedule?capital=60000.000&taux=13&mois=60&categorie=credits_consommation",
		"cle-de-test")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d : %s", w.Code, w.Body.String())
	}

	var corps struct {
		Bareme *struct {
			Categorie string `json:"categorie"`
			Semestre  string `json:"semestre"`
			Tem       string `json:"tem"`
			Arrete    string `json:"arrete"`
		} `json:"bareme"`
		Recapitulatif struct {
			Teg      json.Number `json:"teg"`
			Tem      json.Number `json:"tem"`
			Seuil    json.Number `json:"seuil_excessif"`
			Conforme *bool       `json:"conforme"`
		} `json:"recapitulatif"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
		t.Fatalf("reponse illisible : %v", err)
	}

	if corps.Bareme == nil {
		t.Fatal("le bareme applique devrait etre cite")
	}
	if corps.Bareme.Categorie != "credits_consommation" || corps.Bareme.Tem != "11.23" {
		t.Errorf("bareme %+v", corps.Bareme)
	}
	if corps.Bareme.Arrete == "" || corps.Bareme.Semestre == "" {
		t.Error("l'arrete et le semestre doivent etre cites")
	}
	// Le TEM du bareme doit etre celui applique par le moteur.
	if corps.Recapitulatif.Tem.String() != "11.23" {
		t.Errorf("tem applique %s, bareme 11.23", corps.Recapitulatif.Tem)
	}
	// Seuil = TEM majore d'un cinquieme.
	if corps.Recapitulatif.Seuil.String() != "13.48" {
		t.Errorf("seuil %s, attendu 13.48", corps.Recapitulatif.Seuil)
	}
	if corps.Recapitulatif.Conforme == nil || !*corps.Recapitulatif.Conforme {
		t.Errorf("un TEG de %s devrait passer sous 13.48", corps.Recapitulatif.Teg)
	}
}

// Sans categorie ni TEM, aucun verdict, et aucun bareme cite.
func TestSansCategorieAucunBaremeCite(t *testing.T) {
	h := serveurAvecBase(t)

	w := appeler(h, "GET",
		"/v1/loans/schedule?capital=60000.000&taux=13&mois=60", "cle-de-test")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d", w.Code)
	}
	var corps map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
		t.Fatalf("reponse illisible : %v", err)
	}
	if _, present := corps["bareme"]; present {
		t.Error("aucun bareme ne devrait etre cite sans categorie")
	}
}

func TestCategorieRefuseeQuandInconnueOuRedondante(t *testing.T) {
	h := serveurAvecBase(t)
	base := "/v1/loans/schedule?capital=60000.000&taux=13&mois=60"

	cas := []struct{ nom, requete string }{
		{"categorie inconnue", base + "&categorie=credits_lunaires"},
		// La categorie determine le taux : accepter les deux ouvrirait la
		// porte a un verdict rendu sur un taux different de celui annonce.
		{"categorie et tem ensemble", base + "&categorie=credits_consommation&tem=9.99"},
	}
	for _, c := range cas {
		w := appeler(h, "GET", c.requete, "cle-de-test")
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s : HTTP %d, attendu 400", c.nom, w.Code)
			continue
		}
		var corps struct {
			Champ string `json:"champ"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &corps); err != nil {
			t.Errorf("%s : reponse illisible", c.nom)
			continue
		}
		if corps.Champ != "categorie" {
			t.Errorf("%s : champ %q, attendu \"categorie\"", c.nom, corps.Champ)
		}
	}
}

// Sans base, le parametre categorie doit etre refuse clairement plutot que
// silencieusement ignore.
func TestCategorieRefuseeSansBase(t *testing.T) {
	h := serveurDeTest(t, nil)

	w := appeler(h, "GET",
		"/v1/loans/schedule?capital=60000.000&taux=13&mois=60&categorie=credits_consommation",
		"cle-de-test")
	if w.Code != http.StatusBadRequest {
		t.Errorf("HTTP %d, attendu 400 : %s", w.Code, w.Body.String())
	}
	if got := appeler(h, "GET", "/v1/baremes", "cle-de-test").Code; got != http.StatusServiceUnavailable {
		t.Errorf("/v1/baremes sans base : HTTP %d, attendu 503", got)
	}
}
