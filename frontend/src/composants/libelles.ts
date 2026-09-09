// Les valeurs canoniques de l'API sont en snake_case : elles font de bons
// identifiants, de mauvais libelles. La traduction est ici, en un seul endroit.

const methodes: Record<string, string> = {
  annuite_constante: 'Annuité constante',
  capital_constant: 'Capital constant',
  in_fine: 'In fine',
}

const categories: Record<string, string> = {
  credits_consommation: 'Crédits à la consommation',
  credits_court_terme: 'Crédits à court terme',
  credits_logement: 'Crédits au logement',
  credits_long_terme: 'Crédits à long terme',
  credits_moyen_terme: 'Crédits à moyen terme',
  decouverts: 'Découverts',
  gestion_des_dettes: 'Gestion des dettes',
  leasing: 'Leasing',
}

// Repli sur la valeur brute : un libelle manquant vaut mieux qu'une case vide,
// et signale du meme coup qu'une categorie a ete ajoutee au bareme.
export function libelleMethode(valeur: string): string {
  return methodes[valeur] ?? valeur
}

export function libelleCategorie(valeur: string | null): string {
  if (!valeur) return '—'
  return categories[valeur] ?? valeur
}

export function libelleDiffere(mois: number, type: string): string {
  if (mois === 0) return 'aucun'
  const nature = type === 'total' ? 'total, intérêts capitalisés' : 'partiel'
  return `${mois} mois (${nature})`
}
