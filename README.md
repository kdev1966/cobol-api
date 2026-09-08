# cobol-api

API REST qui expose un générateur de codes promotionnels écrit en COBOL.

Le programme COBOL porte les règles métier — forme du code et répartition des
paliers de remise — pendant que la couche Node fournit l'entropie
cryptographique, la persistance et le contrôle d'accès. Cette séparation est
délibérée : `FUNCTION RANDOM` de GnuCOBOL est amorcée par les secondes depuis
minuit multipliées par des bits de pointeur de module, puis confiée à
`srandom()`, ce qui est trop faible pour des bons de réduction.

## Démarrage

```sh
cp .env.example .env
# renseigner API_KEY, par exemple avec : openssl rand -hex 32
docker compose up -d --build
```

```sh
curl -H "X-API-Key: $API_KEY" 'http://localhost:3000/promocodes?count=5'
```

```json
{
  "status": "success",
  "count": 5,
  "promocodes": [{ "code": "PROSESZ9K17VZ", "discount": "20% OFF" }],
  "saved": 5,
  "savedIds": [1, 2, 3, 4, 5]
}
```

## Endpoints

| Méthode | Chemin | Clé requise | Description |
|---|---|---|---|
| `GET` | `/` | non | Index du service et liste des endpoints |
| `GET` | `/promocodes?count=N` | oui | Génère N codes (1-100, défaut 5) et les enregistre |
| `GET` | `/history?limit=N` | oui | Rend les N derniers codes (1-100, défaut 10) |
| `GET` | `/health` | non | État du service ; `503` si la base ou le binaire manquent |

La clé passe dans l'en-tête `X-API-Key`. `/health` reste ouvert parce que le
`HEALTHCHECK` du conteneur s'appuie dessus.

Un paramètre hors bornes est refusé en `400`, une clé absente ou invalide en
`401`, un dépassement du plafond de requêtes en `429`.

## Configuration

| Variable | Défaut | Rôle |
|---|---|---|
| `API_KEY` | — | Clé attendue sur les endpoints protégés. **Obligatoire quand `NODE_ENV=production`** : sans elle, le serveur refuse de démarrer. Absente hors production, le service tourne ouvert avec un avertissement. |
| `RATE_LIMIT_PER_MINUTE` | `30` | Requêtes par minute et par adresse IP |
| `CORS_ORIGINS` | vide | Origines navigateur autorisées, séparées par des virgules. Vide = aucune origine croisée. |
| `PORT` | `3000` | Port d'écoute |
| `DB_PATH` | `../database/promocodes.db` | Fichier SQLite |
| `COBOL_PROGRAM_PATH` | `../bin/promo_generator` | Binaire COBOL compilé |

Le service tourne derrière la limitation par IP d'`express-rate-limit`. Derrière
un proxy inverse, configurer `trust proxy` pour que le plafond porte sur
l'adresse cliente réelle et non sur celle du proxy.

## Format des codes

`PRO` suivi de dix caractères tirés d'un alphabet base32 de Crockford
(`0-9`, `A-Z` sans `I`, `L`, `O` ni `U`, écartés pour éviter les confusions à la
saisie). Soit environ 1,1 × 10¹⁵ combinaisons, contre 900 000 pour le format à
six chiffres d'origine, qui était énumérable.

Les remises sont réparties sur trois paliers : `10% OFF`, `20% OFF`, `30% OFF`.

## Contrat du programme COBOL

Le binaire est autonome et testable sans la couche Node :

```
promo_generator <nombre-de-codes>
```

Il lit sur son entrée standard une ligne de treize caractères par code — dix de
corps, puis trois chiffres désignant le palier — et écrit une ligne
`<CODE> - <REMISE>` par code.

```sh
$ printf 'ABCDEFGHJK000\n0123456789001\n' | ./bin/promo_generator 2
PROABCDEFGHJK - 10% OFF
PRO0123456789 - 20% OFF
```

Codes de sortie : `2` si le nombre de codes est absent ou invalide, `3` si une
ligne d'entropie est malformée.

## Développement

Sans Docker, il faut GnuCOBOL (`cobc`) et Node 20.17 ou plus.

```sh
mkdir -p bin
cobc -x -free cobol/promo-code-generator.cbl -o bin/promo_generator
cd nodejs && npm install && npm start
```

Sans `API_KEY`, le serveur démarre en laissant les endpoints ouverts et le
signale par un avertissement.

La suite de tests ne nécessite pas de compilateur COBOL : elle s'appuie sur un
double qui respecte le même contrat `argv`/`stdin`.

```sh
cd nodejs && npm test
```

Ces tests tournent aussi pendant la construction de l'image, de même qu'un test
de fumée sur le binaire COBOL : une régression bloque le build.

## Architecture

```
cobol/promo-code-generator.cbl   règles métier : forme du code, paliers
nodejs/promo-generator.js        entropie cryptographique, appel du binaire
nodejs/auth.js                   contrôle de la clé d'API
nodejs/server.js                 routes HTTP, SQLite, arrêt propre
```

L'image est construite en deux étapes : le compilateur COBOL et la chaîne de
build C ne sont présents qu'à la construction. L'image d'exécution ne contient
ni `cobc` ni `gcc`, seulement la bibliothèque `libcob`, et tourne sous un
utilisateur non privilégié.

La base vit dans un volume Docker nommé plutôt que dans un montage lié : l'uid
du conteneur ne correspond à aucun propriétaire de répertoire de l'hôte.
Attention, `docker compose down -v` la supprime.
