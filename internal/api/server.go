// Package api expose le moteur d'amortissement en HTTP.
package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kdev1966/cobol-api/internal/db"
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
	CheminCapacite    string
	CleAPI            string
	RequetesParMinute int
	OriginesCORS      []string
	Production        bool
	DelaiCalcul       time.Duration
	DatabaseURL       string
}

// ConfigDepuisEnv lit la configuration, en appliquant les defauts.
func ConfigDepuisEnv() Config {
	return Config{
		Port:              valeurOuDefaut("PORT", "3000"),
		CheminProgramme:   valeurOuDefaut("COBOL_PROGRAM_PATH", "/app/bin/loan_amortization"),
		CheminCapacite:    valeurOuDefaut("COBOL_CAPACITY_PATH", "/app/bin/loan_capacity"),
		CleAPI:            os.Getenv("API_KEY"),
		RequetesParMinute: entierOuDefaut("RATE_LIMIT_PER_MINUTE", 30),
		OriginesCORS:      origines(os.Getenv("CORS_ORIGINS")),
		Production:        os.Getenv("APP_ENV") == "production",
		DelaiCalcul: time.Duration(
			entierOuDefaut("COMPUTE_TIMEOUT_SECONDS", 5)) * time.Second,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
}

// Serveur assemble les routes et leurs protections.
type Serveur struct {
	cfg    Config
	moteur *loan.Moteur
	// base peut etre nil : /health ne sonde alors pas la base. En service
	// elle est toujours fournie.
	base *db.Pool
	// baremes est nil quand aucune base n'est fournie : le parametre
	// categorie est alors refuse et le TEM doit etre passe directement.
	baremes *db.Baremes
	// audit est nil sans base : le service refuse alors les routes qui en
	// dependent plutot que de produire des offres sans trace.
	audit *db.Simulations
	mux   *http.ServeMux
}

// NewServeur verifie la coherence de la configuration puis monte les routes.
func NewServeur(cfg Config, base *db.Pool) (*Serveur, error) {
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
		moteur: loan.NewMoteur(cfg.CheminProgramme, cfg.CheminCapacite, cfg.DelaiCalcul),
		base:   base,
		mux:    http.NewServeMux(),
	}
	if base != nil {
		s.baremes = db.NewBaremes(base)
		s.audit = db.NewSimulations(base)
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
	s.mux.Handle("GET /v1/loans/capacity", protege(s.capacite))
	s.mux.Handle("GET /v1/baremes", protege(s.bareme))
	s.mux.Handle("GET /v1/simulations", protege(s.simulations))
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
				"path":                "/v1/loans/schedule?capital=&taux=&mois=&methode=&frais_dossier=&frais_garantie=&taux_assurance=&assiette_assurance=&differe=&mois_remboursement_anticipe=&indemnite=&montant_remboursement_anticipe=&mode_remboursement_anticipe=&categorie=",
				"auth":                true,
				"description":         "Echeancier de pret",
				"methodes":            loan.MethodesAcceptees(),
				"methode_par_defaut":  loan.MethodeParDefaut,
				"assiettes_assurance": loan.AssiettesAcceptees(),
				"modes_anticipe":      loan.ModesAcceptes(),
				"mode_par_defaut":     loan.ModeParDefaut,
			},
			{
				"method":      "GET",
				"path":        "/v1/loans/capacity?mensualite=&taux=&mois=&methode=",
				"auth":        true,
				"description": "Capital maximal empruntable pour une mensualite donnee",
			},
			{
				"method":      "GET",
				"path":        "/v1/baremes",
				"auth":        true,
				"description": "Taux effectifs moyens en vigueur, par categorie de concours",
			},
			{
				"method":      "GET",
				"path":        "/v1/simulations?limite=&non_conformes=",
				"auth":        true,
				"description": "Piste d'audit des echeanciers produits",
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

// sante verifie que le binaire et la base repondent. Une reponse
// inconditionnelle ne servirait a rien au HEALTHCHECK.
func (s *Serveur) sante(w http.ResponseWriter, r *http.Request) {
	_, err := os.Stat(s.cfg.CheminProgramme)
	programme := err == nil

	corps := map[string]any{
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
		"programExists": programme,
	}
	sain := programme

	if s.base != nil {
		ctx, annuler := context.WithTimeout(r.Context(), 2*time.Second)
		defer annuler()
		err := s.base.Ping(ctx)
		if err != nil {
			slog.Error("health check, base injoignable",
				"id", IDRequete(r.Context()), "erreur", err)
		}
		corps["databaseReachable"] = err == nil
		sain = sain && err == nil
	}

	code := http.StatusOK
	corps["status"] = "OK"
	if !sain {
		code = http.StatusServiceUnavailable
		corps["status"] = "ERROR"
	}

	ecrireJSON(w, code, corps)
}

// tracer inscrit la simulation dans la piste d'audit. Sans base, il n'y a rien
// a inscrire et l'identifiant rendu est nul.
func (s *Serveur) tracer(r *http.Request, demande loan.Demande,
	resultat *loan.Echeancier, bareme *db.TauxEffectif) (int64, error) {
	if s.audit == nil {
		return 0, nil
	}

	brut, err := json.Marshal(demande)
	if err != nil {
		return 0, fmt.Errorf("serialisation de la demande : %w", err)
	}

	sim := db.Simulation{
		RequeteID:          IDRequete(r.Context()),
		EmpreinteCle:       EmpreinteCle(r.Header.Get("X-API-Key")),
		Adresse:            adresseClient(r),
		Demande:            brut,
		Teg:                resultat.Recapitulatif.Teg.String(),
		CoutCredit:         resultat.Recapitulatif.CoutCredit.String(),
		PremiereMensualite: resultat.Recapitulatif.PremiereMensualite.String(),
		Conforme:           resultat.Recapitulatif.Conforme,
	}
	if resultat.Recapitulatif.Tem != nil {
		tem := resultat.Recapitulatif.Tem.String()
		seuil := resultat.Recapitulatif.Seuil.String()
		sim.Tem, sim.Seuil = &tem, &seuil
	}
	if bareme != nil {
		sim.Categorie, sim.Semestre, sim.Arrete =
			&bareme.Categorie, &bareme.Semestre, &bareme.Arrete
	}

	return s.audit.Enregistrer(r.Context(), sim)
}

// simulations rend la piste d'audit, de la plus recente a la plus ancienne.
func (s *Serveur) simulations(w http.ResponseWriter, r *http.Request) {
	if s.audit == nil {
		ecrireErreur(w, http.StatusServiceUnavailable, "Piste d'audit indisponible")
		return
	}

	limite := 20
	if brut := r.URL.Query().Get("limite"); brut != "" {
		n, err := strconv.Atoi(brut)
		if err != nil || n < 1 || n > 200 {
			ecrireJSON(w, http.StatusBadRequest, map[string]any{
				"status": "error", "champ": "limite",
				"message": "doit etre un entier entre 1 et 200",
			})
			return
		}
		limite = n
	}
	nonConformes := r.URL.Query().Get("non_conformes") == "true"

	toutes, err := s.audit.Lister(r.Context(), limite, nonConformes)
	if err != nil {
		slog.Error("lecture de la piste d'audit", "id", IDRequete(r.Context()), "erreur", err)
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status":      "success",
		"count":       len(toutes),
		"simulations": toutes,
	})
}

// capacite rend le capital maximal empruntable pour une mensualite donnee,
// accompagne de l'echeancier qu'il produit : l'emprunteur veut savoir combien
// il peut emprunter, mais aussi ce que ca donne.
func (s *Serveur) capacite(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if s.cfg.CheminCapacite == "" {
		ecrireErreur(w, http.StatusServiceUnavailable, "Calcul de capacite indisponible")
		return
	}

	params := loan.Parametres{
		Mensualite:    q.Get("mensualite"),
		Taux:          q.Get("taux"),
		Mois:          q.Get("mois"),
		Methode:       q.Get("methode"),
		TauxAssurance: q.Get("taux_assurance"),
		Assiette:      q.Get("assiette_assurance"),
		Differe:       q.Get("differe"),
	}
	budget, err := loan.ParseDemandeCapacite(params)
	if err != nil {
		s.repondreValidation(w, err)
		return
	}

	capacite, err := s.moteur.Capaciter(r.Context(), budget)
	if err != nil {
		slog.Error("calcul de capacite", "id", IDRequete(r.Context()), "erreur", err)
		if errors.Is(err, loan.ErrDelaiDepasse) {
			ecrireErreur(w, http.StatusGatewayTimeout, "Le calcul a depasse son delai")
			return
		}
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}

	// Le capital trouve est ensuite deroule : le calcul inverse ne dispense
	// pas de produire l'echeancier, ni d'en juger le taux.
	params.Capital = capacite.Capital.String()
	params.Differe = q.Get("differe")
	params.Tem, params.Categorie = q.Get("tem"), q.Get("categorie")
	s.produireEcheancier(w, r, params, map[string]any{"capacite": capacite})
}

// bareme rend les taux effectifs moyens en vigueur, une ligne par categorie.
func (s *Serveur) bareme(w http.ResponseWriter, r *http.Request) {
	if s.baremes == nil {
		ecrireErreur(w, http.StatusServiceUnavailable, "Bareme indisponible")
		return
	}
	tous, err := s.baremes.Lister(r.Context())
	if err != nil {
		slog.Error("lecture du bareme", "id", IDRequete(r.Context()), "erreur", err)
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status":  "success",
		"count":   len(tous),
		"baremes": tous,
	})
}

