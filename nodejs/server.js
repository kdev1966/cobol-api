// nodejs/server.js

const express = require("express");
const { exec } = require("child_process");
const path = require("path");
const fs = require("fs");
const sqlite3 = require("sqlite3").verbose();

const app = express();
const port = process.env.PORT || 3000;

// Chemins absolus
const COBOL_SOURCE_PATH = path.resolve(
  __dirname,
  "..",
  "cobol",
  "promo-code-generator.cbl"
);
const COBOL_PROGRAM_PATH = path.resolve(
  __dirname,
  "..",
  "cobol",
  "promo_generator"
);

// Initialisation de la base de données SQLite
const DB_PATH = path.resolve(__dirname, "..", "promocodes.db");
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

// Fonction de compilation COBOL
function compileCOBOLProgram() {
  return new Promise((resolve, reject) => {
    exec(
      `cobc -x -free ${COBOL_SOURCE_PATH} -o ${COBOL_PROGRAM_PATH}`,
      (error, stdout, stderr) => {
        if (error) {
          console.error(`❌ Erreur de compilation : `, error);
          return reject(error);
        }
        resolve(stdout);
      }
    );
  });
}

// Fonction pour générer des codes promotionnels
function generatePromoCodes(count = 5) {
  return new Promise((resolve, reject) => {
    exec(`${COBOL_PROGRAM_PATH} ${count}`, (error, stdout, stderr) => {
      if (error) {
        console.error(`❌ Erreur d'exécution : `, error);
        return reject(error);
      }

      // Traiter la sortie
      const promoCodes = stdout
        .trim()
        .split("\n")
        .filter((line) => line.trim() !== "") // Éliminer les lignes vides
        .map((line) => {
          const [code, discount] = line.split(" - ");
          return { code, discount: discount.trim() };
        });

      resolve(promoCodes);
    });
  });
}

// Fonction pour insérer un code promo en base de données
function savePromoCodeToDatabase(code, discount) {
  return new Promise((resolve, reject) => {
    db.run(
      `INSERT OR IGNORE INTO promocodes (code, discount) VALUES (?, ?)`,
      [code, discount],
      function (err) {
        if (err) reject(err);
        else resolve(this.lastID);
      }
    );
  });
}

// Fonction pour récupérer les codes promos de la base
function getPromoCodesFromDatabase(limit = 10) {
  return new Promise((resolve, reject) => {
    db.all(
      `SELECT * FROM promocodes ORDER BY created_at DESC LIMIT ?`,
      [limit],
      (err, rows) => {
        if (err) reject(err);
        else resolve(rows);
      }
    );
  });
}

// Compilation et initialisation au démarrage
async function initializeServer() {
  try {
    console.log("Compilation du programme COBOL...");
    await compileCOBOLProgram();
    console.log("Initialisation de la base de données...");
    await initDatabase();
  } catch (error) {
    console.error("Erreur d'initialisation :", error);
  }
}

// Middleware de log
app.use((req, res, next) => {
  console.log(`${new Date().toISOString()} - ${req.method} ${req.path}`);
  next();
});

// Route pour générer des codes promotionnels
app.get("/promocodes", async (req, res) => {
  try {
    const count = parseInt(req.query.count) || 5;
    console.log(`Génération de ${count} codes promotionnels...`);

    const promoCodes = await generatePromoCodes(count);

    // Sauvegarde en base de données avec gestion des doublons
    const savedCodes = await Promise.all(
      promoCodes.map(async (pc) => {
        try {
          return await savePromoCodeToDatabase(pc.code, pc.discount);
        } catch (error) {
          console.warn(`Code ${pc.code} existe déjà`);
          return null;
        }
      })
    );

    res.json({
      status: "success",
      count: promoCodes.length,
      promocodes: promoCodes,
      savedIds: savedCodes.filter((id) => id !== null),
    });
  } catch (error) {
    console.error("Erreur lors de la génération des codes :", error);
    res.status(500).send(`Erreur de traitement : ${error.message}`);
  }
});

// Route pour récupérer les codes promos existants
app.get("/history", async (req, res) => {
  try {
    const limit = parseInt(req.query.limit) || 10;
    const historyCodes = await getPromoCodesFromDatabase(limit);

    res.json({
      status: "success",
      count: historyCodes.length,
      history: historyCodes,
    });
  } catch (error) {
    console.error("Erreur lors de la récupération de l'historique :", error);
    res.status(500).send(`Erreur de récupération : ${error.message}`);
  }
});

// Route de health check
app.get("/health", (req, res) => {
  res.json({
    status: "OK",
    timestamp: new Date().toISOString(),
    sourceExists: fs.existsSync(COBOL_SOURCE_PATH),
    programExists: fs.existsSync(COBOL_PROGRAM_PATH),
  });
});

// Initialisation et démarrage du serveur
initializeServer().then(() => {
  app.listen(port, () => {
    console.log(`🚀 Serveur démarré sur le port ${port}`);
  });
});

// Gestion de la fermeture propre de la base de données
process.on("SIGINT", () => {
  db.close((err) => {
    if (err) {
      console.error(err.message);
    }
    console.log("Base de données SQLite fermée");
    process.exit(0);
  });
});
