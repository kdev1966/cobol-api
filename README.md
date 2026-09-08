# cobol-api

API REST qui expose un moteur d'amortissement de prêt écrit en COBOL.

Le partage des rôles est délibéré. Le COBOL fait l'arithmétique parce qu'il la
fait exactement : `PIC S9(11)V99` stocke des centimes en décimal, là où le
`float64` de Go ou de JavaScript ne peut pas représenter 0,07. Sur un
échéancier de 240 mensualités, cette dérive devient des centimes qui ne
réconcilient pas. Go fait le HTTP, la validation et le contrôle d'accès.

## Démarrage

```sh
cp .env.example .env
# renseigner API_KEY, par exemple avec : openssl rand -hex 32
docker compose up -d --build
```

```sh
export K=$(grep '^API_KEY=' .env | cut -d= -f2)
curl -s -H "X-API-Key: $K" \
  'http://localhost:3000/v1/loans/schedule?capital=250000.00&taux=3.45&mois=240' | jq
```

```json
{
  "status": "success",
  "demande": {
    "capital": "250000.00", "taux": "3.450000", "mois": 240,
    "methode": "annuite_constante",
    "frais_dossier": "0.00", "frais_garantie": "0.00",
    "taux_assurance": "0.000000", "assiette_assurance": "aucune"
  },
  "recapitulatif": {
    "echeances": 240,
    "premiere_mensualite": 1443.48,
    "derniere_mensualite": 1444.93,
    "total_interets": 96436.65,
    "total_assurance": 0.00,
    "total_frais": 0.00,
    "total_verse": 346436.65,
    "cout_credit": 96436.65,
    "taeg": 3.5051
  },
  "echeancier": [
    { "n": 1, "echeance": 1443.48, "interets": 718.75, "capital": 724.73,
      "assurance": 0.00, "mensualite": 1443.48, "solde": 249275.27 },
    { "n": 240, "echeance": 1444.93, "interets": 4.14, "capital": 1440.79,
      "assurance": 0.00, "mensualite": 1444.93, "solde": 0.00 }
  ]
}
```

La dernière échéance vaut 1444,93 et non 1443,48 : elle absorbe le résidu
d'arrondi accumulé sur les 239 mois précédents, comme le veut la pratique
bancaire.

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
| `frais_dossier`, `frais_garantie` | Versés au départ. Ils diminuent ce que l'emprunteur perçoit **sans réduire ce qu'il rembourse**, et font donc monter le TAEG. |
| `taux_assurance` | Taux annuel de l'assurance emprunteur, en pourcent. |
| `assiette_assurance` | `capital_initial` — prime constante ; `capital_restant_du` — prime décroissante ; `aucune`. Un taux déclaré sans assiette porte sur le capital initial. |

Sur 250 000 € à 3,45 % sur 240 mois :

| | TAEG | coût du crédit |
|---|---|---|
| Nu | 3,5051 % | 96 436,65 € |
| + 1 500 € de frais de dossier | 3,5752 % | 97 936,65 € |
| + assurance 0,36 % sur capital initial | 4,1020 % | 114 436,65 € |
| + 2 400 € de frais et assurance sur capital restant dû | 3,9926 % | 108 899,58 € |

Chaque ligne de l'échéancier distingue `echeance` (capital + intérêts) de
`mensualite` (échéance + assurance), ce que l'emprunteur verse réellement.

## Le TAEG

Le TAEG rendu est le taux actuariel annuel qui égalise la valeur actuelle des
versements au montant **réellement perçu**, soit le capital diminué des frais.
Il **n'est pas déduit** du taux nominal : dès que des frais ou une assurance
entrent dans les flux, aucune formule fermée ne le donne. Et même sans eux, les
échéances étant arrondies au centime, il faut le résoudre. Le programme
COBOL le fait par dichotomie sur le taux périodique, en arithmétique décimale
exacte, puis capitalise sur douze mois.

Sur le cas de référence, la dichotomie converge vers 3,505078595 %, soit
**3,5051 %** à quatre décimales — valeur recoupée avec une résolution
indépendante en `Decimal` Python sur les mêmes échéances.

