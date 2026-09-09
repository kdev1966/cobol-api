import { Navigate, Route, Routes, NavLink } from 'react-router-dom'
import { useAuth } from './auth'
import Connexion from './pages/Connexion'
import Dossiers from './pages/Dossiers'
import Dossier from './pages/Dossier'

export default function App() {
  const { agent, deconnexion } = useAuth()

  // Le jeton vit en memoire : sans agent, il n'y a pas de session a reprendre,
  // et toute route mene a la connexion.
  if (!agent) return <Connexion />

  return (
    <div className="application">
      <header>
        <nav>
          <NavLink to="/dossiers">Dossiers</NavLink>
        </nav>
        <div className="agent">
          <span>
            {agent.nom} <span className="discret">— {agent.agence}</span>
          </span>
          <button onClick={() => void deconnexion()}>Se déconnecter</button>
        </div>
      </header>

      <main>
        <Routes>
          <Route path="/dossiers" element={<Dossiers />} />
          <Route path="/dossiers/:id" element={<Dossier />} />
          <Route path="*" element={<Navigate to="/dossiers" replace />} />
        </Routes>
      </main>
    </div>
  )
}
