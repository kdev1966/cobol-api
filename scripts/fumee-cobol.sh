#!/bin/sh
# Test de fumee du binaire COBOL sur un cas de reference.
#
# Employe par le Dockerfile et par la CI : l'entree attendue et le resultat
# attendu ne vivent qu'ici. Les avoir recopies a trois endroits avait deja
# produit un build casse lorsque le format d'entree a change.
#
# Usage : fumee-cobol.sh <chemin-du-binaire>
set -e

binaire="${1:?usage: fumee-cobol.sh <chemin-du-binaire>}"

# 250 000,00 a 3,45 % sur 240 mois, annuite constante, sans frais ni assurance.
# Les six chiffres avant les lettres sont le plafond d'usure : nul, donc
# aucune verification demandee.
entree="0000025000000034500000240000000000000000000000000000000000000AN"

# Mensualite 1443,48 : le recapitulatif porte 0240 puis 0000000144348.
prefixe_attendu="^R02400000000144348"
lignes_attendues=241

sortie=$(mktemp)
trap 'rm -f "$sortie"' EXIT

echo "$entree" | "$binaire" > "$sortie"

if ! grep -q "$prefixe_attendu" "$sortie"; then
    echo "test de fumee : recapitulatif inattendu" >&2
    head -1 "$sortie" >&2
    exit 1
fi

# wc cadre son resultat avec des espaces sur BSD, pas sur GNU.
recues=$(wc -l < "$sortie" | tr -d " ")
if [ "$recues" -ne "$lignes_attendues" ]; then
    echo "test de fumee : $recues lignes, $lignes_attendues attendues" >&2
    exit 1
fi

echo "test de fumee : OK ($recues lignes, mensualite de reference conforme)"
