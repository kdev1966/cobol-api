package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kdev1966/cobol-api/internal/db"
)

// tailleMaxCorps borne le corps des requetes JSON. Les charges utiles de ce
// service tiennent en quelques centaines d'octets ; le plafond evite qu'un
// client n'occupe de la memoire indefiniment.
const tailleMaxCorps = 16 << 10

// lireJSON decode le corps d'une requete, en refusant les champs inconnus :
// une faute de frappe dans un nom de champ doit se voir, plutot que d'etre
// ignoree en silence.
func lireJSON(w http.ResponseWriter, r *http.Request, cible any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" &&
		!strings.HasPrefix(ct, "application/json") {
		return errors.New("Content-Type application/json attendu")
	}

	decodeur := json.NewDecoder(io.LimitReader(r.Body, tailleMaxCorps))
	decodeur.DisallowUnknownFields()
	if err := decodeur.Decode(cible); err != nil {
		return errors.New("corps JSON illisible")
	}
	return nil
}

type demandeConnexion struct {
	Identifiant string `json:"identifiant"`
	MotDePasse  string `json:"mot_de_passe"`
}

// connexion ouvre une session d'agent. Elle est deliberement hors de la garde
// par cle d'API : le frontend n'en detient pas, il s'authentifie au nom d'une
// personne.
func (s *Serveur) connexion(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		ecrireErreur(w, http.StatusServiceUnavailable, "Comptes indisponibles")
		return
	}

	var demande demandeConnexion
	if err := lireJSON(w, r, &demande); err != nil {
		ecrireErreur(w, http.StatusBadRequest, err.Error())
		return
	}
	if demande.Identifiant == "" || demande.MotDePasse == "" {
		ecrireErreur(w, http.StatusBadRequest,
			"identifiant et mot_de_passe sont requis")
		return
	}

	jeton, agent, err := s.agents.Authentifier(r.Context(),
		demande.Identifiant, demande.MotDePasse,
		adresseAppelant(r), r.Header.Get("User-Agent"))
	if err != nil {
		if errors.Is(err, db.ErrIdentifiantsRefuses) {
			// Le journal garde l'identifiant tente : une rafale d'echecs sur
			// le meme compte doit pouvoir se reperer. Le mot de passe, lui,
			// n'est jamais journalise.
			slog.Warn("connexion refusee", "id", IDRequete(r.Context()),
				"identifiant", demande.Identifiant)
			ecrireErreur(w, http.StatusUnauthorized,
				"Identifiant ou mot de passe incorrect")
			return
		}
		slog.Error("connexion", "id", IDRequete(r.Context()), "erreur", err)
		ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
		return
	}

	slog.Info("connexion", "id", IDRequete(r.Context()),
		"agent", agent.ID, "identifiant", agent.Identifiant)

	ecrireJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"jeton":     jeton,
		"expire_le": time.Now().UTC().Add(db.DureeSession).Format(time.RFC3339),
		"agent":     agent,
	})
}

// deconnexion revoque la session portee par la requete.
func (s *Serveur) deconnexion(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		ecrireErreur(w, http.StatusServiceUnavailable, "Comptes indisponibles")
		return
	}

	// Revoquer un jeton deja absent n'est pas une erreur : le resultat voulu
	// est atteint, et distinguer les deux cas renseignerait un attaquant sur
	// la validite d'un jeton.
	if jeton := jetonDeLaRequete(r); jeton != "" {
		if err := s.agents.Deconnecter(r.Context(), jeton); err != nil {
			slog.Error("deconnexion", "id", IDRequete(r.Context()), "erreur", err)
			ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// moi rend le titulaire de la session, ce qui permet au frontend de savoir au
// demarrage si son jeton vaut encore.
func (s *Serveur) moi(w http.ResponseWriter, r *http.Request) {
	ecrireJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"agent":  AgentDuContexte(r.Context()),
	})
}

// adresseAppelant rend l'adresse IP de l'appelant, sans le port.
func adresseAppelant(r *http.Request) string {
	hote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return hote
}
