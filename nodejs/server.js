// nodejs/server.js

const express = require("express");
const helmet = require("helmet");
const cors = require("cors");
const rateLimit = require("express-rate-limit");
const path = require("path");
const fs = require("fs");
const sqlite3 = require("sqlite3").verbose();

const { generatePromoCodes } = require("./promo-generator");
const { createApiKeyGuard } = require("./auth");

const app = express();
const port = process.env.PORT || 3000;

// Le binaire COBOL est compilé à la construction de l'image (cf. Dockerfile),
// plus au démarrage : la production n'a pas besoin d'un compilateur.
const COBOL_PROGRAM_PATH =
  process.env.COBOL_PROGRAM_PATH ||
  path.resolve(__dirname, "..", "bin", "promo_generator");

// La base vit dans le volume monté, pas dans l'arbre applicatif.
const DB_PATH =
  process.env.DB_PATH ||
  path.resolve(__dirname, "..", "database", "promocodes.db");

// L'ancien programme plafonnait le nombre de codes à 99 par accident, via son
// PIC 9(2), et une limite d'historique négative faisait rendre toute la table
// par SQLite. Les deux bornes sont désormais explicites.
const MAX_CODES = 100;
const MAX_HISTORY = 100;

// Ces endroits emettent et listent des bons de reduction : ils exigent une cle.
const API_KEY = process.env.API_KEY;
const RATE_LIMIT_PER_MINUTE = Number(process.env.RATE_LIMIT_PER_MINUTE) || 30;

// Aucune origine autorisee par defaut ; en lister via CORS_ORIGINS, separees
// par des virgules.
const CORS_ORIGINS = (process.env.CORS_ORIGINS || "")
  .split(",")
  .map((o) => o.trim())
  .filter(Boolean);

fs.mkdirSync(path.dirname(DB_PATH), { recursive: true });
const db = new sqlite3.Database(DB_PATH);

// Fonction de création de table si non existante
function initDatabase() {
  return new Promise((resolve, reject) => {
    db.run(
      `CREATE TABLE IF NOT EXISTS promocodes (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      code TEXT NOT NULL UNIQUE,
      discount TEXT NOT NULL,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )`,
      (err) => {
        if (err) reject(err);
        else resolve();
      }
    );
  });
}

// Fonction pour insérer un code promo en base de données
function savePromoCodeToDatabase(code, discount) {
  return new Promise((resolve, reject) => {
    db.run(
      `INSERT OR IGNORE INTO promocodes (code, discount) VALUES (?, ?)`,
      [code, discount],
      function (err) {
        if (err) return reject(err);
        // INSERT OR IGNORE ne lève pas sur doublon : changes vaut alors 0 et
        // lastID garde la valeur de l'insertion précédente. Sans ce test, la
        // réponse annonçait l'identifiant d'une ligne déjà présente.
        resolve(this.changes > 0 ? this.lastID : null);
      }
    );
  });
}

// Fonction pour récupérer les codes promos de la base
function getPromoCodesFromDatabase(limit) {
  return new Promise((resolve, reject) => {
    // Tri sur id plutôt que created_at : CURRENT_TIMESTAMP a une granularité
    // d'une seconde, insuffisante pour ordonner des codes créés d'un même
    // appel.
    db.all(
      `SELECT * FROM promocodes ORDER BY id DESC LIMIT ?`,
      [limit],
      (err, rows) => {
        if (err) reject(err);
        else resolve(rows);
      }
    );
  });
}

// Renvoie null si la valeur n'est pas un entier dans [1, max].
function parseBoundedInt(raw, fallback, max) {
  if (raw === undefined) return fallback;

  const value = Number(raw);
  if (!Number.isInteger(value) || value < 1 || value > max) return null;

  return value;
}

// Le détail de l'erreur reste dans les journaux : il porte des chemins absolus
// et la sortie d'erreur de cobc, que le client n'a pas à connaître.
function sendServerError(res, context, error) {
  console.error(`${context} :`, error);
  res.status(500).json({ status: "error", message: "Erreur interne" });
}

const requireApiKey = createApiKeyGuard(API_KEY);

const limiter = rateLimit({
  windowMs: 60 * 1000,
  limit: RATE_LIMIT_PER_MINUTE,
  standardHeaders: "draft-7",
  legacyHeaders: false,
  message: { status: "error", message: "Trop de requetes" },
});

app.disable("x-powered-by");
app.use(helmet());
app.use(cors({ origin: CORS_ORIGINS.length > 0 ? CORS_ORIGINS : false }));

