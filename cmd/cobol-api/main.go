// Commande cobol-api : serveur HTTP du moteur d'amortissement COBOL.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kdev1966/cobol-api/internal/api"
)

func main() {
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

	serveur, err := api.NewServeur(cfg)
	if err != nil {
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

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("arret force", "erreur", err)
	}
	slog.Info("serveur arrete")
}
