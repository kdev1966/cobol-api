package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrDossierAbsent signale un dossier inexistant, ou hors de l'agence de
// l'agent qui le demande. Les confondre est deliberé : dire « ce dossier
// existe mais n'est pas le votre » renseignerait sur l'activite des autres
// agences.
var ErrDossierAbsent = errors.New("dossier introuvable")

// ErrReferencePrise signale une reference deja employee dans l'agence.
var ErrReferencePrise = errors.New("reference deja employee dans cette agence")

// ErrTransitionRefusee signale un changement de statut que le cycle de vie
// n'autorise pas.
var ErrTransitionRefusee = errors.New("changement de statut refuse")

// ErrDossierFige signale une modification sur un dossier qui n'est plus en
// brouillon : une fois transmis a l'instruction, les parametres du pret ne
// bougent plus, sans quoi la decision porterait sur autre chose que ce qui a
// ete instruit.
var ErrDossierFige = errors.New("le dossier n'est plus modifiable")

// Cycle de vie d'un dossier. Un dossier nait en brouillon, part en
// instruction, puis se decide. Les trois etats terminaux ne mènent nulle part.
var transitions = map[string][]string{
	"brouillon":      {"en_instruction", "annule"},
	"en_instruction": {"accorde", "refuse", "annule"},
	"accorde":        {},
	"refuse":         {},
	"annule":         {},
}

// StatutsAcceptes liste les statuts, pour la documentation et les messages.
func StatutsAcceptes() []string {
	return []string{"brouillon", "en_instruction", "accorde", "refuse", "annule"}
}

// TransitionPermise dit si un dossier peut passer d'un statut a un autre.
func TransitionPermise(de, vers string) bool {
	for _, permis := range transitions[de] {
		if permis == vers {
			return true
		}
	}
	return false
}

// TransitionsDepuis liste les suites possibles d'un statut.
func TransitionsDepuis(statut string) []string {
	suites := transitions[statut]
	if suites == nil {
		return []string{}
	}
	return suites
}

// Dossier est un dossier de pret. Aucun champ ne porte de donnee a caractere
// personnel : la reference renvoie au systeme de la banque, ou l'identite de
// l'emprunteur reste.
type Dossier struct {
	ID        int64  `json:"id"`
	Reference string `json:"reference"`
	AgentID   int64  `json:"agent_id"`
	Agence    string `json:"agence"`
	Statut    string `json:"statut"`

	Capital     string  `json:"capital"`
	Taux        string  `json:"taux"`
	Mois        int     `json:"mois"`
	Methode     string  `json:"methode"`
	Differe     int     `json:"differe"`
	TypeDiffere string  `json:"type_differe"`
	Categorie   *string `json:"categorie"`

	SimulationID *int64 `json:"simulation_id"`

	CreeLe string `json:"cree_le"`
	MajLe  string `json:"maj_le"`

	// TransitionsPossibles evite au frontend de reimplementer le cycle de vie.
	TransitionsPossibles []string `json:"transitions_possibles"`
}

// Evenement est un changement de statut.
type Evenement struct {
	ID          int64   `json:"id"`
	AgentID     int64   `json:"agent_id"`
	AgentNom    string  `json:"agent_nom"`
	StatutAvant *string `json:"statut_avant"`
	StatutApres string  `json:"statut_apres"`
	Note        *string `json:"note"`
	CreeLe      string  `json:"cree_le"`
}

// Dossiers tient les dossiers de pret.
type Dossiers struct {
	pool *Pool
}

func NewDossiers(pool *Pool) *Dossiers {
	return &Dossiers{pool: pool}
}

// colonnes liste les champs lus, dans l'ordre ou scanDossier les attend.
const colonnesDossier = `id, reference, agent_id, agence, statut,
	capital, taux, mois, methode, differe, type_differe, categorie,
	simulation_id, cree_le, maj_le`

