import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import {
  api,
  ErreurAPI,
  millimes,
  entier,
  date,
  type Dossier,
  type Bareme,
  type Statut,
} from '../api'
import Etiquette from '../composants/Statut'
import { libelleCategorie } from '../composants/libelles'

const statuts = [
  { valeur: '', libelle: 'Tous' },
  { valeur: 'brouillon', libelle: 'Brouillons' },
  { valeur: 'en_instruction', libelle: 'En instruction' },
  { valeur: 'accorde', libelle: 'Accordés' },
  { valeur: 'refuse', libelle: 'Refusés' },
  { valeur: 'annule', libelle: 'Annulés' },
]

export default function Dossiers() {
  const [dossiers, poserDossiers] = useState<Dossier[]>([])
  const [total, poserTotal] = useState(0)
  const [repartition, poserRepartition] = useState<Record<string, number>>({})
  const [curseur, poserCurseur] = useState<string | undefined>()
  const [filtre, poserFiltre] = useState('')
  const [saisie, poserSaisie] = useState('')
  const [recherche, poserRecherche] = useState('')
  const [erreur, poserErreur] = useState<string | null>(null)
  const [chargement, poserChargement] = useState(true)
  const [suiteEnCours, poserSuiteEnCours] = useState(false)
  const [formulaireOuvert, poserFormulaireOuvert] = useState(false)

  // La premiere page remplace la liste ; les suivantes s'y ajoutent. C'est ce
  // que le curseur permet : il n'y a pas de numero de page ou revenir.
  const charger = useCallback(
    async (statut: string, cherche: string) => {
      poserChargement(true)
      poserErreur(null)
      try {
        const rep = await api.listerDossiers({
          statut: statut || undefined,
          recherche: cherche || undefined,
        })
        poserDossiers(rep.dossiers)
        poserTotal(rep.total ?? 0)
        poserRepartition(rep.repartition ?? {})
        poserCurseur(rep.curseur_suivant)
      } catch (err) {
        poserErreur(
          err instanceof ErreurAPI ? err.message : 'Service injoignable',
        )
      } finally {
        poserChargement(false)
      }
    },
    [],
  )

  const chargerLaSuite = useCallback(async () => {
    if (!curseur || suiteEnCours) return
    poserSuiteEnCours(true)
    try {
      const rep = await api.listerDossiers({
        statut: filtre || undefined,
        recherche: recherche || undefined,
        curseur,
      })
      poserDossiers((d) => [...d, ...rep.dossiers])
      poserCurseur(rep.curseur_suivant)
    } catch (err) {
      poserErreur(err instanceof ErreurAPI ? err.message : 'Service injoignable')
    } finally {
      poserSuiteEnCours(false)
    }
  }, [curseur, suiteEnCours, filtre, recherche])

  useEffect(() => {
    void charger(filtre, recherche)
  }, [charger, filtre, recherche])

  // La recherche part apres une pause de frappe : interroger a chaque touche
  // enverrait une requete par caractere sur une table d'un million de lignes.
  useEffect(() => {
    const minuteur = setTimeout(() => poserRecherche(saisie.trim()), 300)
    return () => clearTimeout(minuteur)
  }, [saisie])

  return (
    <section>
      <div className="entete-section">
        <h2>
          Dossiers de l'agence{' '}
          {!chargement && (
            <span className="compte">{entier(total)}</span>
          )}
        </h2>
        <button onClick={() => poserFormulaireOuvert((o) => !o)}>
          {formulaireOuvert ? 'Fermer' : 'Nouveau dossier'}
        </button>
      </div>

      {formulaireOuvert && (
        <NouveauDossier
          apresCreation={() => {
            poserFormulaireOuvert(false)
            void charger(filtre, recherche)
          }}
        />
      )}

      <div className="barre-recherche">
        <input
          value={saisie}
          onChange={(e) => poserSaisie(e.target.value)}
          placeholder="Rechercher une référence (début suffit)"
          aria-label="Rechercher une référence"
        />
        {saisie && (
          <button onClick={() => poserSaisie('')} title="Effacer">
            Effacer
          </button>
        )}
      </div>

      <div className="filtres">
        {statuts.map((s) => {
          // Le compte total figure en face de « Tous » ; les autres portent
          // celui de leur statut, pris dans la repartition.
          const n = s.valeur
            ? repartition[s.valeur as Statut]
            : Object.values(repartition).reduce((a, b) => a + b, 0)
          return (
            <button
              key={s.valeur}
              className={filtre === s.valeur ? 'actif' : ''}
              onClick={() => poserFiltre(s.valeur)}
            >
              {s.libelle}
              {n !== undefined && <span className="badge">{entier(n)}</span>}
            </button>
          )
        })}
      </div>

      {erreur && <p className="erreur">{erreur}</p>}
      {chargement && <p className="discret">Chargement…</p>}

      {!chargement && dossiers.length === 0 && (
        <p className="discret">
          {recherche
            ? `Aucune référence ne commence par « ${recherche} ».`
            : 'Aucun dossier.'}
        </p>
      )}

      {/* La liste est bornee : dire lesquels sont affiches evite de laisser
          croire que l'agence n'en compte que cinquante. Sous recherche, le
          total porte sur l'agence et non sur le resultat : ne pas l'opposer
          au nombre affiche eviterait de le laisser croire. */}
      {!chargement && dossiers.length > 0 && !recherche && (
        <p className="discret petit">
          {entier(dossiers.length)} dossiers affichés sur {entier(total)}, du
          plus récent au plus ancien.
        </p>
      )}

      {dossiers.length > 0 && (
        <table>
          <thead>
            <tr>
              <th>Référence</th>
              <th>Statut</th>
              <th className="nombre">Capital</th>
              <th className="nombre">Taux</th>
              <th className="nombre">Durée</th>
              <th>Créé le</th>
            </tr>
          </thead>
          <tbody>
            {dossiers.map((d) => (
              <tr key={d.id}>
                <td>
                  <Link to={`/dossiers/${d.id}`}>{d.reference}</Link>
                </td>
                <td>
                  <Etiquette statut={d.statut} />
                </td>
                <td className="nombre">{millimes(Number(d.capital))}</td>
                <td className="nombre">{Number(d.taux).toFixed(2)} %</td>
                <td className="nombre">{d.mois} mois</td>
                <td className="discret">{date(d.cree_le)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {curseur && (
        <div className="suite">
          <button onClick={() => void chargerLaSuite()} disabled={suiteEnCours}>
            {suiteEnCours ? 'Chargement…' : 'Afficher les suivants'}
          </button>
        </div>
      )}
    </section>
  )
}

function NouveauDossier({ apresCreation }: { apresCreation: () => void }) {
  const [reference, poserReference] = useState('')
  const [capital, poserCapital] = useState('')
  const [taux, poserTaux] = useState('')
  const [mois, poserMois] = useState('240')
  const [methode, poserMethode] = useState('annuite_constante')
  const [categorie, poserCategorie] = useState('')
  const [baremes, poserBaremes] = useState<Bareme[]>([])
  const [erreur, poserErreur] = useState<string | null>(null)
  const [enCours, poserEnCours] = useState(false)

  useEffect(() => {
    api
      .baremes()
      .then((r) => poserBaremes(r.baremes))
      .catch(() => poserBaremes([]))
  }, [])

  async function soumettre(e: FormEvent) {
    e.preventDefault()
    poserErreur(null)
    poserEnCours(true)
    try {
      await api.creerDossier({
        reference,
        capital,
        taux,
        mois: Number(mois),
        methode,
        categorie: categorie || null,
      })
      apresCreation()
    } catch (err) {
      poserErreur(
        err instanceof ErreurAPI
          ? err.champ
            ? `${err.champ} : ${err.message}`
            : err.message
          : 'Service injoignable',
      )
    } finally {
      poserEnCours(false)
    }
  }

  return (
    <form onSubmit={soumettre} className="carte formulaire">
      <p className="discret petit">
        Le dossier ne porte aucune donnée personnelle. La référence est celle
        que la banque emploie déjà dans son propre système.
      </p>
      <div className="grille">
        <label>
          Référence
          <input
            value={reference}
            onChange={(e) => poserReference(e.target.value)}
            placeholder="PRT-2026-0001"
            maxLength={64}
            required
          />
        </label>
        <label>
          Capital (DT)
          <input
            value={capital}
            onChange={(e) => poserCapital(e.target.value)}
            placeholder="250000.000"
            required
          />
        </label>
        <label>
          Taux nominal annuel (%)
          <input
            value={taux}
            onChange={(e) => poserTaux(e.target.value)}
            placeholder="8.5"
            required
          />
        </label>
        <label>
          Durée (mois)
          <input
            value={mois}
            onChange={(e) => poserMois(e.target.value)}
            required
          />
        </label>
        <label>
          Méthode
          <select value={methode} onChange={(e) => poserMethode(e.target.value)}>
            <option value="annuite_constante">Annuité constante</option>
            <option value="capital_constant">Capital constant</option>
            <option value="in_fine">In fine</option>
          </select>
        </label>
        <label>
          Catégorie de concours
          <select
            value={categorie}
            onChange={(e) => poserCategorie(e.target.value)}
          >
            <option value="">Aucune (pas de verdict d'usure)</option>
            {baremes.map((b) => (
              <option key={b.categorie} value={b.categorie}>
                {libelleCategorie(b.categorie)} — TEM{' '}
                {Number(b.taux).toFixed(2)} %
              </option>
            ))}
          </select>
        </label>
      </div>

      {erreur && <p className="erreur">{erreur}</p>}

      <button type="submit" disabled={enCours}>
        {enCours ? 'Création…' : 'Créer le dossier'}
      </button>
    </form>
  )
}
