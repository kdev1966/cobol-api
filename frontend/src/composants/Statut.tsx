import type { Statut } from '../api'

const libelles: Record<Statut, string> = {
  brouillon: 'Brouillon',
  en_instruction: 'En instruction',
  accorde: 'Accordé',
  refuse: 'Refusé',
  annule: 'Annulé',
}

export function libelleStatut(statut: Statut): string {
  return libelles[statut] ?? statut
}

export default function Etiquette({ statut }: { statut: Statut }) {
  return <span className={`etiquette ${statut}`}>{libelles[statut]}</span>
}
