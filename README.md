# cobol-api

API REST qui expose un moteur d'amortissement de prêt écrit en COBOL, conforme
à la réglementation tunisienne du crédit.

Le partage des rôles est délibéré. Le COBOL fait l'arithmétique parce qu'il la
fait exactement : `PIC S9(11)V999` stocke des millimes en décimal, là où le
`float64` de Go ou de JavaScript ne peut pas représenter 0,07. Sur un
échéancier de 240 mensualités, cette dérive devient des millimes qui ne
réconcilient pas. Go fait le HTTP, la validation et le contrôle d'accès.

**Les montants sont en dinars tunisiens, exprimés au millime** — le dinar se
divise en mille, pas en cent. Toutes les valeurs portent trois décimales.

## Démarrage

```sh
cp .env.example .env
# renseigner API_KEY et POSTGRES_PASSWORD :
#   openssl rand -hex 32   pour la cle
#   openssl rand -hex 24   pour le mot de passe
docker compose up -d --build
```

Deux services démarrent : PostgreSQL, qui porte les barèmes réglementaires, et
l'API. Le service **refuse de démarrer si la base est injoignable** plutôt que
de répondre à côté, et applique ses migrations au démarrage.

```sh
export K=$(grep '^API_KEY=' .env | cut -d= -f2)
curl -s -H "X-API-Key: $K" \
  'http://localhost:3000/v1/loans/schedule?capital=250000.00&taux=3.45&mois=240' | jq
```

```json
{
  "status": "success",
  "demande": {
    "capital": "250000.000", "taux": "8.500000", "mois": 240,
    "methode": "annuite_constante",
    "frais_dossier": "0.000", "frais_garantie": "0.000",
    "taux_assurance": "0.000000", "assiette_assurance": "aucune"
  },
  "recapitulatif": {
    "echeances": 240,
    "premiere_mensualite": 2169.558,
    "derniere_mensualite": 2169.614,
    "total_interets": 270693.976,
    "total_assurance": 0.000,
    "total_frais": 0.000,
    "total_verse": 520693.976,
    "cout_credit": 270693.976,
    "teg": 8.50,
    "tem": null, "seuil_excessif": null, "conforme": null, "marge": null
  },
  "echeancier": [
    { "n": 1, "echeance": 2169.558, "interets": 1770.833, "capital": 398.725,
      "assurance": 0.000, "mensualite": 2169.558, "solde": 249601.275 },
    { "n": 240, "echeance": 2169.614, "interets": 15.260, "capital": 2154.354,
      "assurance": 0.000, "mensualite": 2169.614, "solde": 0.000 }
  ]
}
```

La dernière échéance vaut 2 169,614 DT et non 2 169,558 : elle absorbe le
résidu d'arrondi accumulé sur les 239 mois précédents, comme le veut la
pratique bancaire.

## Les trois méthodes

| `methode` | Comportement |
|---|---|
| `annuite_constante` *(défaut)* | L'échéance ne bouge pas ; la part de capital croît à mesure que les intérêts diminuent. |
| `capital_constant` | La part de capital ne bouge pas ; l'échéance décroît. Amortissement plus rapide, donc moins d'intérêts. |
| `in_fine` | Intérêts seuls chaque mois, capital remboursé en totalité à la dernière échéance. Le plus coûteux. |

Sur 120 000 € à 4,2 % sur 60 mois, le total des intérêts va de **12 810 €** à
capital constant, à **13 249,77 €** en annuité constante, à **25 200 €** en in
fine — un écart que la suite de tests vérifie comme un invariant à part
entière.

Sous `annuite_constante`, toutes les échéances sauf la dernière valent
`premiere_echeance`. Sous les deux autres méthodes, l'échéance varie à chaque
période ; le récapitulatif rend donc la première et la dernière.

## Frais et assurance

Quatre paramètres facultatifs entrent dans les flux :

