package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/kdev1966/cobol-api/internal/db"
)

// longueurMinimale du mot de passe. Le NIST SP 800-63B recommande de miser sur
// la longueur plutot que sur des regles de composition, qui poussent surtout a
// des substitutions previsibles.
const longueurMinimale = 12

// ouvrirBase etablit la connexion pour une commande d'administration.
func ouvrirBase(ctx context.Context) (*db.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, errors.New("DATABASE_URL est vide")
	}
	return db.Ouvrir(ctx, url)
}

// commandeMigrer applique les migrations sans demarrer le serveur. Utile au
// deploiement, et aux tests qui ont besoin d'un schema a jour.
func commandeMigrer() int {
	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()

	base, err := ouvrirBase(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ouverture de la base :", err)
		return 1
	}
	defer base.Close()

	if err := db.Migrer(ctx, base); err != nil {
		fmt.Fprintln(os.Stderr, "migration :", err)
		return 1
	}
	fmt.Println("migrations appliquees")
	return 0
}

// commandeCreerAgent inscrit un compte d'agent de credit. Il n'y a pas
// d'auto-inscription : ouvrir une route de creation publique sur un service
// qui produit des offres de credit n'aurait pas de sens.
func commandeCreerAgent(args []string) int {
	jeu := flag.NewFlagSet("creer-agent", flag.ContinueOnError)
	identifiant := jeu.String("identifiant", "", "identifiant de connexion")
	nom := jeu.String("nom", "", "nom de l'agent")
	agence := jeu.String("agence", "", "agence de rattachement")
	if err := jeu.Parse(args); err != nil {
		return 2
	}

	manquants := []string{}
	for nomChamp, valeur := range map[string]string{
		"identifiant": *identifiant, "nom": *nom, "agence": *agence,
	} {
		if strings.TrimSpace(valeur) == "" {
			manquants = append(manquants, nomChamp)
		}
	}
	if len(manquants) > 0 {
		fmt.Fprintf(os.Stderr, "arguments manquants : %s\n",
			strings.Join(manquants, ", "))
		jeu.Usage()
		return 2
	}

	motDePasse, err := lireMotDePasse()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len([]rune(motDePasse)) < longueurMinimale {
		fmt.Fprintf(os.Stderr,
			"le mot de passe doit faire au moins %d caracteres\n",
			longueurMinimale)
		return 1
	}

	ctx, annuler := context.WithTimeout(context.Background(), 60*time.Second)
	defer annuler()

	base, err := ouvrirBase(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ouverture de la base :", err)
		return 1
	}
	defer base.Close()

	agent, err := db.NewAgents(base).Creer(ctx,
		*identifiant, motDePasse, *nom, *agence)
	if err != nil {
		if errors.Is(err, db.ErrIdentifiantPris) {
			fmt.Fprintf(os.Stderr, "l'identifiant %q est deja attribue\n",
				*identifiant)
			return 1
		}
		fmt.Fprintln(os.Stderr, "creation :", err)
		return 1
	}

	fmt.Printf("agent %d cree : %s (%s, %s)\n",
		agent.ID, agent.Identifiant, agent.Nom, agent.Agence)
	return 0
}

// lireMotDePasse prend le mot de passe sur l'entree standard. Au terminal, la
// frappe n'est pas affichee ; en script ou en conteneur, il se passe par un
// tube, ce qui evite de le laisser dans l'historique du shell.
func lireMotDePasse() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "mot de passe : ")
		brut, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("lecture du mot de passe : %w", err)
		}
		return string(brut), nil
	}

	lecteur := bufio.NewReader(os.Stdin)
	ligne, err := lecteur.ReadString('\n')
	if err != nil && ligne == "" {
		return "", errors.New("aucun mot de passe sur l'entree standard")
	}
	return strings.TrimRight(ligne, "\r\n"), nil
}
