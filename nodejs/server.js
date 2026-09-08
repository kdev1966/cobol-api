// nodejs/server.js

const express = require("express");
const { execFile } = require("child_process");
const crypto = require("crypto");
const path = require("path");
const fs = require("fs");
const sqlite3 = require("sqlite3").verbose();

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

// Le nombre de codes bornait implicitement le PIC 9(2) de l'ancien programme.
// L'entropie étant désormais construite en amont, la borne devient explicite.
const MAX_CODES = 100;

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

// Neuf chiffres par code : six pour le numéro, trois pour le palier de remise.
// FUNCTION RANDOM de GnuCOBOL n'est ni amorcée ni cryptographique — elle
// rejouait la même séquence à chaque exécution du binaire. L'entropie vient
// donc d'ici, où un CSPRNG est disponible.
function buildEntropy(count) {
  const lines = [];
  for (let i = 0; i < count; i++) {
    const codeDigits = String(crypto.randomInt(0, 1000000)).padStart(6, "0");
    const tierDigits = String(crypto.randomInt(0, 1000)).padStart(3, "0");
    lines.push(codeDigits + tierDigits);
  }
  return lines.join("\n") + "\n";
}

// Une ligne bien formée vaut "PRO123456 - 20% OFF".
const PROMO_LINE = /^(\S+) - (.+)$/;

function parsePromoCodes(stdout) {
  const promoCodes = [];

  for (const line of stdout.split("\n")) {
    const trimmed = line.trim();
    if (trimmed === "") continue;

    const match = PROMO_LINE.exec(trimmed);
    if (!match) {
      // Un avertissement du compilateur ou du runtime sur stdout ne doit pas
      // faire tomber le process.
      console.warn(`⚠️  Ligne COBOL ignorée (format inattendu) : ${trimmed}`);
      continue;
    }

    promoCodes.push({ code: match[1], discount: match[2].trim() });
  }

  return promoCodes;
}

// Fonction pour générer des codes promotionnels
function generatePromoCodes(count = 5) {
  return new Promise((resolve, reject) => {
    // execFile plutôt que exec : les arguments sont passés en tableau, sans
    // interprétation par un shell.
    const child = execFile(
      COBOL_PROGRAM_PATH,
      [String(count)],
      (error, stdout, stderr) => {
        if (stderr && stderr.trim() !== "") {
          console.error(`❌ Sortie d'erreur COBOL : ${stderr.trim()}`);
        }
        if (error) {
          return reject(error);
        }
        resolve(parsePromoCodes(stdout));
      }
    );

    child.stdin.on("error", reject);
    child.stdin.end(buildEntropy(count));
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

// Middleware de log
app.use((req, res, next) => {
  console.log(`${new Date().toISOString()} - ${req.method} ${req.path}`);
  next();
});

// Route pour générer des codes promotionnels
app.get("/promocodes", async (req, res) => {
  const rawCount = req.query.count;
  const count = rawCount === undefined ? 5 : Number(rawCount);

  if (!Number.isInteger(count) || count < 1 || count > MAX_CODES) {
    return res.status(400).json({
      status: "error",
      message: `Le paramètre count doit être un entier entre 1 et ${MAX_CODES}`,
    });
  }

  try {
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
    programExists: fs.existsSync(COBOL_PROGRAM_PATH),
  });
});

// Le serveur ne démarre pas si le binaire ou la base sont indisponibles :
// mieux vaut un conteneur qui refuse de se lever qu'un service qui répond
// 500 sur chaque requête.
async function initializeServer() {
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

initializeServer()
  .then(() => {
    app.listen(port, () => {
      console.log(`🚀 Serveur démarré sur le port ${port}`);
    });
  })
  .catch((error) => {
    console.error("Erreur d'initialisation :", error.message);
    process.exit(1);
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
