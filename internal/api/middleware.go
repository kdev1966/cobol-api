package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// enTetesSecurite pose l'equivalent de ce que helmet fournit cote Node. La
// liste est volontairement courte : ces en-tetes suffisent pour une API JSON
// qui ne sert aucune page.
func enTetesSecurite(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		suivant.ServeHTTP(w, r)
	})
}

// corsPolitique n'autorise que les origines explicitement listees. Sans
// liste, aucune origine croisee n'est acceptee : c'est le defaut ferme.
func corsPolitique(origines []string) func(http.Handler) http.Handler {
	autorisees := make(map[string]bool, len(origines))
	for _, o := range origines {
		autorisees[o] = true
	}

	return func(suivant http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origine := r.Header.Get("Origin")
			if origine != "" && autorisees[origine] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origine)
				h.Set("Access-Control-Allow-Headers", "X-API-Key")
				h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				h.Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			suivant.ServeHTTP(w, r)
		})
	}
}

// memeSecret compare deux valeurs a temps constant. Les condensats ont une
// longueur fixe, ce qui evite de divulguer celle de la cle attendue.
func memeSecret(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// gardeCle exige l'en-tete X-API-Key. Sans cle configuree il laisse passer :
// le serveur refuse alors de demarrer en production.
func gardeCle(cle string) func(http.Handler) http.Handler {
	return func(suivant http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cle == "" {
				suivant.ServeHTTP(w, r)
				return
			}
			if memeSecret(r.Header.Get("X-API-Key"), cle) {
				suivant.ServeHTTP(w, r)
				return
			}
			ecrireErreur(w, http.StatusUnauthorized, "Cle d'API absente ou invalide")
		})
	}
}

// limiteur applique un plafond de requetes par minute et par adresse IP.
type limiteur struct {
	mu         sync.Mutex
	parIP      map[string]*compteur
	debit      rate.Limit
	rafale     int
	peremption time.Duration
}

type compteur struct {
	limite *rate.Limiter
	vu     time.Time
}

func nouveauLimiteur(parMinute int) *limiteur {
	return &limiteur{
		parIP:      make(map[string]*compteur),
		debit:      rate.Limit(float64(parMinute) / 60.0),
		rafale:     parMinute,
		peremption: 10 * time.Minute,
	}
}

// autoriser consomme un jeton pour l'adresse donnee. Les compteurs inactifs
// sont purges au passage, sinon la table croitrait sans fin.
func (l *limiteur) autoriser(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	maintenant := time.Now()
	for adresse, c := range l.parIP {
		if maintenant.Sub(c.vu) > l.peremption {
			delete(l.parIP, adresse)
		}
	}

	c, connu := l.parIP[ip]
	if !connu {
		c = &compteur{limite: rate.NewLimiter(l.debit, l.rafale)}
		l.parIP[ip] = c
	}
	c.vu = maintenant

	return c.limite.Allow()
}

func (l *limiteur) intergiciel(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.autoriser(adresseClient(r)) {
			ecrireErreur(w, http.StatusTooManyRequests, "Trop de requetes")
			return
		}
		suivant.ServeHTTP(w, r)
	})
}

// adresseClient rend l'adresse vue par le serveur. Les en-tetes de type
// X-Forwarded-For sont volontairement ignores : ils sont falsifiables tant
// qu'aucun proxy de confiance n'est declare.
func adresseClient(r *http.Request) string {
	hote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return hote
}

func journal(suivant http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		suivant.ServeHTTP(w, r)
	})
}

// enchainer applique les intergiciels dans l'ordre de lecture.
func enchainer(h http.Handler, intergiciels ...func(http.Handler) http.Handler) http.Handler {
	for i := len(intergiciels) - 1; i >= 0; i-- {
		h = intergiciels[i](h)
	}
	return h
}

func origines(brut string) []string {
	var liste []string
	for _, o := range strings.Split(brut, ",") {
		if o = strings.TrimSpace(o); o != "" {
			liste = append(liste, o)
		}
	}
	return liste
}
