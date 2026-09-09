import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  api,
  ErreurAPI,
  millimes,
  pourcent,
  date,
  type Dossier as TDossier,
  type Evenement,
  type ReponseEcheancier,
  type Statut,
} from '../api'
import Etiquette, { libelleStatut } from '../composants/Statut'
import {
  libelleMethode,
  libelleCategorie,
  libelleDiffere,
} from '../composants/libelles'

export default function Dossier() {
  const { id } = useParams()
  const numero = Number(id)

  const [dossier, poserDossier] = useState<TDossier | null>(null)
  const [historique, poserHistorique] = useState<Evenement[]>([])
  const [calcul, poserCalcul] = useState<ReponseEcheancier | null>(null)
  const [erreur, poserErreur] = useState<string | null>(null)
  const [note, poserNote] = useState('')
  const [enCours, poserEnCours] = useState(false)

  const charger = useCallback(async () => {
    poserErreur(null)
    try {
      const rep = await api.lireDossier(numero)
      poserDossier(rep.dossier)
      poserHistorique(rep.historique)
    } catch (err) {
      poserErreur(err instanceof ErreurAPI ? err.message : 'Service injoignable')
    }
  }, [numero])

  useEffect(() => {
    void charger()
  }, [charger])

  async function calculer() {
    poserErreur(null)
    poserEnCours(true)
    try {
      poserCalcul(await api.echeancierDossier(numero))
      await charger()
    } catch (err) {
      poserErreur(err instanceof ErreurAPI ? err.message : 'Service injoignable')
    } finally {
      poserEnCours(false)
    }
  }

  async function changerStatut(statut: Statut) {
    poserErreur(null)
    poserEnCours(true)
    try {
      await api.changerLeStatut(numero, statut, note)
      poserNote('')
      await charger()
    } catch (err) {
      poserErreur(err instanceof ErreurAPI ? err.message : 'Service injoignable')
    } finally {
      poserEnCours(false)
    }
  }

  if (erreur && !dossier) return <p className="erreur">{erreur}</p>
  if (!dossier) return <p className="discret">Chargement…</p>

  return (
    <section>
      <p>
        <Link to="/dossiers">← Tous les dossiers</Link>
      </p>

      <div className="entete-section">
        <h2>
          {dossier.reference} <Etiquette statut={dossier.statut} />
        </h2>
        <button onClick={calculer} disabled={enCours}>
          Calculer l'échéancier
        </button>
      </div>

      {erreur && <p className="erreur">{erreur}</p>}

      <div className="carte">
        <dl className="fiche">
          <div><dt>Capital</dt><dd>{millimes(Number(dossier.capital))} DT</dd></div>
          <div><dt>Taux nominal</dt><dd>{pourcent(Number(dossier.taux))}</dd></div>
          <div><dt>Durée</dt><dd>{dossier.mois} mois</dd></div>
          <div><dt>Méthode</dt><dd>{libelleMethode(dossier.methode)}</dd></div>
          <div><dt>Différé</dt><dd>{libelleDiffere(dossier.differe, dossier.type_differe)}</dd></div>
          <div><dt>Catégorie</dt><dd>{libelleCategorie(dossier.categorie)}</dd></div>
          <div><dt>Agence</dt><dd>{dossier.agence}</dd></div>
          <div><dt>Simulation</dt><dd>{dossier.simulation_id ?? '—'}</dd></div>
        </dl>
      </div>

      {dossier.transitions_possibles.length > 0 ? (
        <div className="carte">
          <h3>Suite à donner</h3>
          <label>
            Note (facultative)
            <input
              value={note}
              onChange={(e) => poserNote(e.target.value)}
              maxLength={1000}
              placeholder="Motif de la décision"
            />
          </label>
          <div className="actions">
            {dossier.transitions_possibles.map((s) => (
              <button key={s} onClick={() => changerStatut(s)} disabled={enCours}>
                {libelleStatut(s)}
              </button>
            ))}
          </div>
          {dossier.statut === 'brouillon' && (
            <p className="discret petit">
              Une fois transmis à l'instruction, les paramètres du prêt ne
              bougent plus : la décision doit porter sur ce qui a été instruit.
            </p>
          )}
        </div>
      ) : (
        <p className="discret">
          Ce statut est terminal : une décision ne se revient pas.
        </p>
      )}

      <div className="carte">
        <h3>Historique</h3>
        <ol className="historique">
          {historique.map((e) => (
            <li key={e.id}>
              <span className="discret">{date(e.cree_le)}</span>{' '}
              {e.statut_avant ? `${e.statut_avant} → ` : ''}
              <strong>{e.statut_apres}</strong> par {e.agent_nom}
              {e.note && <span className="note"> — {e.note}</span>}
            </li>
          ))}
        </ol>
      </div>

      {calcul && <Resultat calcul={calcul} />}
    </section>
  )
}

