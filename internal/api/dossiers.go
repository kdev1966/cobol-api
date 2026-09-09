package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kdev1966/cobol-api/internal/db"
	"github.com/kdev1966/cobol-api/internal/loan"
)

// corpsDossier est la saisie acceptee a la creation comme a la modification.
// Aucun champ ne porte de donnee a caractere personnel : la reference renvoie
// au systeme de la banque, ou l'identite de l'emprunteur reste.
type corpsDossier struct {
	Reference   string  `json:"reference"`
	Capital     string  `json:"capital"`
	Taux        string  `json:"taux"`
	Mois        int     `json:"mois"`
	Methode     string  `json:"methode"`
	Differe     int     `json:"differe"`
	TypeDiffere string  `json:"type_differe"`
	Categorie   *string `json:"categorie"`
}

// versDossier valide la saisie en la passant par le meme analyseur que le
// moteur de calcul : un dossier qui ne produirait pas d'echeancier n'a pas
// lieu d'etre enregistre.
func (c corpsDossier) versDossier() (db.Dossier, error) {
	if c.Reference == "" {
		return db.Dossier{}, &loan.ErreurValidation{
			Champ: "reference", Message: "est requise"}
	}
	if len(c.Reference) > 64 {
		return db.Dossier{}, &loan.ErreurValidation{
			Champ: "reference", Message: "fait au plus 64 caracteres"}
	}

	methode := c.Methode
	if methode == "" {
		methode = loan.MethodeParDefaut
	}
	typeDiffere := c.TypeDiffere
	if typeDiffere == "" {
		typeDiffere = loan.TypeDiffereParDefaut
	}

	params := loan.Parametres{
		Capital: c.Capital, Taux: c.Taux,
		Mois:    strconv.Itoa(c.Mois),
		Methode: methode, Differe: strconv.Itoa(c.Differe),
	}
	if c.Differe > 0 {
		params.TypeDiffere = typeDiffere
	} else if typeDiffere != loan.TypeDiffereParDefaut {
		return db.Dossier{}, &loan.ErreurValidation{
			Champ: "type_differe", Message: "sans objet sans differe"}
	}
	if c.Categorie != nil {
		params.Categorie = *c.Categorie
	}

	demande, err := loan.ParseDemande(params)
	if err != nil {
		return db.Dossier{}, err
	}

	return db.Dossier{
		Reference: c.Reference,
		// Les valeurs normalisees sont enregistrees, non la saisie brute :
		// « 250000 » et « 250000.000 » designent le meme pret.
		Capital: demande.Capital, Taux: demande.Taux, Mois: demande.Mois,
		Methode: demande.Methode, Differe: demande.DiffereMois,
		TypeDiffere: typeDiffere, Categorie: c.Categorie,
	}, nil
}

// repondreDossier traduit les erreurs du magasin en codes HTTP.
func (s *Serveur) repondreErreurDossier(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrDossierAbsent):
		ecrireErreur(w, http.StatusNotFound, "Dossier introuvable")
	case errors.Is(err, db.ErrReferencePrise):
		ecrireErreur(w, http.StatusConflict,
			"Cette reference est deja employee dans l'agence")
	case errors.Is(err, db.ErrDossierFige):
		ecrireErreur(w, http.StatusConflict,
			"Le dossier n'est plus modifiable : seul un brouillon l'est")
	case errors.Is(err, db.ErrTransitionRefusee):
		ecrireErreur(w, http.StatusConflict, err.Error())
	default:
		slog.Error("dossier", "id", IDRequete(r.Context()), "erreur", err)
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
	}
}

func (s *Serveur) creerDossier(w http.ResponseWriter, r *http.Request) {
	agent := AgentDuContexte(r.Context())

	var corps corpsDossier
	if err := lireJSON(w, r, &corps); err != nil {
		ecrireErreur(w, http.StatusBadRequest, err.Error())
		return
	}
	saisie, err := corps.versDossier()
	if err != nil {
		s.repondreValidation(w, err)
		return
	}
	if err := s.verifierLaCategorie(r, saisie.Categorie); err != nil {
		s.repondreValidation(w, err)
		return
	}

	dossier, err := s.dossiers.Creer(r.Context(), agent, saisie)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}

	slog.Info("dossier cree", "id", IDRequete(r.Context()),
		"dossier", dossier.ID, "agent", agent.ID)
	ecrireJSON(w, http.StatusCreated, map[string]any{
		"status": "success", "dossier": dossier})
}

func (s *Serveur) listerDossiers(w http.ResponseWriter, r *http.Request) {
	agent := AgentDuContexte(r.Context())
	q := r.URL.Query()

	statut := q.Get("statut")
	if statut != "" && !contient(db.StatutsAcceptes(), statut) {
		s.repondreValidation(w, &loan.ErreurValidation{Champ: "statut",
			Message: "doit valoir " + strings.Join(db.StatutsAcceptes(), ", ")})
		return
	}

	limite, _ := strconv.Atoi(q.Get("limite"))
	liste, err := s.dossiers.Lister(r.Context(), agent, statut, limite)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status": "success", "dossiers": liste, "total": len(liste)})
}