// Middleware de log
app.use((req, res, next) => {
  console.log(`${new Date().toISOString()} - ${req.method} ${req.path}`);
  next();
});

// Route pour générer des codes promotionnels
app.get("/promocodes", limiter, requireApiKey, async (req, res) => {
  const count = parseBoundedInt(req.query.count, 5, MAX_CODES);

  if (count === null) {
    return res.status(400).json({
      status: "error",
      message: `Le paramètre count doit être un entier entre 1 et ${MAX_CODES}`,
    });
  }

  try {
    console.log(`Génération de ${count} codes promotionnels...`);

    const promoCodes = await generatePromoCodes(count, COBOL_PROGRAM_PATH);

    const savedIds = (
      await Promise.all(
        promoCodes.map((pc) => savePromoCodeToDatabase(pc.code, pc.discount))
      )
    ).filter((id) => id !== null);

    res.json({
      status: "success",
      count: promoCodes.length,
      promocodes: promoCodes,
      saved: savedIds.length,
      savedIds,
    });
  } catch (error) {
    sendServerError(res, "Erreur lors de la génération des codes", error);
  }
});

// Route pour récupérer les codes promos existants
app.get("/history", limiter, requireApiKey, async (req, res) => {
  const limit = parseBoundedInt(req.query.limit, 10, MAX_HISTORY);

  if (limit === null) {
    return res.status(400).json({
      status: "error",
      message: `Le paramètre limit doit être un entier entre 1 et ${MAX_HISTORY}`,
    });
  }

  try {
    const historyCodes = await getPromoCodesFromDatabase(limit);

    res.json({
      status: "success",
      count: historyCodes.length,
      history: historyCodes,
    });
  } catch (error) {
    sendServerError(res, "Erreur lors de la récupération de l'historique", error);
  }
});

// Route de health check. Elle interroge réellement la base : le HEALTHCHECK du
// conteneur s'appuie dessus, une réponse inconditionnelle ne servirait à rien.
app.get("/health", (req, res) => {
  const programExists = fs.existsSync(COBOL_PROGRAM_PATH);

  db.get("SELECT 1", (err) => {
    const healthy = programExists && !err;

    if (err) console.error("Health check, base injoignable :", err);

    res.status(healthy ? 200 : 503).json({
      status: healthy ? "OK" : "ERROR",
      timestamp: new Date().toISOString(),
      programExists,
      databaseReachable: !err,
    });
  });
});

app.use((req, res) => {
  res.status(404).json({ status: "error", message: "Ressource inconnue" });
});

// Le serveur ne démarre pas si le binaire ou la base sont indisponibles :
// mieux vaut un conteneur qui refuse de se lever qu'un service qui répond
// 500 sur chaque requête.
async function initializeServer() {
  if (!API_KEY) {
    if (process.env.NODE_ENV === "production") {
      throw new Error(
        "API_KEY est obligatoire en production : /promocodes et /history " +
          "emettent et listent des bons de reduction."
      );
    }
    console.warn(
      "⚠️  API_KEY absente : /promocodes et /history sont ouverts. " +
        "Ne pas exploiter ainsi hors developpement."
    );
  }

  if (!fs.existsSync(COBOL_PROGRAM_PATH)) {
    throw new Error(
      `Binaire COBOL introuvable : ${COBOL_PROGRAM_PATH}\n` +
        `Le compiler avec : cobc -x -free cobol/promo-code-generator.cbl ` +
        `-o ${COBOL_PROGRAM_PATH}`
    );
  }

  console.log("Initialisation de la base de données...");
  await initDatabase();
}

let server;

initializeServer()
  .then(() => {
    server = app.listen(port, () => {
      console.log(`🚀 Serveur démarré sur le port ${port}`);
    });
  })
  .catch((error) => {
    console.error("Erreur d'initialisation :", error.message);
    process.exit(1);
  });

// Arrêt propre. Docker envoie SIGTERM : sans ce gestionnaire, le conteneur
// attendait dix secondes puis était tué, sans fermer la base.
function shutdown(signal) {
  console.log(`${signal} reçu, arrêt en cours...`);

  const closeDatabase = () =>
    db.close((err) => {
      if (err) console.error(err.message);
      else console.log("Base de données SQLite fermée");
      process.exit(0);
    });

  if (server) server.close(closeDatabase);
  else closeDatabase();
}

process.on("SIGINT", () => shutdown("SIGINT"));
process.on("SIGTERM", () => shutdown("SIGTERM"));
