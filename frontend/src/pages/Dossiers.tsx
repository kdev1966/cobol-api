import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, ErreurAPI, millimes, date, type Dossier, type Bareme } from '../api'
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
  const [filtre, poserFiltre] = useState('')
  const [erreur, poserErreur] = useState<string | null>(null)
  const [chargement, poserChargement] = useState(true)
  const [formulaireOuvert, poserFormulaireOuvert] = useState(false)

  const charger = useCallback(async (statut: string) => {
    poserChargement(true)
    poserErreur(null)
    try {
      const rep = await api.listerDossiers(statut || undefined)
      poserDossiers(rep.dossiers)
    } catch (err) {
      poserErreur(err instanceof ErreurAPI ? err.message : 'Service injoignable')
    } finally {
      poserChargement(false)
    }
  }, [])

  useEffect(() => {
    void charger(filtre)
  }, [charger, filtre])

  return (
    <section>
      <div className="entete-section">
        <h2>Dossiers de l'agence</h2>
        <button onClick={() => poserFormulaireOuvert((o) => !o)}>
          {formulaireOuvert ? 'Fermer' : 'Nouveau dossier'}
        </button>
      </div>

      {formulaireOuvert && (
        <NouveauDossier
          apresCreation={() => {
            poserFormulaireOuvert(false)
            void charger(filtre)
          }}
        />
      )}

      <div className="filtres">
        {statuts.map((s) => (
          <button
            key={s.valeur}
            className={filtre === s.valeur ? 'actif' : ''}
            onClick={() => poserFiltre(s.valeur)}
          >
            {s.libelle}
          </button>
        ))}
      </div>

      {erreur && <p className="erreur">{erreur}</p>}
      {chargement && <p className="discret">Chargement…</p>}

      {!chargement && dossiers.length === 0 && (
        <p className="discret">Aucun dossier.</p>
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