| Paramètre | Effet |
|---|---|
| `frais_dossier`, `frais_garantie` | Versés au départ. Ils diminuent ce que l'emprunteur perçoit **sans réduire ce qu'il rembourse**, et font donc monter le TEG. |
| `taux_assurance` | Taux annuel de l'assurance emprunteur, en pourcent. |
| `assiette_assurance` | `capital_initial` — prime constante ; `capital_restant_du` — prime décroissante ; `aucune`. Un taux déclaré sans assiette porte sur le capital initial. |

Sur 250 000 DT à 8,5 % sur 240 mois :

| | TEG | coût du crédit |
|---|---|---|
| Nu | 8,50 % | 270 693,976 DT |
| + 1 500 DT de frais de dossier | 8,58 % | 272 193,976 DT |
| + assurance 0,36 % sur capital initial | 8,97 % | 288 693,976 DT |

Chaque ligne de l'échéancier distingue `echeance` (capital + intérêts) de
`mensualite` (échéance + assurance), ce que l'emprunteur verse réellement.

## Le différé d'amortissement

Une franchise peut précéder l'amortissement : pendant `differe` échéances,
l'emprunteur ne paie que les intérêts et l'assurance, le capital reste intact.
L'amortissement se fait ensuite sur la durée restante.

Sur 250 000 DT à 8,5 % sur 240 mois avec 24 mois de différé :

```
n=  1   échéance 1770,833   intérêts 1770,833   capital 0,000     solde 250000,000
n= 24   échéance 1770,833   intérêts 1770,833   capital 0,000     solde 250000,000
n= 25   échéance 2263,644   intérêts 1770,833   capital 492,811   solde 249507,189
n=240   échéance 2263,471   intérêts   15,920   capital 2247,551  solde      0,000
```

Le crédit coûte plus cher — 281 446,923 DT d'intérêts contre 270 693,976 — et
l'échéance d'amortissement est plus élevée, le même capital devant être remboursé
en 216 mensualités au lieu de 240. Les quatre invariants tiennent inchangés : la
somme des parts de capital reste exactement le capital emprunté.

**Les intérêts ne sont pas capitalisés.** Un différé total le ferait, et
demanderait de distinguer sur chaque ligne les intérêts payés de ceux ajoutés au
capital — une colonne de plus, et l'invariant 1 à reformuler. Ce n'est pas fait.

## Le remboursement anticipé

Solder le capital restant dû à une échéance donnée, moyennant une indemnité.
La réponse compare les deux trajectoires :

```json
"anticipe": {
  "mois": 120,
  "solde_restant": 174984.575,
  "indemnite": 1749.846,
  "total_anticipe": 437081.381,
  "total_terme": 520693.976,
  "economie": 83612.595,
  "interets_economises": 85362.441
}
```

**L'indemnité n'est pas encadrée par la loi tunisienne mais par le contrat** :
les banques pratiquent couramment 1 à 1,5 % du capital restant dû, et c'est
négociable. Le taux est donc un paramètre, sans plafond imposé par le service.

Deux propriétés vérifiées par la suite de tests : solder à la dernière échéance
équivaut exactement à aller au terme — économie nulle, totaux identiques — et
une indemnité assez forte peut rendre l'opération perdante, auquel cas
l'économie est **nulle et jamais négative**.

## Le remboursement partiel

Rembourser une part du capital en cours de prêt, sans solder. Deux suites
possibles, et c'est l'emprunteur qui choisit :

| Mode | Ce qui bouge | Ce qui ne bouge pas |
|---|---|---|
| `duree_reduite` | la durée raccourcit | l'échéance reste la même |
| `echeance_reduite` | l'échéance s'allège | la durée reste la même |

```json
"remboursement_partiel": {
  "mois": 120,
  "montant": 50000.000,
  "indemnite": 500.000,
  "mode": "duree_reduite",
  "duree": 195,
  "echeance_suivante": 2169.558,
  "total": 472019.531,
  "total_terme": 520693.976,
  "economie": 48674.445,
  "interets_economises": 49174.445
}
```