func (s *Serveur) lireDossier(w http.ResponseWriter, r *http.Request) {
	agent := AgentDuContexte(r.Context())
	id, ok := identifiantDeLURL(w, r)
	if !ok {
		return
	}

	dossier, err := s.dossiers.Lire(r.Context(), agent, id)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}
	historique, err := s.dossiers.Historique(r.Context(), agent, id)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status": "success", "dossier": dossier, "historique": historique})
}

func (s *Serveur) modifierDossier(w http.ResponseWriter, r *http.Request) {
	agent := AgentDuContexte(r.Context())
	id, ok := identifiantDeLURL(w, r)
	if !ok {
		return
	}

	var corps corpsDossier
	if err := lireJSON(w, r, &corps); err != nil {
		ecrireErreur(w, http.StatusBadRequest, err.Error())
		return
	}
	saisie, err := corps.versDossier()
	if err != nil {
		s.repondreValidation(w, err)
		return
	}
	if err := s.verifierLaCategorie(r, saisie.Categorie); err != nil {
		s.repondreValidation(w, err)
		return
	}

	dossier, err := s.dossiers.Modifier(r.Context(), agent, id, saisie)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status": "success", "dossier": dossier})
}

type corpsStatut struct {
	Statut string `json:"statut"`
	Note   string `json:"note"`
}

func (s *Serveur) changerLeStatut(w http.ResponseWriter, r *http.Request) {
	agent := AgentDuContexte(r.Context())
	id, ok := identifiantDeLURL(w, r)
	if !ok {
		return
	}

	var corps corpsStatut
	if err := lireJSON(w, r, &corps); err != nil {
		ecrireErreur(w, http.StatusBadRequest, err.Error())
		return
	}
	if !contient(db.StatutsAcceptes(), corps.Statut) {
		s.repondreValidation(w, &loan.ErreurValidation{Champ: "statut",
			Message: "doit valoir " + strings.Join(db.StatutsAcceptes(), ", ")})
		return
	}
	if len(corps.Note) > 1000 {
		s.repondreValidation(w, &loan.ErreurValidation{Champ: "note",
			Message: "fait au plus 1000 caracteres"})
		return
	}

	dossier, err := s.dossiers.ChangerLeStatut(r.Context(), agent, id,
		corps.Statut, corps.Note)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}

	slog.Info("statut change", "id", IDRequete(r.Context()),
		"dossier", id, "agent", agent.ID, "statut", corps.Statut)
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status": "success", "dossier": dossier})
}

// echeancierDossier calcule l'echeancier du dossier et l'y rattache. Le calcul
// passe par le meme chemin que la route publique : un dossier ne dispose pas
// d'un moteur a part.
func (s *Serveur) echeancierDossier(w http.ResponseWriter, r *http.Request) {
	agent := AgentDuContexte(r.Context())
	id, ok := identifiantDeLURL(w, r)
	if !ok {
		return
	}

	dossier, err := s.dossiers.Lire(r.Context(), agent, id)
	if err != nil {
		s.repondreErreurDossier(w, r, err)
		return
	}

	params := loan.Parametres{
		Capital: dossier.Capital, Taux: dossier.Taux,
		Mois:    strconv.Itoa(dossier.Mois),
		Methode: dossier.Methode,
		Differe: strconv.Itoa(dossier.Differe),
	}
	if dossier.Differe > 0 {
		params.TypeDiffere = dossier.TypeDiffere
	}
	if dossier.Categorie != nil {
		params.Categorie = *dossier.Categorie
	}

	// La simulation produite est rattachee au dossier : c'est elle qui porte
	// le verdict de taux excessif sur lequel la decision s'appuiera.
	s.produireEcheancier(w, r, params, map[string]any{"dossier": dossier},
		func(simulation int64) error {
			return s.dossiers.RattacherLaSimulation(r.Context(), agent, id,
				simulation)
		})
}

// verifierLaCategorie refuse a l'enregistrement une categorie que le bareme ne
// connait pas. Sans ce controle, le dossier s'enregistrerait sans bruit et
// n'echouerait qu'au calcul de son echeancier, bien plus tard.
func (s *Serveur) verifierLaCategorie(r *http.Request, categorie *string) error {
	if categorie == nil || *categorie == "" {
		return nil
	}
	_, err := s.resoudreTem(r, *categorie)
	return err
}

func identifiantDeLURL(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		ecrireErreur(w, http.StatusBadRequest, "Identifiant de dossier invalide")
		return 0, false
	}
	return id, true
}

func contient(liste []string, valeur string) bool {
	for _, v := range liste {
		if v == valeur {
			return true
		}
	}
	return false
}

// exigeDossiers refuse les routes de dossiers quand la base est absente,
// plutot que de laisser un pointeur nul se manifester plus loin.
func (s *Serveur) exigeDossiers(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.dossiers == nil {
			ecrireErreur(w, http.StatusServiceUnavailable,
				"Dossiers indisponibles")
			return
		}
		suivant.ServeHTTP(w, r)
	})
}
