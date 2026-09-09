// Commande cobol-api : serveur HTTP du moteur d'amortissement COBOL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kdev1966/cobol-api/internal/api"
	"github.com/kdev1966/cobol-api/internal/db"
)

// usage decrit les sous-commandes. Sans argument, le binaire sert l'API :
// c'est le mode attendu par le conteneur, et il reste le defaut.
func usage() {
	fmt.Fprint(os.Stderr, `cobol-api : moteur d'amortissement COBOL servi en Go

  cobol-api                sert l'API (mode par defaut)
  cobol-api migrer         applique les migrations puis sort
  cobol-api creer-agent    inscrit un compte d'agent de credit
                           -identifiant -nom -agence, mot de passe sur stdin

DATABASE_URL est requise par les trois.
`)
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrer":
			os.Exit(commandeMigrer())
		case "creer-agent":
			os.Exit(commandeCreerAgent(os.Args[2:]))
		case "-h", "--help", "aide":
			usage()
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "sous-commande inconnue : %s\n", os.Args[1])
			usage()
			os.Exit(2)
		}
	}

	// Journaux structures : en production au format JSON, pour etre ingerables
	// tels quels ; en texte ailleurs, pour rester lisibles au terminal.
	cfg := api.ConfigDepuisEnv()
	var gestionnaire slog.Handler
	if cfg.Production {
		gestionnaire = slog.NewJSONHandler(os.Stderr, nil)
	} else {
		gestionnaire = slog.NewTextHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(gestionnaire))

	// La base porte les bareme reglementaires : le service ne demarre pas
	// sans elle, plutot que de repondre a cote.
	ctxDemarrage, finDemarrage := context.WithTimeout(context.Background(), 30*time.Second)
	base, err := db.Ouvrir(ctxDemarrage, cfg.DatabaseURL)
	if err != nil {
		finDemarrage()
		slog.Error("erreur d'initialisation", "erreur", err)
		os.Exit(1)
	}
	if err := db.Migrer(ctxDemarrage, base); err != nil {
		finDemarrage()
		base.Close()
		slog.Error("migration de la base", "erreur", err)
		os.Exit(1)
	}
	finDemarrage()

	serveur, err := api.NewServeur(cfg, base)
	if err != nil {
		base.Close()
		slog.Error("erreur d'initialisation", "erreur", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           serveur.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	arret := make(chan os.Signal, 1)
	signal.Notify(arret, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("serveur demarre", "port", cfg.Port, "production", cfg.Production)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("erreur du serveur", "erreur", err)
			os.Exit(1)
		}
	}()

	sig := <-arret
	slog.Info("signal recu, arret en cours", "signal", sig.String())

	ctx, annuler := context.WithTimeout(context.Background(), 10*time.Second)
	defer annuler()

	// Les requetes en vol d'abord, la base ensuite : fermer le pool avant
	// qu'elles aient fini les ferait echouer sur la ligne d'arrivee.
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("arret force", "erreur", err)
	}
	base.Close()
	slog.Info("serveur arrete")
}