func scanDossier(rang pgx.Row) (*Dossier, error) {
	var d Dossier
	var creeLe, majLe time.Time
	err := rang.Scan(&d.ID, &d.Reference, &d.AgentID, &d.Agence, &d.Statut,
		&d.Capital, &d.Taux, &d.Mois, &d.Methode, &d.Differe, &d.TypeDiffere,
		&d.Categorie, &d.SimulationID, &creeLe, &majLe)
	if err != nil {
		return nil, err
	}
	d.CreeLe = creeLe.UTC().Format(time.RFC3339)
	d.MajLe = majLe.UTC().Format(time.RFC3339)
	d.TransitionsPossibles = TransitionsDepuis(d.Statut)
	return &d, nil
}

// Creer inscrit un dossier en brouillon et son premier evenement.
func (dd *Dossiers) Creer(ctx context.Context, agent *Agent, d Dossier) (*Dossier, error) {
	tx, err := dd.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ouverture de transaction : %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cree, err := scanDossier(tx.QueryRow(ctx, `
		INSERT INTO dossiers (reference, agent_id, agence, capital, taux,
			mois, methode, differe, type_differe, categorie)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+colonnesDossier,
		strings.TrimSpace(d.Reference), agent.ID, agent.Agence,
		d.Capital, d.Taux, d.Mois, d.Methode, d.Differe, d.TypeDiffere,
		d.Categorie))
	if err != nil {
		if strings.Contains(err.Error(), "dossiers_reference") {
			return nil, ErrReferencePrise
		}
		return nil, fmt.Errorf("creation du dossier : %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO dossiers_evenements (dossier_id, agent_id, statut_apres)
		VALUES ($1, $2, 'brouillon')`, cree.ID, agent.ID); err != nil {
		return nil, fmt.Errorf("inscription de l'evenement : %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("validation de la transaction : %w", err)
	}
	return cree, nil
}

// Lire rend un dossier de l'agence de l'agent.
func (dd *Dossiers) Lire(ctx context.Context, agent *Agent, id int64) (*Dossier, error) {
	d, err := scanDossier(dd.pool.QueryRow(ctx,
		`SELECT `+colonnesDossier+` FROM dossiers
		 WHERE id = $1 AND agence = $2`, id, agent.Agence))
	if err != nil {
		if estAbsent(err) {
			return nil, ErrDossierAbsent
		}
		return nil, fmt.Errorf("lecture du dossier : %w", err)
	}
	return d, nil
}

// Lister rend les dossiers de l'agence, du plus recent au plus ancien. Un
// statut vide ne filtre pas.
func (dd *Dossiers) Lister(ctx context.Context, agent *Agent,
	statut string, limite int) ([]Dossier, error) {

	if limite < 1 || limite > 200 {
		limite = 50
	}

	rangs, err := dd.pool.Query(ctx,
		`SELECT `+colonnesDossier+` FROM dossiers
		 WHERE agence = $1 AND ($2 = '' OR statut = $2)
		 ORDER BY cree_le DESC LIMIT $3`,
		agent.Agence, statut, limite)
	if err != nil {
		return nil, fmt.Errorf("liste des dossiers : %w", err)
	}
	defer rangs.Close()

	liste := []Dossier{}
	for rangs.Next() {
		d, err := scanDossier(rangs)
		if err != nil {
			return nil, fmt.Errorf("lecture d'un dossier : %w", err)
		}
		liste = append(liste, *d)
	}
	return liste, rangs.Err()
}

// Modifier change les parametres du pret. Seul un brouillon est modifiable :
// une fois transmis a l'instruction, la decision doit porter sur ce qui a ete
// instruit.
func (dd *Dossiers) Modifier(ctx context.Context, agent *Agent,
	id int64, d Dossier) (*Dossier, error) {

	actuel, err := dd.Lire(ctx, agent, id)
	if err != nil {
		return nil, err
	}
	if actuel.Statut != "brouillon" {
		return nil, ErrDossierFige
	}

	modifie, err := scanDossier(dd.pool.QueryRow(ctx, `
		UPDATE dossiers SET reference = $3, capital = $4, taux = $5,
			mois = $6, methode = $7, differe = $8, type_differe = $9,
			categorie = $10, maj_le = now()
		WHERE id = $1 AND agence = $2
		RETURNING `+colonnesDossier,
		id, agent.Agence, strings.TrimSpace(d.Reference), d.Capital, d.Taux,
		d.Mois, d.Methode, d.Differe, d.TypeDiffere, d.Categorie))
	if err != nil {
		if strings.Contains(err.Error(), "dossiers_reference") {
			return nil, ErrReferencePrise
		}
		return nil, fmt.Errorf("modification du dossier : %w", err)
	}
	return modifie, nil
}

// ChangerLeStatut fait passer un dossier d'un statut a un autre et inscrit
// l'evenement. La transition est verifiee dans la transaction : deux agents
// qui decident en meme temps ne doivent pas pouvoir accorder puis refuser.
func (dd *Dossiers) ChangerLeStatut(ctx context.Context, agent *Agent,
	id int64, vers, note string) (*Dossier, error) {

	tx, err := dd.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ouverture de transaction : %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var avant string
	err = tx.QueryRow(ctx,
		`SELECT statut FROM dossiers WHERE id = $1 AND agence = $2 FOR UPDATE`,
		id, agent.Agence).Scan(&avant)
	if err != nil {
		if estAbsent(err) {
			return nil, ErrDossierAbsent
		}
		return nil, fmt.Errorf("lecture du statut : %w", err)
	}

	if !TransitionPermise(avant, vers) {
		return nil, fmt.Errorf("%w : de %s vers %s", ErrTransitionRefusee, avant, vers)
	}

	modifie, err := scanDossier(tx.QueryRow(ctx,
		`UPDATE dossiers SET statut = $3, maj_le = now()
		 WHERE id = $1 AND agence = $2 RETURNING `+colonnesDossier,
		id, agent.Agence, vers))
	if err != nil {
		return nil, fmt.Errorf("changement de statut : %w", err)
	}

	var noteOuNil any
	if strings.TrimSpace(note) != "" {
		noteOuNil = note
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dossiers_evenements (dossier_id, agent_id, statut_avant,
			statut_apres, note)
		VALUES ($1, $2, $3, $4, $5)`,
		id, agent.ID, avant, vers, noteOuNil); err != nil {
		return nil, fmt.Errorf("inscription de l'evenement : %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("validation de la transaction : %w", err)
	}
	return modifie, nil
}

// RattacherLaSimulation associe la derniere simulation calculee au dossier.
func (dd *Dossiers) RattacherLaSimulation(ctx context.Context,
	agent *Agent, id, simulation int64) error {

	etiquette, err := dd.pool.Exec(ctx,
		`UPDATE dossiers SET simulation_id = $3, maj_le = now()
		 WHERE id = $1 AND agence = $2`, id, agent.Agence, simulation)
	if err != nil {
		return fmt.Errorf("rattachement de la simulation : %w", err)
	}
	if etiquette.RowsAffected() == 0 {
		return ErrDossierAbsent
	}
	return nil
}

// Historique rend les changements de statut d'un dossier, du plus ancien au
// plus recent.
func (dd *Dossiers) Historique(ctx context.Context, agent *Agent,
	id int64) ([]Evenement, error) {

	if _, err := dd.Lire(ctx, agent, id); err != nil {
		return nil, err
	}

	rangs, err := dd.pool.Query(ctx, `
		SELECT e.id, e.agent_id, a.nom, e.statut_avant, e.statut_apres,
			e.note, e.cree_le
		FROM dossiers_evenements e JOIN agents a ON a.id = e.agent_id
		WHERE e.dossier_id = $1 ORDER BY e.cree_le, e.id`, id)
	if err != nil {
		return nil, fmt.Errorf("historique du dossier : %w", err)
	}
	defer rangs.Close()

	liste := []Evenement{}
	for rangs.Next() {
		var e Evenement
		var creeLe time.Time
		if err := rangs.Scan(&e.ID, &e.AgentID, &e.AgentNom, &e.StatutAvant,
			&e.StatutApres, &e.Note, &creeLe); err != nil {
			return nil, fmt.Errorf("lecture d'un evenement : %w", err)
		}
		e.CreeLe = creeLe.UTC().Format(time.RFC3339)
		liste = append(liste, e)
	}
	return liste, rangs.Err()
}