En l'absence de frais **et** d'assurance, le TAEG ne dépend que du taux
nominal : il vaut la même chose pour les trois méthodes d'amortissement. La
suite de tests le vérifie comme une propriété du domaine. Dès qu'un frais ou
une prime apparaît, cette propriété tombe — et c'est exactement là que la
résolution gagne son coût d'une douzaine de millisecondes.

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
| `GET` | `/v1/loans/schedule?capital=&taux=&mois=&methode=` | oui | Échéancier de prêt |
| `GET` | `/health` | non | État du service ; `503` si le binaire COBOL manque |

Les routes métier sont versionnées. Le format du récapitulatif a déjà changé
une fois ; un client tiers ne doit pas en pâtir au prochain changement.

La spécification est **embarquée dans le binaire** et servie telle quelle : elle
est versionnée avec le code qu'elle décrit et ne peut pas en diverger. Un test
vérifie que chaque chemin qu'elle décrit répond réellement.

Paramètres : `capital` de 0.01 à 99999999999.99, `taux` nominal annuel en
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
| `COBOL_PROGRAM_PATH` | `/app/bin/loan_amortization` | Binaire COBOL compilé |
| `COMPUTE_TIMEOUT_SECONDS` | `5` | Délai maximal d'un calcul. Au-delà, le processus COBOL est tué et la requête rend `504`. Un échéancier de 600 mois avec TAEG prend une vingtaine de millisecondes. |
| `APP_ENV` | vide | `production` rend `API_KEY` obligatoire |

Le plafond de débit porte sur l'adresse vue par le serveur. Les en-têtes
`X-Forwarded-For` sont volontairement ignorés : ils sont falsifiables tant
qu'aucun proxy de confiance n'est déclaré. Derrière un proxy inverse, il faut
donc en tenir compte avant de se fier au plafond.

## Contrat du programme COBOL

Le binaire est autonome et testable sans la couche Go. Il lit sur son entrée
standard **une ligne de 57 caractères** — capital `9(11)V99`, taux annuel
`9(2)V9(6)`, durée `9(4)`, frais de dossier et de garantie `9(9)V99`, taux
d'assurance `9(2)V9(6)`, puis les lettres de la méthode (`A`, `C`, `I`) et de
l'assiette d'assurance (`N`, `I`, `R`) — et écrit des enregistrements à largeur
fixe :

```
R + échéances 9(4) + première et dernière mensualité 9(11)V99
                   + total intérêts et total assurance 9(13)V99
                   + total frais 9(9)V99
                   + total versé et coût du crédit 9(13)V99
                   + TAEG 9(2)V9(4)                                          110 car.
E + numéro    9(4) + échéance, intérêts, capital, assurance,
                     mensualité, solde 9(11)V99                               83 car.
```

```sh
$ echo "0000025000000034500000240000000000000000000000000000000AN" | ./bin/loan_amortization | head -2
R0240000000014434800000001444930000000096436650000000000000000000000000000000000034643665000000009643665035051
E0001000000014434800000000718750000000072473000000000000000000001443480000024927527
```

Codes de sortie : `2` si l'entrée est malformée, `3` si le capital ou la durée
sont nuls ou la durée supérieure à 600 mois, `4` si la méthode est inconnue.

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
go test ./...
go run ./cmd/cobol-api
```

**Attention** : les tests qui ont besoin du binaire COBOL se sautent d'eux-mêmes
s'il n'a pas été compilé, et `go test` affiche `ok` malgré tout. Sans le
binaire, 18 tests sur 37 ne s'exécutent pas. La CI le compile toujours.

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

## Architecture

```
cobol/loan-amortization.cbl   règles métier et arithmétique exacte
internal/loan/                formatage de la demande, appel du binaire, lecture
internal/api/                 routes, authentification, débit, en-têtes
cmd/cobol-api/                démarrage et arrêt propre
internal/api/openapi.json     le contrat, embarqué dans le binaire
```

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
- Pas d'échéances irrégulières, de différé d'amortissement ni de remboursement anticipé.
- Le taux d'usure n'est pas vérifié : le service ne dit pas si le TAEG rendu
  dépasse le plafond réglementaire de la catégorie de prêt.
