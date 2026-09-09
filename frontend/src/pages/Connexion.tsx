import { useState, type FormEvent } from 'react'
import { useAuth } from '../auth'
import { ErreurAPI } from '../api'

export default function Connexion() {
  const { connexion } = useAuth()
  const [identifiant, poserIdentifiant] = useState('')
  const [motDePasse, poserMotDePasse] = useState('')
  const [erreur, poserErreur] = useState<string | null>(null)
  const [enCours, poserEnCours] = useState(false)

  async function soumettre(e: FormEvent) {
    e.preventDefault()
    poserErreur(null)
    poserEnCours(true)
    try {
      await connexion(identifiant, motDePasse)
    } catch (err) {
      poserErreur(
        err instanceof ErreurAPI ? err.message : 'Service injoignable',
      )
    } finally {
      poserEnCours(false)
    }
  }

  return (
    <div className="connexion">
      <form onSubmit={soumettre} className="carte">
        <h1>Dossiers de prêt</h1>
        <p className="discret">
          Les comptes sont créés par un administrateur. Il n'y a pas
          d'inscription.
        </p>

        <label>
          Identifiant
          <input
            value={identifiant}
            onChange={(e) => poserIdentifiant(e.target.value)}
            autoComplete="username"
            required
            autoFocus
          />
        </label>

        <label>
          Mot de passe
          <input
            type="password"
            value={motDePasse}
            onChange={(e) => poserMotDePasse(e.target.value)}
            autoComplete="current-password"
            required
          />
        </label>

        {erreur && <p className="erreur">{erreur}</p>}

        <button type="submit" disabled={enCours}>
          {enCours ? 'Connexion…' : 'Se connecter'}
        </button>

        <p className="discret petit">
          La session est gardée en mémoire seulement : rafraîchir la page
          demande de se reconnecter.
        </p>
      </form>
    </div>
  )
}
