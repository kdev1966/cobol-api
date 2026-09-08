// Package api expose le moteur d'amortissement en HTTP.
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/kdev1966/cobol-api/internal/loan"
)

// Config rassemble ce que le service lit dans son environnement.
type Config struct {
	Port              string
	CheminProgramme   string
	CleAPI            string
	RequetesParMinute int
	OriginesCORS      []string
	Production        bool
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
		log.Println("API_KEY absente : les endpoints proteges sont ouverts. Ne pas exploiter ainsi hors developpement.")
	}
	if _, err := os.Stat(cfg.CheminProgramme); err != nil {
		return nil, errors.New("binaire COBOL introuvable : " + cfg.CheminProgramme +
			"\nLe compiler avec : cobc -x -free cobol/loan-amortization.cbl -o " + cfg.CheminProgramme)
	}

	s := &Serveur{
		cfg:    cfg,
		moteur: loan.NewMoteur(cfg.CheminProgramme),
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
	// s'appuie dessus.
	s.mux.Handle("GET /health", http.HandlerFunc(s.sante))
	s.mux.Handle("GET /{$}", http.HandlerFunc(s.index))
	s.mux.Handle("GET /loans/schedule", protege(s.echeancier))
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
		"endpoints": []map[string]any{
			{
				"method":      "GET",
				"path":        "/loans/schedule?capital=&taux=&mois=",
				"auth":        true,
				"description": "Echeancier a mensualite constante",
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
	demande, err := loan.ParseDemande(q.Get("capital"), q.Get("taux"), q.Get("mois"))
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
		log.Printf("calcul de l'echeancier : %v", err)
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
		log.Printf("ecriture de la reponse : %v", err)
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
