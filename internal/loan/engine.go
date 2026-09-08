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
	"time"
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
	// Les frais tiennent dans un PIC 9(9)V99.
	FraisMax = 99999999999
)

// Demande est une demande d'echeancier deja validee.
type Demande struct {
	// CapitalCentimes evite tout flottant : les montants restent entiers.
	CapitalCentimes int64 `json:"-"`
	// TauxMillioniemes porte le taux nominal annuel, 3.45 % valant 3450000.
	TauxMillioniemes int64 `json:"-"`
	// CodeMethode et CodeAssiette sont les lettres attendues par le
	// programme COBOL.
	CodeMethode  byte `json:"-"`
	CodeAssiette byte `json:"-"`

	FraisDossierCentimes  int64 `json:"-"`
	FraisGarantieCentimes int64 `json:"-"`
	TauxAssuranceMillion  int64 `json:"-"`
	// TauxUsureDixMillieme porte le plafond reglementaire, 5.88 % valant
	// 58800. Zero signifie qu'aucune verification n'est demandee.
	TauxUsureDixMillieme int64 `json:"-"`

	Mois          int    `json:"mois"`
	Capital       string `json:"capital"`
	Taux          string `json:"taux"`
	Methode       string `json:"methode"`
	FraisDossier  string `json:"frais_dossier"`
	FraisGarantie string `json:"frais_garantie"`
	TauxAssurance string `json:"taux_assurance"`
	AssietteAssur string `json:"assiette_assurance"`
	// TauxUsure est absent de la reponse quand aucun plafond n'est demande.
	TauxUsure *string `json:"taux_usure,omitempty"`
}

// Format des enregistrements rendus par le programme COBOL. Les positions
// sont un contrat partage avec cobol/loan-amortization.cbl.
const (
	tagRecap     = 'R'
	tagEcheance  = 'E'
	longRecap    = 124
	longEcheance = 83
)

// Recapitulatif est la premiere ligne rendue par le programme COBOL.
//
// Les montants sont des json.Number : les chiffres produits par le COBOL
// traversent le service sous forme de texte, sans jamais passer par un
// flottant, sans quoi l'exactitude au centime que garantit le moteur serait
// perdue au moment de la relecture.
type Recapitulatif struct {
	Echeances int `json:"echeances"`
	// Sous la methode a annuite constante et sans assurance sur le capital
	// restant du, toutes les mensualites sauf la derniere valent
	// PremiereMensualite. Sous les autres combinaisons, elle varie.
	PremiereMensualite json.Number `json:"premiere_mensualite"`
	DerniereMensualite json.Number `json:"derniere_mensualite"`
	TotalInterets      json.Number `json:"total_interets"`
	TotalAssurance     json.Number `json:"total_assurance"`
	TotalFrais         json.Number `json:"total_frais"`
	// TotalVerse est la somme des mensualites, frais initiaux exclus.
	TotalVerse json.Number `json:"total_verse"`
	// CoutCredit agrege interets, assurance et frais.
	CoutCredit json.Number `json:"cout_credit"`
	// Taeg est le taux actuariel annuel qui egalise la valeur actuelle des
	// versements au montant reellement percu, capital diminue des frais.
	// Des lors que des frais ou une assurance entrent dans les flux, aucune
	// formule fermee ne le donne : il est resolu par dichotomie.
	Taeg json.Number `json:"taeg"`

	// Les trois champs suivants sont nuls quand aucun plafond n'a ete
	// fourni : le service rend alors le TAEG sans le juger.
	TauxUsure  *json.Number `json:"taux_usure"`
	Conforme   *bool        `json:"conforme"`
	MargeUsure *json.Number `json:"marge_usure"`
}

// Echeance est une ligne de l'echeancier.
type Echeance struct {
	N int `json:"n"`
	// Echeance est la part de credit : capital + interets.
	Echeance  json.Number `json:"echeance"`
	Interets  json.Number `json:"interets"`
	Capital   json.Number `json:"capital"`
	Assurance json.Number `json:"assurance"`
	// Mensualite est ce que l'emprunteur verse : echeance + assurance.
	Mensualite json.Number `json:"mensualite"`
	Solde      json.Number `json:"solde"`
}

// Echeancier est le resultat complet d'un calcul.
type Echeancier struct {
	Recapitulatif Recapitulatif `json:"recapitulatif"`
	Echeancier    []Echeance    `json:"echeancier"`
}

// ErrProgramme signale un echec du programme COBOL lui-meme.
var ErrProgramme = errors.New("le programme COBOL a echoue")

// ErrDelaiDepasse signale que le programme n'a pas rendu la main a temps.
var ErrDelaiDepasse = errors.New("le programme COBOL a depasse son delai")

// DelaiParDefaut borne un calcul. Un echeancier de 600 mois avec la
// resolution du TAEG prend une vingtaine de millisecondes ; cinq secondes
// laissent une marge considerable tout en garantissant qu'un processus bloque
// ne retient ni goroutine ni descripteur.
const DelaiParDefaut = 5 * time.Second

// delaiAvantArret laisse au processus le temps de sortir apres l'annulation,
// avant que Wait ne rende la main sans lui.
const delaiAvantArret = 2 * time.Second

// Moteur lance le binaire COBOL.
type Moteur struct {
	Chemin string
	Delai  time.Duration
}

// NewMoteur rend un moteur qui appellera le binaire situe a chemin. Un delai
// nul ou negatif retombe sur DelaiParDefaut.
func NewMoteur(chemin string, delai time.Duration) *Moteur {
	if delai <= 0 {
		delai = DelaiParDefaut
	}
	return &Moteur{Chemin: chemin, Delai: delai}
}

