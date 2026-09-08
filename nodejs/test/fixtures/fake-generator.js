#!/usr/bin/env node
// Double du programme COBOL, même contrat : le nombre de codes en argument,
// une ligne de treize caractères par code sur stdin (dix de corps, trois
// chiffres de palier).
//
// Variables d'environnement de test :
//   FAKE_NOISE=1  écrit une ligne parasite avant et après les codes
//   FAKE_FAIL=1   échoue avec un message sur stderr

const DISCOUNTS = ["10% OFF", "20% OFF", "30% OFF"];
const count = Number(process.argv[2]);

let input = "";
process.stdin.on("data", (chunk) => (input += chunk));
process.stdin.on("end", () => {
  if (process.env.FAKE_FAIL === "1") {
    process.stderr.write("entropie invalide a la ligne 0001\n");
    process.exit(3);
  }

  const lines = input.split("\n").filter((l) => l.trim() !== "");
  const out = [];

  if (process.env.FAKE_NOISE === "1") {
    out.push("libcob: warning: implicit CLOSE of SYSIN");
  }

  for (let i = 0; i < count && i < lines.length; i++) {
    const line = lines[i].trim();
    const tier = Number(line.slice(10, 13)) % 3;
    out.push(`PRO${line.slice(0, 10)} - ${DISCOUNTS[tier]}`);
  }

  if (process.env.FAKE_NOISE === "1") {
    out.push("note de fin sans separateur");
  }

  process.stdout.write(out.join("\n") + "\n");
});
