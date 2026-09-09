import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { api, poserLeJeton, type Agent } from './api'

interface Contexte {
  agent: Agent | null
  connexion: (identifiant: string, motDePasse: string) => Promise<void>
  deconnexion: () => Promise<void>
}

const ContexteAuth = createContext<Contexte | null>(null)

// La prop garde le nom que React lui donne : c'est lui qui la pose.
export function FournisseurAuth({ children }: { children: ReactNode }) {
  const [agent, poserAgent] = useState<Agent | null>(null)

  const connexion = useCallback(
    async (identifiant: string, motDePasse: string) => {
      const rep = await api.connexion(identifiant, motDePasse)
      // Le jeton ne quitte pas la memoire : ni localStorage, ni cookie.
      poserLeJeton(rep.jeton)
      poserAgent(rep.agent)
    },
    [],
  )

  const deconnexion = useCallback(async () => {
    try {
      await api.deconnexion()
    } finally {
      // Meme si la revocation cote serveur echoue, la session locale est
      // abandonnee : l'utilisateur a demande a partir.
      poserLeJeton(null)
      poserAgent(null)
    }
  }, [])

  const valeur = useMemo(
    () => ({ agent, connexion, deconnexion }),
    [agent, connexion, deconnexion],
  )

  return (
    <ContexteAuth.Provider value={valeur}>{children}</ContexteAuth.Provider>
  )
}

export function useAuth(): Contexte {
  const contexte = useContext(ContexteAuth)
  if (!contexte) {
    throw new Error("useAuth doit etre appele sous FournisseurAuth")
  }
  return contexte
}
