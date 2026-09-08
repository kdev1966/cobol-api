// nodejs/promo-generator.js
//
// Interface avec le programme COBOL. Le binaire reçoit le nombre de codes en
// argument et une ligne de treize caractères par code sur son entrée standard ;
// il répond une ligne « <CODE> - <REMISE> » par code.

const { execFile } = require("child_process");
const crypto = require("crypto");

// Alphabet base32 de Crockford : I, L, O et U sont exclus pour éviter les
// confusions à la lecture et à la saisie d'un code.
const ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
const CODE_LENGTH = 10;

// Treize caractères par code : dix pour le corps, trois chiffres pour le
// palier de remise. FUNCTION RANDOM de GnuCOBOL est amorcée par les secondes
// depuis minuit multipliées par des bits de pointeur de module, puis confiée à
// srandom() — trop faible pour des codes qui valent de l'argent. L'entropie
// vient donc d'ici, où un CSPRNG est disponible.
//
// Dix caractères sur un alphabet de 32 donnent 32^10, soit environ 1,1e15
// combinaisons : l'énumération n'est plus praticable, là où les six chiffres
// d'origine n'en offraient que 900 000.
function buildEntropy(count) {
  const lines = [];

  for (let i = 0; i < count; i++) {
    let body = "";
    for (let c = 0; c < CODE_LENGTH; c++) {
      body += ALPHABET[crypto.randomInt(0, ALPHABET.length)];
    }
    const tierDigits = String(crypto.randomInt(0, 1000)).padStart(3, "0");
    lines.push(body + tierDigits);
  }

  return lines.join("\n") + "\n";
}

// Une ligne bien formée vaut « PROABCDEFGHJK - 20% OFF ».
const PROMO_LINE = /^(\S+) - (.+)$/;

// Renvoie les codes reconnus et les lignes écartées. Un avertissement de libcob
// sur stdout ne doit pas interrompre le traitement.
function parsePromoCodes(stdout) {
  const promoCodes = [];
  const skippedLines = [];

  for (const line of stdout.split("\n")) {
    const trimmed = line.trim();
    if (trimmed === "") continue;

    const match = PROMO_LINE.exec(trimmed);
    if (!match) {
      skippedLines.push(trimmed);
      continue;
    }

    promoCodes.push({ code: match[1], discount: match[2].trim() });
  }

  return { promoCodes, skippedLines };
}

function generatePromoCodes(count, programPath) {
  return new Promise((resolve, reject) => {
    // execFile plutôt que exec : les arguments sont passés en tableau, sans
    // interprétation par un shell.
    const child = execFile(
      programPath,
      [String(count)],
      (error, stdout, stderr) => {
        if (stderr && stderr.trim() !== "") {
          console.error(`❌ Sortie d'erreur COBOL : ${stderr.trim()}`);
        }
        if (error) {
          return reject(error);
        }

        const { promoCodes, skippedLines } = parsePromoCodes(stdout);
        for (const line of skippedLines) {
          console.warn(`⚠️  Ligne COBOL ignorée (format inattendu) : ${line}`);
        }

        resolve(promoCodes);
      }
    );

    child.stdin.on("error", reject);
    child.stdin.end(buildEntropy(count));
  });
}

module.exports = {
  ALPHABET,
  CODE_LENGTH,
  buildEntropy,
  parsePromoCodes,
  generatePromoCodes,
};
