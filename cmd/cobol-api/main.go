// Commande cobol-api : serveur HTTP du moteur d'amortissement COBOL.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kdev1966/cobol-api/internal/api"
)

func main() {
	cfg := api.ConfigDepuisEnv()

	serveur, err := api.NewServeur(cfg)
	if err != nil {
		log.Printf("Erreur d'initialisation : %v", err)
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
		log.Printf("Serveur demarre sur le port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Erreur du serveur : %v", err)
			os.Exit(1)
		}
	}()

	sig := <-arret
	log.Printf("%v recu, arret en cours...", sig)

	ctx, annuler := context.WithTimeout(context.Background(), 10*time.Second)
	defer annuler()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Arret force : %v", err)
	}
	log.Println("Serveur arrete")
}
