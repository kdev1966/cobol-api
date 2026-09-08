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
  'http://localhost:3000/loans/schedule?capital=250000.00&taux=3.45&mois=240' | jq
```

```json
{
  "status": "success",
  "demande": {
    "capital": "250000.00", "taux": "3.450000",
    "mois": 240, "methode": "annuite_constante"
  },
  "recapitulatif": {
    "echeances": 240,
    "premiere_echeance": 1443.48,
    "derniere_echeance": 1444.93,
    "total_interets": 96436.65,
    "total_du": 346436.65,
    "taeg": 3.5051
  },
  "echeancier": [
    { "n": 1, "paiement": 1443.48, "interets": 718.75, "capital": 724.73, "solde": 249275.27 },
    { "n": 240, "paiement": 1444.93, "interets": 4.14, "capital": 1440.79, "solde": 0.00 }
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

## Le TAEG

Le TAEG rendu est le taux actuariel annuel qui égalise la valeur actuelle des
échéances au capital emprunté. Il **n'est pas déduit** du taux nominal : comme
les échéances sont arrondies au centime, il faut le résoudre. Le programme
COBOL le fait par dichotomie sur le taux périodique, en arithmétique décimale
exacte, puis capitalise sur douze mois.

Sur le cas de référence, la dichotomie converge vers 3,505078595 %, soit
**3,5051 %** à quatre décimales — valeur recoupée avec une résolution
indépendante en `Decimal` Python sur les mêmes échéances.

En l'absence de frais, le TAEG ne dépend que du taux nominal : il vaut la même
chose pour les trois méthodes d'amortissement. La suite de tests le vérifie
comme une propriété du domaine.

Ce calcul a un coût : sur un échéancier de 240 mois, la requête passe de
7,3 ms à **19,5 ms**. C'est le prix d'une résolution honnête, qui restera juste
le jour où des frais de dossier ou une assurance entreront dans les flux — là
où une formule fermée deviendrait fausse.

## Les invariants

Le moteur tient quatre garanties, vérifiées par la suite de tests sur neuf
jeux de paramètres allant du taux nul à 600 mensualités :

1. la somme des parts de capital égale **exactement** le capital emprunté ;
2. la somme des échéances égale **exactement** capital + intérêts ;
3. le solde après la dernière échéance est **exactement** nul ;
4. sur chaque ligne, paiement = capital + intérêts.

Les montants ne transitent jamais par un flottant : le COBOL écrit des
chiffres, Go y insère le point décimal et les transporte en `json.Number`
jusqu'à la réponse.

## Endpoints

| Méthode | Chemin | Clé requise | Description |
|---|---|---|---|
| `GET` | `/` | non | Index du service |
| `GET` | `/loans/schedule?capital=&taux=&mois=&methode=` | oui | Échéancier de prêt |
| `GET` | `/health` | non | État du service ; `503` si le binaire COBOL manque |

Paramètres : `capital` de 0.01 à 99999999999.99, `taux` nominal annuel en
pourcent de 0 à 99.999999, `mois` de 1 à 600, `methode` parmi les trois
libellés ci-dessus — les lettres `A`, `C` et `I` sont acceptées comme alias.
Un paramètre invalide rend un
`400` nommant le champ fautif ; une clé absente ou invalide un `401` ; un
dépassement du plafond de requêtes un `429`.

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
| `APP_ENV` | vide | `production` rend `API_KEY` obligatoire |

Le plafond de débit porte sur l'adresse vue par le serveur. Les en-têtes
`X-Forwarded-For` sont volontairement ignorés : ils sont falsifiables tant
qu'aucun proxy de confiance n'est déclaré. Derrière un proxy inverse, il faut
donc en tenir compte avant de se fier au plafond.

## Contrat du programme COBOL

Le binaire est autonome et testable sans la couche Go. Il lit sur son entrée
standard **une ligne de 26 caractères** — capital `9(11)V99`, taux annuel
`9(2)V9(6)`, durée `9(4)`, puis la lettre de la méthode (`A`, `C` ou `I`) — et
écrit des enregistrements à largeur fixe :

```
R + échéances 9(4) + première et dernière échéance 9(11)V99
                   + total intérêts et total dû 9(13)V99
                   + TAEG 9(2)V9(4)                                           67 car.
E + numéro    9(4) + paiement, intérêts, capital, solde 9(11)V99              57 car.
```

```sh
$ echo "0000025000000034500000240A" | ./bin/loan_amortization | head -2
R024000000001443480000000144493000000009643665000000034643665035051
E00010000000144348000000007187500000000724730000024927527
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
```

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
- Ni assurance, ni frais de dossier, ni échéances irrégulières.