// resoudreTem traduit une categorie de concours en taux effectif moyen. Rendre
// le bareme applique, et non seulement le taux, permet a l'appelant de citer
// l'arrete sur lequel repose le verdict.
func (s *Serveur) resoudreTem(r *http.Request, categorie string) (*db.TauxEffectif, error) {
	if s.baremes == nil {
		return nil, &loan.ErreurValidation{Champ: "categorie",
			Message: "le bareme n'est pas disponible ; fournir tem directement"}
	}
	t, err := s.baremes.TauxEffectifMoyen(r.Context(), categorie)
	if err != nil {
		if errors.Is(err, db.ErrCategorieInconnue) {
			connues, _ := s.baremes.Categories(r.Context())
			return nil, &loan.ErreurValidation{Champ: "categorie",
				Message: "inconnue ; categories du bareme : " + strings.Join(connues, ", ")}
		}
		return nil, err
	}
	return &t, nil
}

// repondreValidation traduit une erreur de validation en 400 nommant le champ.
func (s *Serveur) repondreValidation(w http.ResponseWriter, err error) {
	var invalide *loan.ErreurValidation
	if errors.As(err, &invalide) {
		ecrireJSON(w, http.StatusBadRequest, map[string]any{
			"status": "error", "champ": invalide.Champ, "message": invalide.Message,
		})
		return
	}
	ecrireErreur(w, http.StatusBadRequest, "Parametres invalides")
}

