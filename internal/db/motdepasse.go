package db

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parametres argon2id. Ceux-ci suivent la deuxieme option recommandee par le
// RFC 9106 : 64 Mio de memoire, trois passes, un degre de parallelisme de
// quatre. Sur un poste ordinaire, une verification coute une cinquantaine de
// millisecondes, ce qui rend une attaque par dictionnaire couteuse sans peser
// sur une connexion legitime.
//
// Les valeurs sont ecrites dans l'empreinte : les durcir plus tard
// n'invalidera pas les empreintes deja calculees.
const (
	argonMemoire     = 64 * 1024
	argonPasses      = 3
	argonParallelism = 4
	argonSel         = 16
	argonEmpreinte   = 32
)

// ErrEmpreinteInvalide signale une empreinte que ce code ne sait pas relire.
var ErrEmpreinteInvalide = errors.New("empreinte de mot de passe illisible")

// HacherMotDePasse rend une empreinte argon2id au format PHC, qui porte ses
// propres parametres.
func HacherMotDePasse(motDePasse string) (string, error) {
	sel := make([]byte, argonSel)
	if _, err := rand.Read(sel); err != nil {
		return "", fmt.Errorf("tirage du sel : %w", err)
	}

	cle := argon2.IDKey([]byte(motDePasse), sel,
		argonPasses, argonMemoire, argonParallelism, argonEmpreinte)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoire, argonPasses, argonParallelism,
		base64.RawStdEncoding.EncodeToString(sel),
		base64.RawStdEncoding.EncodeToString(cle)), nil
}

// VerifierMotDePasse compare un mot de passe a une empreinte. La comparaison
// est a temps constant : une comparaison ordinaire divulguerait, par sa duree,
// combien d'octets de tete concordent.
func VerifierMotDePasse(motDePasse, empreinte string) (bool, error) {
	parts := strings.Split(empreinte, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrEmpreinteInvalide
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrEmpreinteInvalide
	}
	if version != argon2.Version {
		return false, fmt.Errorf("%w : version %d", ErrEmpreinteInvalide, version)
	}

	var memoire uint32
	var passes uint32
	var parallelisme uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d",
		&memoire, &passes, &parallelisme); err != nil {
		return false, ErrEmpreinteInvalide
	}

	sel, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrEmpreinteInvalide
	}
	attendu, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrEmpreinteInvalide
	}

	calcule := argon2.IDKey([]byte(motDePasse), sel,
		passes, memoire, parallelisme, uint32(len(attendu)))

	return subtle.ConstantTimeCompare(calcule, attendu) == 1, nil
}
