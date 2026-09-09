// Client de l'API. Le jeton de session est garde en memoire, jamais dans
// localStorage : un jeton porteur y survivrait a la fermeture de l'onglet, et
// un script injecte pourrait l'y lire a loisir. Rafraichir la page redemande
// donc une connexion, ce qui est le comportement voulu.
let jeton: string | null = null

export function poserLeJeton(valeur: string | null) {
  jeton = valeur
}

export function jetonPresent(): boolean {
  return jeton !== null
}

// En developpement, Vite mandate /v1 vers le backend et la base reste vide.
const base = import.meta.env.VITE_API_URL ?? ''

export class ErreurAPI extends Error {
  constructor(
    readonly statut: number,
    message: string,
    readonly champ?: string,
  ) {
    super(message)
  }
}

async function appeler<T>(
  chemin: string,
  options: RequestInit = {},
): Promise<T> {
  const entetes = new Headers(options.headers)
  if (options.body) entetes.set('Content-Type', 'application/json')
  if (jeton) entetes.set('Authorization', `Bearer ${jeton}`)

  const reponse = await fetch(base + chemin, { ...options, headers: entetes })

  if (reponse.status === 204) return undefined as T

  const texte = await reponse.text()
  let corps: Record<string, unknown> = {}
  try {
    corps = texte ? JSON.parse(texte) : {}
  } catch {
    throw new ErreurAPI(reponse.status, 'Reponse illisible du service')
  }

  if (!reponse.ok) {
    throw new ErreurAPI(
      reponse.status,
      (corps.message as string) ?? 'Erreur inattendue',
      corps.champ as string | undefined,
    )
  }
  return corps as T
}

// --- Types rendus par l'API ---
//
// Les montants arrivent en nombres JSON. Le service les produit avec leurs
// decimales — « 250000.000 » — mais JSON.parse les rend en flottants, ce qui
// perd les zeros de queue. Le frontend ne fait donc aucun calcul dessus : il
// les met en forme a l'affichage, et seulement la.

export interface Agent {
  id: number
  identifiant: string
  nom: string
  agence: string
  actif: boolean
  cree_le: string
  derniere_connexion: string | null
}

export type Statut =
  | 'brouillon'
  | 'en_instruction'
  | 'accorde'
  | 'refuse'
  | 'annule'

export interface Dossier {
  id: number
  reference: string
  agent_id: number
  agence: string
  statut: Statut
  capital: string
  taux: string
  mois: number
  methode: string
  differe: number
  type_differe: string
  categorie: string | null
  simulation_id: number | null
  cree_le: string
  maj_le: string
  transitions_possibles: Statut[]
}

export interface Evenement {
  id: number
  agent_id: number
  agent_nom: string
  statut_avant: string | null
  statut_apres: string
  note: string | null
  cree_le: string
}

export interface Echeance {
  n: number
  echeance: number
  interets: number
  capital: number
  assurance: number
  mensualite: number
  solde: number
  capitalise: number
}

export interface Recapitulatif {
  echeances: number
  premiere_mensualite: number
  derniere_mensualite: number
  total_interets: number
  total_assurance: number
  total_frais: number
  total_verse: number
  cout_credit: number
  interets_capitalises: number
  teg: number
  tem: number | null
  seuil_excessif: number | null
  conforme: boolean | null
  marge: number | null
}

export interface Bareme {
  categorie: string
  taux: number
  semestre: string
  arrete: string
}

export interface ReponseEcheancier {
  recapitulatif: Recapitulatif
  echeancier: Echeance[]
  bareme?: Bareme
  dossier?: Dossier
  simulation_id?: number
}

// --- Routes ---

export const api = {
  connexion: (identifiant: string, motDePasse: string) =>
    appeler<{ jeton: string; agent: Agent; expire_le: string }>(
      '/v1/auth/connexion',
      {
        method: 'POST',
        body: JSON.stringify({ identifiant, mot_de_passe: motDePasse }),
      },
    ),

  deconnexion: () =>
    appeler<void>('/v1/auth/deconnexion', { method: 'POST' }),

  moi: () => appeler<{ agent: Agent }>('/v1/auth/moi'),

  listerDossiers: (statut?: string) =>
    appeler<{ dossiers: Dossier[]; total: number }>(
      '/v1/dossiers' + (statut ? `?statut=${encodeURIComponent(statut)}` : ''),
    ),

  lireDossier: (id: number) =>
    appeler<{ dossier: Dossier; historique: Evenement[] }>(
      `/v1/dossiers/${id}`,
    ),

  creerDossier: (corps: Record<string, unknown>) =>
    appeler<{ dossier: Dossier }>('/v1/dossiers', {
      method: 'POST',
      body: JSON.stringify(corps),
    }),

  modifierDossier: (id: number, corps: Record<string, unknown>) =>
    appeler<{ dossier: Dossier }>(`/v1/dossiers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(corps),
    }),

  changerLeStatut: (id: number, statut: Statut, note: string) =>
    appeler<{ dossier: Dossier }>(`/v1/dossiers/${id}/statut`, {
      method: 'POST',
      body: JSON.stringify({ statut, note }),
    }),

  echeancierDossier: (id: number) =>
    appeler<ReponseEcheancier>(`/v1/dossiers/${id}/echeancier`),

  baremes: () => appeler<{ baremes: Bareme[] }>('/v1/baremes'),
}

// --- Mise en forme ---
//
// Aucun calcul : seulement de l'affichage. Un dinar vaut mille millimes, et un
// flottant double represente exactement les millimes jusqu'a 2^53, soit bien
// au-dela de tout montant de credit. Le formatage est donc exact ici.

export function millimes(valeur: number | null | undefined): string {
  if (valeur === null || valeur === undefined) return '—'
  return valeur.toLocaleString('fr-TN', {
    minimumFractionDigits: 3,
    maximumFractionDigits: 3,
  })
}

export function pourcent(valeur: number | null | undefined): string {
  if (valeur === null || valeur === undefined) return '—'
  return (
    valeur.toLocaleString('fr-TN', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }) + ' %'
  )
}

export function date(iso: string): string {
  return new Date(iso).toLocaleString('fr-TN', {
    dateStyle: 'short',
    timeStyle: 'short',
  })
}
