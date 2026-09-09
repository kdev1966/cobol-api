package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kdev1966/cobol-api/internal/db"
)

// Le frontend etant servi depuis une autre origine, la session voyage dans
// l'en-tete Authorization plutot que dans un cookie. Un cookie transmis entre
// origines exigerait SameSite=None, donc une protection CSRF ; un jeton
// porteur n'est jamais envoye par le navigateur de lui-meme, ce qui supprime
// cette classe d'attaque.
//
// La contrepartie est qu'un script injecte dans le frontend peut lire le
// jeton. Le frontend doit donc le garder en memoire, et non dans
// localStorage, ou il survivrait a la fermeture de l'onglet.
const prefixePorteur = "Bearer "

// cleAgent est la cle sous laquelle l'agent authentifie voyage dans le
// contexte. Un type prive evite toute collision avec une autre cle.
type cleAgent struct{}

// AgentDuContexte rend l'agent authentifie, ou nil.
func AgentDuContexte(ctx context.Context) *db.Agent {
	agent, _ := ctx.Value(cleAgent{}).(*db.Agent)
	return agent
}

// jetonDeLaRequete extrait le jeton porteur de l'en-tete Authorization.
func jetonDeLaRequete(r *http.Request) string {
	entete := r.Header.Get("Authorization")
	if len(entete) <= len(prefixePorteur) ||
		!strings.EqualFold(entete[:len(prefixePorteur)], prefixePorteur) {
		return ""
	}
	return strings.TrimSpace(entete[len(prefixePorteur):])
}

// gardeSession exige une session d'agent valable et depose l'agent dans le
// contexte. Elle est distincte de gardeCle : la cle d'API identifie un
// systeme appelant, la session identifie une personne.
func (s *Serveur) gardeSession(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.agents == nil {
			ecrireErreur(w, http.StatusServiceUnavailable,
				"Comptes indisponibles")
			return
		}

		jeton := jetonDeLaRequete(r)
		if jeton == "" {
			ecrireErreur(w, http.StatusUnauthorized,
				"Jeton de session absent")
			return
		}

		agent, err := s.agents.AgentDeSession(r.Context(), jeton)
		if err != nil {
			if errors.Is(err, db.ErrSessionInconnue) {
				ecrireErreur(w, http.StatusUnauthorized,
					"Session inconnue ou expiree")
				return
			}
			slog.Error("lecture de la session",
				"id", IDRequete(r.Context()), "erreur", err)
			ecrireErreur(w, http.StatusInternalServerError, "Erreur interne")
			return
		}

		ctx := context.WithValue(r.Context(), cleAgent{}, agent)
		suivant.ServeHTTP(w, r.WithContext(ctx))
	})
}