function Resultat({ calcul }: { calcul: ReponseEcheancier }) {
  const r = calcul.recapitulatif
  const [tout, poserTout] = useState(false)
  const lignes = tout ? calcul.echeancier : calcul.echeancier.slice(0, 12)

  return (
    <>
      <div className="carte">
        <h3>Récapitulatif</h3>
        <dl className="fiche">
          <div><dt>Première mensualité</dt><dd>{millimes(r.premiere_mensualite)} DT</dd></div>
          <div><dt>Dernière mensualité</dt><dd>{millimes(r.derniere_mensualite)} DT</dd></div>
          <div><dt>Total des intérêts</dt><dd>{millimes(r.total_interets)} DT</dd></div>
          <div><dt>Coût du crédit</dt><dd>{millimes(r.cout_credit)} DT</dd></div>
          <div><dt>Total versé</dt><dd>{millimes(r.total_verse)} DT</dd></div>
          <div><dt>TEG</dt><dd>{pourcent(r.teg)}</dd></div>
          {r.interets_capitalises > 0 && (
            <div><dt>Intérêts capitalisés</dt><dd>{millimes(r.interets_capitalises)} DT</dd></div>
          )}
        </dl>

        {r.conforme !== null && (
          <p className={r.conforme ? 'verdict conforme' : 'verdict excessif'}>
            {r.conforme
              ? `Taux conforme : TEG ${pourcent(r.teg)} contre un seuil de ${pourcent(r.seuil_excessif)}`
              : `Taux excessif : TEG ${pourcent(r.teg)} dépasse le seuil de ${pourcent(r.seuil_excessif)}`}
            {calcul.bareme && (
              <span className="discret petit">
                {' '}— {libelleCategorie(calcul.bareme.categorie)}, {calcul.bareme.arrete}
              </span>
            )}
          </p>
        )}
      </div>

      <div className="carte">
        <div className="entete-section">
          <h3>Échéancier</h3>
          <button onClick={() => poserTout((t) => !t)}>
            {tout ? 'Réduire' : `Tout afficher (${calcul.echeancier.length})`}
          </button>
        </div>
        <div className="defilement">
          <table>
            <thead>
              <tr>
                <th className="nombre">N°</th>
                <th className="nombre">Mensualité</th>
                <th className="nombre">Intérêts</th>
                <th className="nombre">Capital</th>
                <th className="nombre">Capitalisé</th>
                <th className="nombre">Solde</th>
              </tr>
            </thead>
            <tbody>
              {lignes.map((e) => (
                <tr key={e.n}>
                  <td className="nombre">{e.n}</td>
                  <td className="nombre">{millimes(e.mensualite)}</td>
                  <td className="nombre">{millimes(e.interets)}</td>
                  <td className="nombre">{millimes(e.capital)}</td>
                  <td className="nombre">
                    {e.capitalise > 0 ? millimes(e.capitalise) : '—'}
                  </td>
                  <td className="nombre">{millimes(e.solde)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  )
}