Sur le prêt de référence, rembourser 50 000 DT au 120ᵉ mois raccourcit le prêt
de 45 mois et économise 48 674,445 DT, contre 23 891,432 DT si l'on allège
l'échéance. **À montant égal, raccourcir la durée rapporte davantage** : le
capital cesse plus tôt de porter intérêt. C'est vérifié par un test, pas
seulement affirmé ici.

L'indemnité porte sur le **capital remboursé**, non sur le solde restant —
d'où 500 DT ici, contre 1 749,846 DT pour un solde total au même mois.

`duree_reduite` est refusé avec la méthode `in_fine` : aucun capital n'y est
amorti avant le terme, il n'y a donc pas de durée à raccourcir.

**L'échéancier rendu reste celui du contrat.** Un remboursement anticipé est
une décision de l'emprunteur, pas une clause du prêt : le service le simule
sans réécrire le tableau d'amortissement contractuel. Pour la même raison, le
**TEG affiché reste celui du contrat** — le décret n° 2000-462 le définit sur
l'échéancier contractuel, et un TEG recalculé sur une trajectoire écourtée
ressemblerait à la mention légale sans en être une.

## La capacité d'emprunt

Le calcul inverse : à partir d'une mensualité supportable, le capital maximal
empruntable. Un second programme COBOL le cherche **par dichotomie**.

La formule fermée existe pour chaque méthode, mais elle ignore les arrondis :
le capital qu'elle rend produit parfois une mensualité d'un millime au-dessus
du budget. La dichotomie retient le plus grand capital dont la première
mensualité tient réellement dans le budget — un millime de plus le dépasse, ce
qu'un test vérifie contre l'échéancier produit par l'autre programme.

À 2 000 DT par mois, 8,5 % sur 240 mois :

| méthode | capital |
|---|---|
| capital constant | 177 777,811 DT |
| annuité constante | 230 461,737 DT |
| in fine | 282 353,011 DT |

Plus l'amortissement est lent, plus on peut emprunter à mensualité égale.

Avec un différé, la contrainte porte sur la **première échéance amortissante**
et non sur la franchise, qui ne paie que les intérêts : s'y fier donnerait une
capacité follement optimiste. Un différé de 24 mois ramène donc la capacité de
230 461,737 à **220 882,879 DT**.

La réponse porte aussi **l'échéancier complet** que ce capital produit, son TEG
et le verdict de taux excessif : savoir combien on peut emprunter n'a d'intérêt
que si l'on voit ce que ça donne. Deux programmes COBOL s'enchaînent, d'où une
requête à **34,2 ms** contre 22,4 pour un échéancier seul.

## Le taux excessif

