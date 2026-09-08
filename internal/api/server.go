// Package api expose le moteur d'amortissement en HTTP.
package api

import (
	_ "embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/kdev1966/cobol-api/internal/loan"
)

// specificationOpenAPI est embarquee dans le binaire : la specification est
// versionnee avec le code qu'elle decrit, et ne peut pas en diverger a
// l'execution.
//
//go:embed openapi.json
var specificationOpenAPI []byte

// Config rassemble ce que le service lit dans son environnement.
type Config struct {
	Port              string
	CheminProgramme   string
	CleAPI            string
	RequetesParMinute int
	OriginesCORS      []string
	Production        bool
	DelaiCalcul       time.Duration
}

// ConfigDepuisEnv lit la configuration, en appliquant les defauts.
func ConfigDepuisEnv() Config {
	return Config{
		Port:              valeurOuDefaut("PORT", "3000"),
		CheminProgramme:   valeurOuDefaut("COBOL_PROGRAM_PATH", "/app/bin/loan_amortization"),
		CleAPI:            os.Getenv("API_KEY"),
		RequetesParMinute: entierOuDefaut("RATE_LIMIT_PER_MINUTE", 30),
		OriginesCORS:      origines(os.Getenv("CORS_ORIGINS")),
		Production:        os.Getenv("APP_ENV") == "production",
		DelaiCalcul: time.Duration(
			entierOuDefaut("COMPUTE_TIMEOUT_SECONDS", 5)) * time.Second,
	}
}

// Serveur assemble les routes et leurs protections.
type Serveur struct {
	cfg    Config
	moteur *loan.Moteur
	mux    *http.ServeMux
}

// NewServeur verifie la coherence de la configuration puis monte les routes.
func NewServeur(cfg Config) (*Serveur, error) {
	if cfg.CleAPI == "" && cfg.Production {
		return nil, errors.New(
			"API_KEY est obligatoire en production : /loans/schedule expose un moteur de calcul de credit")
	}
	if cfg.CleAPI == "" {
		slog.Warn("API_KEY absente : les endpoints proteges sont ouverts, " +
			"ne pas exploiter ainsi hors developpement")
	}
	if _, err := os.Stat(cfg.CheminProgramme); err != nil {
		return nil, errors.New("binaire COBOL introuvable : " + cfg.CheminProgramme +
			"\nLe compiler avec : cobc -x -free cobol/loan-amortization.cbl -o " + cfg.CheminProgramme)
	}

	s := &Serveur{
		cfg:    cfg,
		moteur: loan.NewMoteur(cfg.CheminProgramme, cfg.DelaiCalcul),
		mux:    http.NewServeMux(),
	}
	s.monterRoutes()
	return s, nil
}

func (s *Serveur) monterRoutes() {
	lim := nouveauLimiteur(s.cfg.RequetesParMinute)
	protege := func(h http.HandlerFunc) http.Handler {
		return enchainer(h, lim.intergiciel, gardeCle(s.cfg.CleAPI))
	}

	// /health reste ouvert et hors plafond : le HEALTHCHECK du conteneur
	// s'appuie dessus. L'index et la specification le sont aussi : ils ne
	// divulguent rien de plus que le README.
	s.mux.Handle("GET /health", http.HandlerFunc(s.sante))
	s.mux.Handle("GET /{$}", http.HandlerFunc(s.index))
	s.mux.Handle("GET /openapi.json", http.HandlerFunc(s.specification))

	// Les routes metier sont versionnees : le format du recapitulatif a
	// deja change une fois, et un client tiers ne doit pas en patir.
	s.mux.Handle("GET /v1/loans/schedule", protege(s.echeancier))
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ecrireErreur(w, http.StatusNotFound, "Ressource inconnue")
	}))
}

// Handler rend le gestionnaire complet, intergiciels globaux compris.
func (s *Serveur) Handler() http.Handler {
	return enchainer(s.mux,
		journal,
		enTetesSecurite,
		corsPolitique(s.cfg.OriginesCORS),
	)
}