func (s *Serveur) echeancier(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.produireEcheancier(w, r, loan.Parametres{
		Capital:         q.Get("capital"),
		Taux:            q.Get("taux"),
		Mois:            q.Get("mois"),
		Methode:         q.Get("methode"),
		FraisDossier:    q.Get("frais_dossier"),
		FraisGarantie:   q.Get("frais_garantie"),
		TauxAssurance:   q.Get("taux_assurance"),
		Assiette:        q.Get("assiette_assurance"),
		Differe:         q.Get("differe"),
		MoisAnticipe:    q.Get("mois_remboursement_anticipe"),
		Indemnite:       q.Get("indemnite"),
		MontantAnticipe: q.Get("montant_remboursement_anticipe"),
		Mode:            q.Get("mode_remboursement_anticipe"),
		Tem:             q.Get("tem"),
		Categorie:       q.Get("categorie"),
	}, nil)
}

// produireEcheancier resout le bareme, calcule, trace et repond. Le calcul
// inverse s'y branche apres avoir determine le capital.
func (s *Serveur) produireEcheancier(w http.ResponseWriter, r *http.Request,
	params loan.Parametres, supplement map[string]any) {

	// La categorie et le taux effectif moyen designent la meme chose : l'un se
	// lit dans le bareme, l'autre est impose. Les accepter ensemble ouvrirait
	// la porte a un verdict rendu sur un taux qui n'est pas celui annonce.
	categorie, tem := strings.TrimSpace(params.Categorie), params.Tem
	var bareme *db.TauxEffectif
	if categorie != "" {
		if strings.TrimSpace(tem) != "" {
			ecrireJSON(w, http.StatusBadRequest, map[string]any{
				"status": "error", "champ": "categorie",
				"message": "categorie et tem s'excluent : la categorie determine le taux",
			})
			return
		}
		var err error
		bareme, err = s.resoudreTem(r, categorie)
		if err != nil {
			var invalide *loan.ErreurValidation
			if errors.As(err, &invalide) {
				s.repondreValidation(w, err)
				return
			}
			slog.Error("resolution du bareme", "id", IDRequete(r.Context()), "erreur", err)
			ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
			return
		}
		tem = bareme.Tem
	}

	params.Tem = tem
	demande, err := loan.ParseDemande(params)
	if err != nil {
		s.repondreValidation(w, err)
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
		// Certaines regles ne se verifient qu'une fois l'echeancier deroule :
		// le refus vient alors du moteur, mais reste une erreur de saisie.
		var refus *loan.ErreurRefus
		if errors.As(err, &refus) {
			ecrireErreur(w, http.StatusBadRequest, refus.Motif)
			return
		}
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}

	// La piste d'audit est ecrite avant la reponse, et son echec fait echouer
	// la requete. Un service qui produit des offres de credit doit pouvoir
	// dire ce qu'il a produit ; une trace a trous n'en est pas une.
	idSimulation, err := s.tracer(r, demande, resultat, bareme)
	if err != nil {
		slog.Error("piste d'audit", "id", IDRequete(r.Context()), "erreur", err)
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}

	corps := map[string]any{
		"status":        "success",
		"demande":       demande,
		"recapitulatif": resultat.Recapitulatif,
		"echeancier":    resultat.Echeancier,
	}
	if resultat.Anticipe != nil {
		corps["anticipe"] = resultat.Anticipe
	}
	if resultat.Partiel != nil {
		corps["remboursement_partiel"] = resultat.Partiel
	}
	if idSimulation != 0 {
		corps["simulation_id"] = idSimulation
	}
	// Citer l'arrete applique : le verdict de taux excessif ne vaut que
	// rapporte au bareme sur lequel il repose.
	if bareme != nil {
		corps["bareme"] = bareme
	}
	for cle, valeur := range supplement {
		corps[cle] = valeur
	}
	ecrireJSON(w, http.StatusOK, corps)
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