// ligneEntree rend les 63 caracteres attendus par le programme : capital
// 9(11)V99, taux 9(2)V9(6), duree 9(4), frais de dossier et de garantie
// 9(9)V99, taux d'assurance 9(2)V9(6), taux d'usure 9(2)V9(4), puis les
// lettres de la methode et de l'assiette d'assurance.
func ligneEntree(d Demande) string {
	return fmt.Sprintf("%013d%08d%04d%011d%011d%08d%06d%c%c\n",
		d.CapitalCentimes, d.TauxMillioniemes, d.Mois,
		d.FraisDossierCentimes, d.FraisGarantieCentimes,
		d.TauxAssuranceMillion, d.TauxUsureDixMillieme,
		d.CodeMethode, d.CodeAssiette)
}

// Calculer produit l'echeancier de la demande.
func (m *Moteur) Calculer(ctx context.Context, d Demande) (*Echeancier, error) {
	ctx, annuler := context.WithTimeout(ctx, m.Delai)
	defer annuler()

	cmd := exec.CommandContext(ctx, m.Chemin)
	// Sans WaitDelay, un processus qui ignore le signal d'arret retiendrait
	// Wait indefiniment, et avec lui la goroutine de la requete.
	cmd.WaitDelay = delaiAvantArret
	cmd.Stdin = strings.NewReader(ligneEntree(d))

	var sortie, erreurs bytes.Buffer
	cmd.Stdout = &sortie
	cmd.Stderr = &erreurs

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w apres %s", ErrDelaiDepasse, m.Delai)
		}
		detail := strings.TrimSpace(erreurs.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("%w : %s", ErrProgramme, detail)
	}

	return lireSortie(&sortie)
}

// nombre insere le point decimal dans une suite de chiffres. Le texte du
// COBOL devient le texte du JSON : aucun flottant n'intervient.
func nombre(chiffres string, decimales int) (json.Number, error) {
	if len(chiffres) <= decimales {
		return "", fmt.Errorf("valeur trop courte : %q", chiffres)
	}
	for _, r := range chiffres {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("valeur non numerique : %q", chiffres)
		}
	}

	coupe := len(chiffres) - decimales
	entiere := strings.TrimLeft(chiffres[:coupe], "0")
	if entiere == "" {
		entiere = "0"
	}
	return json.Number(entiere + "." + chiffres[coupe:]), nil
}

// montant traite les champs monetaires, tous a deux decimales.
func montant(chiffres string) (json.Number, error) {
	return nombre(chiffres, 2)
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

	champs := []struct {
		cible *json.Number
		debut int
		fin   int
	}{
		{&r.PremiereMensualite, 5, 18},
		{&r.DerniereMensualite, 18, 31},
		{&r.TotalInterets, 31, 46},
		{&r.TotalAssurance, 46, 61},
		{&r.TotalFrais, 61, 74},
		{&r.TotalVerse, 74, 89},
		{&r.CoutCredit, 89, 104},
	}
	for _, c := range champs {
		if *c.cible, err = montant(ligne[c.debut:c.fin]); err != nil {
			return fmt.Errorf("recapitulatif illisible : %w", err)
		}
	}

	// Le TAEG est un pourcentage a quatre decimales, pas un montant.
	if r.Taeg, err = nombre(ligne[104:110], 4); err != nil {
		return fmt.Errorf("recapitulatif illisible : %w", err)
	}

	// Le verdict d'usure : "-" quand aucun plafond n'a ete demande, auquel
	// cas les trois champs restent nuls.
	switch ligne[116] {
	case '-':
		return nil
	case 'O', 'N':
		conforme := ligne[116] == 'O'
		usure, err := nombre(ligne[110:116], 4)
		if err != nil {
			return fmt.Errorf("recapitulatif illisible : %w", err)
		}
		marge, err := nombreSigne(ligne[117:124], 4)
		if err != nil {
			return fmt.Errorf("recapitulatif illisible : %w", err)
		}
		r.TauxUsure, r.Conforme, r.MargeUsure = &usure, &conforme, &marge
		return nil
	default:
		return fmt.Errorf("recapitulatif illisible : conformite %q inattendue",
			ligne[116])
	}
}

// nombreSigne lit un champ COBOL a signe separe en tete : un caractere de
// signe, puis les chiffres.
func nombreSigne(champ string, decimales int) (json.Number, error) {
	if len(champ) < 2 {
		return "", fmt.Errorf("valeur signee trop courte : %q", champ)
	}
	valeur, err := nombre(champ[1:], decimales)
	if err != nil {
		return "", err
	}
	switch champ[0] {
	case '+':
		return valeur, nil
	case '-':
		// Un zero negatif n'existe pas dans la reponse.
		if strings.Trim(valeur.String(), "0.") == "" {
			return valeur, nil
		}
		return json.Number("-" + valeur.String()), nil
	default:
		return "", fmt.Errorf("signe %q inattendu", champ[0])
	}
}

func lireEcheance(ligne string, e *Echeance) error {
	var err error
	if e.N, err = entier(ligne[1:5]); err != nil {
		return fmt.Errorf("echeance illisible : %w", err)
	}

	champs := []struct {
		cible *json.Number
		debut int
		fin   int
	}{
		{&e.Echeance, 5, 18},
		{&e.Interets, 18, 31},
		{&e.Capital, 31, 44},
		{&e.Assurance, 44, 57},
		{&e.Mensualite, 57, 70},
		{&e.Solde, 70, 83},
	}
	for _, c := range champs {
		if *c.cible, err = montant(ligne[c.debut:c.fin]); err != nil {
			return fmt.Errorf("echeance illisible : %w", err)
		}
	}
	return nil
}