La [loi n° 99-64 du 15 juillet 1999](https://www.jurisitetunisie.com/tunisie/codes/teg/tie1000.htm)
définit comme excessif tout prêt dont le TEG **excède de plus du cinquième** le
taux effectif moyen pratiqué au semestre précédent pour la même catégorie de
concours. Deux façons d'obtenir le verdict. La plus sûre est de nommer la **catégorie de
concours** : le taux effectif moyen est alors lu dans le barème, et la réponse
**cite l'arrêté** sur lequel elle repose.

```sh
curl -s -H "X-API-Key: $K" \
  '.../v1/loans/schedule?capital=60000.000&taux=13&mois=60&categorie=credits_consommation' | jq .bareme
```
```json
{
  "categorie": "credits_consommation", "semestre": "2026S1", "tem": "11.23",
  "arrete": "Arrete du 28 juillet 2026", "publie_le": "2026-07-28"
}
```

Un verdict de taux excessif ne vaut que rapporté au barème qui le fonde ; la
réponse le porte donc avec elle.

L'alternative est de passer directement `tem`. Les deux paramètres **s'excluent** :
les accepter ensemble ouvrirait la porte à un verdict rendu sur un taux
différent de celui annoncé. Dans les deux cas :

```json
"tem": 10.25, "seuil_excessif": 12.30, "conforme": true, "marge": 3.80
```

Le seuil est calculé par le programme COBOL — c'est la règle légale, pas une
donnée — en majorant le TEM d'un cinquième et en arrondissant à deux décimales.
Un TEG **égal** au seuil reste licite : le dépassement est strict.

Les taux effectifs moyens sont publiés par arrêté du ministre des finances, sur
proposition de la Banque Centrale de Tunisie, au dernier mois de chaque
semestre. Le service les conserve en base, par catégorie et par semestre, et
`GET /v1/baremes` rend ceux en vigueur. Une suite de tests vérifie que le
calcul du seuil redonne bien les huit valeurs publiées par l'arrêté du
28 juillet 2026 :

| Catégorie | TEM | Seuil |
|---|---|---|
| Leasing | 13,37 % | 16,04 % |
| Découverts | 12,29 % | 14,75 % |
| Gestion des dettes | 11,78 % | 14,14 % |
| Crédits à la consommation | 11,23 % | 13,48 % |
| Crédits logement | 10,25 % | 12,30 % |
| Crédits à moyen terme | 9,80 % | 11,76 % |
| Crédits à long terme | 9,63 % | 11,56 % |
| Crédits à court terme | 9,57 % | 11,48 % |

Sans `categorie` ni `tem`, les quatre champs sont `null` et aucun barème n'est
cité : le service rend le TEG sans le juger.

## Le TEG

Le TEG est calculé selon le [décret n° 2000-462 du 21 février 2000](https://www.jurisitetunisie.com/tunisie/codes/teg/teg1000.htm),
qui impose deux choses souvent mal comprises.

**L'annualisation est proportionnelle, pas actuarielle.** Le décret parle d'« un
taux annuel, **proportionnel** au taux d'intérêt de la période » : le taux de
période est multiplié par le nombre de périodes annuelles, il n'est pas
capitalisé. Conséquence directe et vérifiable : sans frais ni assurance, le TEG
**égale exactement le taux nominal**. Un prêt à 8,5 % rend un TEG de 8,50 %.

**Le taux de période, lui, est bien résolu par méthode actuarielle**, en
égalisant la valeur actuelle de tous les versements dus au montant réellement
perçu. Le service le résout par dichotomie, en arithmétique décimale exacte,
puis annualise proportionnellement et arrondit à deux décimales comme l'exige
le décret.

Entrent dans le calcul les intérêts, frais, commissions et rémunérations de
toute nature, directs ou indirects. En sont exclus les impôts et droits perçus
au profit de l'État, et les commissions sans lien avec le crédit.

Dès qu'un frais ou une prime d'assurance entre dans les flux, aucune formule
fermée ne donne le résultat : sur 250 000 DT à 8,5 % sur 240 mois, le TEG passe
de 8,50 % à 8,58 % avec 1 500 DT de frais de dossier, et à 8,97 % avec une
assurance à 0,36 % sur le capital initial.

## Les invariants

Le moteur tient six garanties, vérifiées par la suite de tests sur neuf jeux de
paramètres allant du taux nul à 600 mensualités, puis sur six combinaisons de
frais et d'assurance, le tout pour chacune des trois méthodes :

1. la somme des parts de capital égale **exactement** le capital emprunté ;
2. la somme des échéances égale **exactement** capital + intérêts ;
3. le solde après la dernière échéance est **exactement** nul ;
4. sur chaque ligne, échéance = capital + intérêts ;
5. sur chaque ligne, mensualité = échéance + assurance ;
6. le total versé égale la somme des mensualités.

Les montants ne transitent jamais par un flottant : le COBOL écrit des
chiffres, Go y insère le point décimal et les transporte en `json.Number`
jusqu'à la réponse.

## Endpoints

| Méthode | Chemin | Clé requise | Description |
|---|---|---|---|
| `GET` | `/` | non | Index du service |
| `GET` | `/openapi.json` | non | Spécification OpenAPI 3.1 |
| `GET` | `/v1/loans/schedule?capital=&taux=&mois=&methode=…` | oui | Échéancier de prêt |
| `GET` | `/v1/loans/capacity?mensualite=&taux=&mois=…` | oui | Capital maximal empruntable |
| `GET` | `/v1/baremes` | oui | Taux effectifs moyens en vigueur, par catégorie |
| `GET` | `/v1/simulations?limite=&non_conformes=` | oui | Piste d'audit des échéanciers produits |
| `GET` | `/health` | non | État du service ; `503` si le binaire COBOL manque |

Les routes métier sont versionnées. Le format du récapitulatif a déjà changé
une fois ; un client tiers ne doit pas en pâtir au prochain changement.

La spécification est **embarquée dans le binaire** et servie telle quelle : elle
est versionnée avec le code qu'elle décrit et ne peut pas en diverger. Un test
vérifie que chaque chemin qu'elle décrit répond réellement.

Paramètres : `capital` de 0.001 à 99999999999.999 dinars, `taux` nominal annuel en
pourcent de 0 à 99.999999, `mois` de 1 à 600, `methode` parmi les trois
libellés ci-dessus — les lettres `A`, `C` et `I` sont acceptées comme alias.
Un paramètre invalide rend un
`400` nommant le champ fautif ; une clé absente ou invalide un `401` ; un
dépassement du plafond de requêtes un `429` ; un calcul qui dépasse son délai
un `504`.

Chaque réponse porte un `X-Request-Id` que l'on retrouve dans les journaux.

La clé passe dans l'en-tête `X-API-Key`. `/health` reste ouvert et hors
plafond parce que le `HEALTHCHECK` du conteneur s'appuie dessus.

## Configuration

| Variable | Défaut | Rôle |
|---|---|---|
| `API_KEY` | — | Clé attendue sur les endpoints protégés. **Obligatoire quand `APP_ENV=production`** : sans elle, le serveur refuse de démarrer. Absente hors production, le service tourne ouvert avec un avertissement. |
| `RATE_LIMIT_PER_MINUTE` | `30` | Requêtes par minute et par adresse IP |
| `CORS_ORIGINS` | vide | Origines navigateur autorisées, séparées par des virgules. Vide = aucune origine croisée. |
| `PORT` | `3000` | Port d'écoute |
| `COBOL_PROGRAM_PATH` | `/app/bin/loan_amortization` | Binaire COBOL de l'échéancier |
| `COBOL_CAPACITY_PATH` | `/app/bin/loan_capacity` | Binaire COBOL du calcul inverse |
| `DATABASE_URL` | — | Adresse PostgreSQL. **Obligatoire** : le service ne démarre pas sans base. |
| `COMPUTE_TIMEOUT_SECONDS` | `5` | Délai maximal d'un calcul. Au-delà, le processus COBOL est tué et la requête rend `504`. Un échéancier de 600 mois avec résolution du TEG prend une vingtaine de millisecondes. |
| `APP_ENV` | vide | `production` rend `API_KEY` obligatoire |

Le plafond de débit porte sur l'adresse vue par le serveur. Les en-têtes
`X-Forwarded-For` sont volontairement ignorés : ils sont falsifiables tant
qu'aucun proxy de confiance n'est déclaré. Derrière un proxy inverse, il faut
donc en tenir compte avant de se fier au plafond.

## Contrat du programme COBOL

Le binaire est autonome et testable sans la couche Go. Il lit sur son entrée
standard **une ligne de 64 caractères** — capital `9(11)V999`, taux annuel
`9(2)V9(6)`, durée `9(4)`, frais de dossier et de garantie `9(9)V999`, taux
d'assurance `9(2)V9(6)`, taux effectif moyen `9(2)V99`, puis les lettres de la
méthode (`A`, `C`, `I`) et de l'assiette d'assurance (`N`, `I`, `R`) — et écrit
des enregistrements à largeur fixe :

```
R + échéances 9(4) + première et dernière mensualité 9(11)V999
                   + total intérêts et total assurance 9(13)V999
                   + total frais 9(9)V999
                   + total versé et coût du crédit 9(13)V999
                   + TEG, TEM et seuil 9(2)V99
                   + conformité X + marge S9(2)V99 à signe séparé             129 car.
E + numéro    9(4) + échéance, intérêts, capital, assurance,
                     mensualité, solde 9(11)V999                               89 car.
```

```sh
$ echo "00000250000000085000000240000000000000000000000000000000000000AN" | ./bin/loan_amortization | head -2
R02400000000216955800000002169614000000027069397600000000000000000000000000000000000005206939760000000270693976085000000000-+0000
E0001000000021695580000000177083300000000398725000000000000000000000216955800000249601275
```

Codes de sortie : `2` si l'entrée est malformée, `3` si le capital ou la durée
sont nuls ou la durée supérieure à 600 mois, `4` si la méthode ou l'assiette
d'assurance est inconnue.

**`JSON GENERATE` n'est délibérément pas utilisé.** Le paquet GnuCOBOL des
distributions est construit avec `JSON library: not found` : l'instruction
compile, s'exécute, et ne produit rien — ni erreur, ni avertissement. Même
piège pour `JSON PARSE`, non implémentée, qui rend des champs à zéro sans
signaler quoi que ce soit. La mise en forme revient donc à l'appelant.

## Développement

Il faut GnuCOBOL (`cobc`) et Go 1.24 ou plus.

```sh
mkdir -p bin
cobc -x -free cobol/loan-amortization.cbl -o bin/loan_amortization
cobc -x -free cobol/loan-capacity.cbl -o bin/loan_capacity
go test ./...
go run ./cmd/cobol-api
```

**Attention** : les tests qui ont besoin du binaire COBOL ou de PostgreSQL se
sautent d'eux-mêmes en leur absence, et `go test` affiche `ok` malgré tout.
Avec les deux, 56 tests s'exécutent ; sans base, 5 se sautent. La CI fournit
toujours les deux.

Pour une base de test locale :

```sh
docker run -d --name pg-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=cobol_api \
  -p 55432:5432 postgres:17-alpine
export DATABASE_URL="postgres://postgres:test@localhost:55432/cobol_api?sslmode=disable"
```

Le lecteur d'enregistrements et la conversion décimale sont couverts par du
fuzzing, exécuté en CI :

```sh
go test ./internal/loan/ -run '^$' -fuzz FuzzLireSortie -fuzztime 60s
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Sans `API_KEY`, le serveur démarre en laissant les endpoints ouverts et le
signale par un avertissement. Les tests qui ont besoin du binaire COBOL se
sautent d'eux-mêmes s'il n'a pas été compilé ; les autres tournent sans lui.

L'image exécute `go vet`, la suite de tests et un test de fumée sur le binaire
COBOL pendant sa construction : une régression bloque le build.

## La base de données

PostgreSQL porte les **barèmes réglementaires** : les taux effectifs moyens
publiés par arrêté, par catégorie de concours et par semestre.

```
taux_effectifs_moyens (categorie, semestre) -> tem, arrete, publie_le
```

Le seuil du taux excessif **n'y est pas stocké** : il se déduit du TEM par la
règle du cinquième, et cette règle appartient au programme COBOL. On ne
conserve que la donnée publiée, avec la référence de l'arrêté pour la
traçabilité réglementaire.

Un nouvel arrêté s'ajoute sans effacer le précédent : la table est un
historique, et la recherche retient le semestre le plus récent. Le format
`2026S1` rend l'ordre lexicographique et l'ordre chronologique identiques.

Les migrations sont embarquées dans le binaire et appliquées au démarrage, sous
un verrou consultatif PostgreSQL : plusieurs instances peuvent démarrer en même
temps sans appliquer deux fois la même migration. Un test le vérifie sur six
démarrages simultanés.

`/health` sonde réellement la base et rend `503` si elle ne répond pas.

## La piste d'audit

Chaque échéancier produit laisse une trace : la demande normalisée, le TEG, le
coût du crédit, le verdict, le barème appliqué, l'identifiant de corrélation,
l'adresse et l'horodatage. La réponse rend l'identifiant de la ligne inscrite.

**L'écriture est synchrone, et son échec fait échouer la requête.** Un service
qui produit des offres de crédit doit pouvoir dire ce qu'il a produit ; une
piste d'audit à trous n'en est pas une. Le coût est mesuré : la requête passe
de 19,5 à **22,4 ms** sur un échéancier de 240 mois.

**La clé d'API n'est jamais stockée.** Seule une empreinte de huit caractères,
tirée de son condensat, permet de distinguer les appelants — assez pour un
contrôle, inutile à un attaquant. Un test et une étape de CI vérifient qu'elle
n'apparaît nulle part.

`?non_conformes=true` restreint aux prêts jugés excessifs, ce que demande un
contrôle. La table porte un index partiel pour cette requête.

## Architecture

```
cobol/loan-amortization.cbl   règles métier et arithmétique exacte
cobol/loan-capacity.cbl       calcul inverse, par dichotomie
internal/loan/                formatage de la demande, appel du binaire, lecture
internal/api/                 routes, authentification, débit, en-têtes
internal/db/                  pool, migrations, barèmes, piste d'audit
cmd/cobol-api/                démarrage et arrêt propre
internal/api/openapi.json     le contrat, embarqué dans le binaire
```

À l'arrêt, les requêtes en vol sont drainées **avant** la fermeture du pool :
l'inverse les ferait échouer sur la ligne d'arrivée.

Les journaux sont structurés — JSON en production, texte ailleurs — et chaque
requête achevée est tracée avec son identifiant, son statut et sa durée.

Le service lance un processus COBOL par requête. Ce lancement coûte environ
4,7 ms, contre quelques microsecondes pour le calcul lui-même — mais il achète
l'isolation : un abend COBOL tue un processus jetable et rend une erreur, là
où un appel en direct emporterait le serveur. Un échéancier complet étant
produit par un seul lancement, ce coût est amorti sur toute la réponse.

L'image est construite en trois étapes. Ni `cobc`, ni `gcc`, ni la chaîne Go
ne sont présents à l'exécution : il ne reste que le binaire Go statique, le
binaire COBOL, et `libcob`. Le conteneur tourne sous un utilisateur non
privilégié et n'écrit rien sur disque.

## Limites connues

- Les échéances sont mensuelles. Aucune autre périodicité n'est proposée.
- Le taux périodique est **proportionnel** (taux nominal annuel divisé par
  douze), et non le taux actuariel équivalent. C'est un choix, pas un oubli.
- Pas d'échéances irrégulières.
- Le barème doit être alimenté à chaque nouvel arrêté, par migration. Aucune
  interface d'administration n'existe.
- La piste d'audit n'a ni purge ni rétention : elle croît indéfiniment.
- Le différé total, qui capitalise les intérêts de la franchise, n'est pas
  implémenté.
- Le remboursement anticipé, total ou partiel, est simulé : l'échéancier rendu
  reste celui du contrat. Un tableau d'amortissement réécrit après l'opération
  n'est pas proposé.
- Un seul remboursement anticipé par simulation. Des versements exceptionnels
  répétés demanderaient une liste d'opérations en entrée.