func (s *Serveur) index(w http.ResponseWriter, r *http.Request) {
	ecrireJSON(w, http.StatusOK, map[string]any{
		"service":        "cobol-api",
		"description":    "Moteur d'amortissement de pret ecrit en COBOL, expose en REST",
		"authentication": "En-tete X-API-Key sur les endpoints marques auth",
		"version":        "v1",
		"openapi":        "/openapi.json",
		"endpoints": []map[string]any{
			{
				"method":              "GET",
				"path":                "/v1/loans/schedule?capital=&taux=&mois=&methode=&frais_dossier=&frais_garantie=&taux_assurance=&assiette_assurance=&taux_usure=",
				"auth":                true,
				"description":         "Echeancier de pret",
				"methodes":            loan.MethodesAcceptees(),
				"methode_par_defaut":  loan.MethodeParDefaut,
				"assiettes_assurance": loan.AssiettesAcceptees(),
			},
			{
				"method":      "GET",
				"path":        "/health",
				"auth":        false,
				"description": "Etat du service",
			},
		},
	})
}

func (s *Serveur) specification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(specificationOpenAPI); err != nil {
		slog.Error("ecriture de la specification", "erreur", err)
	}
}

// sante verifie que le binaire est toujours la. Une reponse inconditionnelle
// ne servirait a rien au HEALTHCHECK.
func (s *Serveur) sante(w http.ResponseWriter, r *http.Request) {
	_, err := os.Stat(s.cfg.CheminProgramme)
	sain := err == nil

	code := http.StatusOK
	etat := "OK"
	if !sain {
		code = http.StatusServiceUnavailable
		etat = "ERROR"
	}

	ecrireJSON(w, code, map[string]any{
		"status":        etat,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
		"programExists": sain,
	})
}

func (s *Serveur) echeancier(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	demande, err := loan.ParseDemande(loan.Parametres{
		Capital:       q.Get("capital"),
		Taux:          q.Get("taux"),
		Mois:          q.Get("mois"),
		Methode:       q.Get("methode"),
		FraisDossier:  q.Get("frais_dossier"),
		FraisGarantie: q.Get("frais_garantie"),
		TauxAssurance: q.Get("taux_assurance"),
		Assiette:      q.Get("assiette_assurance"),
		TauxUsure:     q.Get("taux_usure"),
	})
	if err != nil {
		var invalide *loan.ErreurValidation
		if errors.As(err, &invalide) {
			ecrireJSON(w, http.StatusBadRequest, map[string]any{
				"status":  "error",
				"champ":   invalide.Champ,
				"message": invalide.Message,
			})
			return
		}
		ecrireErreur(w, http.StatusBadRequest, "Parametres invalides")
		return
	}

	resultat, err := s.moteur.Calculer(r.Context(), demande)
	if err != nil {
		// Le detail reste dans les journaux : il porte des chemins absolus
		// et la sortie d'erreur du programme.
		slog.Error("calcul de l'echeancier",
			"id", IDRequete(r.Context()), "erreur", err)
		if errors.Is(err, loan.ErrDelaiDepasse) {
			ecrireErreur(w, http.StatusGatewayTimeout, "Le calcul a depasse son delai")
			return
		}
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]any{
		"status":        "success",
		"demande":       demande,
		"recapitulatif": resultat.Recapitulatif,
		"echeancier":    resultat.Echeancier,
	})
}

func ecrireJSON(w http.ResponseWriter, code int, corps any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(corps); err != nil {
		slog.Error("ecriture de la reponse", "erreur", err)
	}
}

func ecrireErreur(w http.ResponseWriter, code int, message string) {
	ecrireJSON(w, code, map[string]any{"status": "error", "message": message})
}

func valeurOuDefaut(cle, defaut string) string {
	if v := os.Getenv(cle); v != "" {
		return v
	}
	return defaut
}

func entierOuDefaut(cle string, defaut int) int {
	v := os.Getenv(cle)
	if v == "" {
		return defaut
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaut
	}
	return n
}
