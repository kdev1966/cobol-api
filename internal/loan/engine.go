// Package loan expose le moteur d'amortissement COBOL.
//
// Le programme COBOL porte les regles metier et toute l'arithmetique. Ce
// paquet ne fait que formater la demande, lancer le binaire et relayer sa
// sortie : aucun montant n'est recalcule ici.
package loan

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Bornes acceptees. Le capital et le taux sont limites par les PIC du
// programme COBOL ; la duree est bornee plus bas que son PIC 9(4) pour
// eviter des reponses demesurees.
const (
	CapitalMin = 1 // en centimes
	CapitalMax = 9999999999999
	TauxMax    = 99999999 // 99.999999 %, en millioniemes
	MoisMin    = 1
	MoisMax    = 600
)

// Demande est une demande d'echeancier deja validee.
type Demande struct {
	// CapitalCentimes evite tout flottant : les montants restent entiers.
	CapitalCentimes int64 `json:"-"`
	// TauxMillioniemes porte le taux nominal annuel, 3.45 % valant 3450000.
	TauxMillioniemes int64 `json:"-"`
	// CodeMethode est la lettre attendue par le programme COBOL.
	CodeMethode byte `json:"-"`

	Mois    int    `json:"mois"`
	Capital string `json:"capital"`
	Taux    string `json:"taux"`
	Methode string `json:"methode"`
}

// Format des enregistrements rendus par le programme COBOL. Les positions
// sont un contrat partage avec cobol/loan-amortization.cbl.
const (
	tagRecap     = 'R'
	tagEcheance  = 'E'
	longRecap    = 61
	longEcheance = 57
)

// Recapitulatif est la premiere ligne rendue par le programme COBOL.
//
// Les montants sont des json.Number : les chiffres produits par le COBOL
// traversent le service sous forme de texte, sans jamais passer par un
// flottant, sans quoi l'exactitude au centime que garantit le moteur serait
// perdue au moment de la relecture.
type Recapitulatif struct {
	Echeances int `json:"echeances"`
	// Sous la methode a annuite constante, toutes les echeances sauf la
	// derniere valent PremiereEcheance. Sous les deux autres, l'echeance
	// varie a chaque periode.
	PremiereEcheance json.Number `json:"premiere_echeance"`
	DerniereEcheance json.Number `json:"derniere_echeance"`
	TotalInterets    json.Number `json:"total_interets"`
	TotalDu          json.Number `json:"total_du"`
}

// Echeance est une ligne de l'echeancier.
type Echeance struct {
	N        int         `json:"n"`
	Paiement json.Number `json:"paiement"`
	Interets json.Number `json:"interets"`
	Capital  json.Number `json:"capital"`
	Solde    json.Number `json:"solde"`
}

// Echeancier est le resultat complet d'un calcul.
type Echeancier struct {
	Recapitulatif Recapitulatif `json:"recapitulatif"`
	Echeancier    []Echeance    `json:"echeancier"`
}

// ErrProgramme signale un echec du programme COBOL lui-meme.
var ErrProgramme = errors.New("le programme COBOL a echoue")

// Moteur lance le binaire COBOL.
type Moteur struct {
	Chemin string
}

// NewMoteur rend un moteur qui appellera le binaire situe a chemin.
func NewMoteur(chemin string) *Moteur {
	return &Moteur{Chemin: chemin}
}

// ligneEntree rend les 26 caracteres attendus par le programme : capital
// 9(11)V99, taux 9(2)V9(6), duree 9(4), puis la lettre de la methode.
func ligneEntree(d Demande) string {
	return fmt.Sprintf("%013d%08d%04d%c\n",
		d.CapitalCentimes, d.TauxMillioniemes, d.Mois, d.CodeMethode)
}

// Calculer produit l'echeancier de la demande.
func (m *Moteur) Calculer(ctx context.Context, d Demande) (*Echeancier, error) {
	cmd := exec.CommandContext(ctx, m.Chemin)
	cmd.Stdin = strings.NewReader(ligneEntree(d))

	var sortie, erreurs bytes.Buffer
	cmd.Stdout = &sortie
	cmd.Stderr = &erreurs

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(erreurs.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%w : %s", ErrProgramme, detail)
	}

	return lireSortie(&sortie)
}

// montant insere le point decimal dans une suite de chiffres. Le texte du
// COBOL devient le texte du JSON : aucun flottant n'intervient.
func montant(chiffres string) (json.Number, error) {
	if len(chiffres) < 3 {
		return "", fmt.Errorf("montant trop court : %q", chiffres)
	}
	for _, r := range chiffres {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("montant non numerique : %q", chiffres)
		}
	}

	entiere := strings.TrimLeft(chiffres[:len(chiffres)-2], "0")
	if entiere == "" {
		entiere = "0"
	}
	return json.Number(entiere + "." + chiffres[len(chiffres)-2:]), nil
}

func entier(chiffres string) (int, error) {
	n, err := strconv.Atoi(chiffres)
	if err != nil {
		return 0, fmt.Errorf("entier illisible : %q", chiffres)
	}
	return n, nil
}

// lireSortie interprete les enregistrements a largeur fixe rendus par le
// COBOL : un recapitulatif puis une ligne par echeance. Les lignes non
// conformes sont ecartees plutot que de faire echouer le calcul, un
// avertissement de libcob pouvant se glisser sur la sortie standard.
func lireSortie(r *bytes.Buffer) (*Echeancier, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	res := &Echeancier{}
	recapLu := false

	for scanner.Scan() {
		ligne := strings.TrimRight(scanner.Text(), " \r")

		switch {
		case len(ligne) == longRecap && ligne[0] == tagRecap:
			if recapLu {
				return nil, fmt.Errorf("%w : recapitulatif en double", ErrProgramme)
			}
			if err := lireRecapitulatif(ligne, &res.Recapitulatif); err != nil {
				return nil, err
			}
			recapLu = true

		case len(ligne) == longEcheance && ligne[0] == tagEcheance:
			var e Echeance
			if err := lireEcheance(ligne, &e); err != nil {
				return nil, err
			}
			res.Echeancier = append(res.Echeancier, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lecture de la sortie : %w", err)
	}

	if !recapLu {
		return nil, fmt.Errorf("%w : aucune sortie exploitable", ErrProgramme)
	}
	if len(res.Echeancier) != res.Recapitulatif.Echeances {
		return nil, fmt.Errorf("%w : %d echeances annoncees, %d recues",
			ErrProgramme, res.Recapitulatif.Echeances, len(res.Echeancier))
	}

	return res, nil
}

func lireRecapitulatif(ligne string, r *Recapitulatif) error {
	var err error
	if r.Echeances, err = entier(ligne[1:5]); err != nil {
		return fmt.Errorf("recapitulatif illisible : %w", err)
	}
	if r.PremiereEcheance, err = montant(ligne[5:18]); err != nil {
		return fmt.Errorf("recapitulatif illisible : %w", err)
	}
	if r.DerniereEcheance, err = montant(ligne[18:31]); err != nil {
		return fmt.Errorf("recapitulatif illisible : %w", err)
	}
	if r.TotalInterets, err = montant(ligne[31:46]); err != nil {
		return fmt.Errorf("recapitulatif illisible : %w", err)
	}
	if r.TotalDu, err = montant(ligne[46:61]); err != nil {
		return fmt.Errorf("recapitulatif illisible : %w", err)
	}
	return nil
}

func lireEcheance(ligne string, e *Echeance) error {
	var err error
	if e.N, err = entier(ligne[1:5]); err != nil {
		return fmt.Errorf("echeance illisible : %w", err)
	}
	if e.Paiement, err = montant(ligne[5:18]); err != nil {
		return fmt.Errorf("echeance illisible : %w", err)
	}
	if e.Interets, err = montant(ligne[18:31]); err != nil {
		return fmt.Errorf("echeance illisible : %w", err)
	}
	if e.Capital, err = montant(ligne[31:44]); err != nil {
		return fmt.Errorf("echeance illisible : %w", err)
	}
	if e.Solde, err = montant(ligne[44:57]); err != nil {
		return fmt.Errorf("echeance illisible : %w", err)
	}
	return nil
}
