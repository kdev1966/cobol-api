// nodejs/test/promo-generator.test.js

const test = require("node:test");
const assert = require("node:assert");
const path = require("node:path");

const {
  buildEntropy,
  parsePromoCodes,
  generatePromoCodes,
} = require("../promo-generator");

const FAKE = path.resolve(__dirname, "fixtures", "fake-generator.js");

test("buildEntropy produit une ligne de neuf chiffres par code", () => {
  const lines = buildEntropy(7).split("\n").filter((l) => l !== "");

  assert.strictEqual(lines.length, 7);
  for (const line of lines) {
    assert.match(line, /^\d{9}$/);
  }
});

test("buildEntropy ne rejoue pas la même séquence", () => {
  // Le défaut central de l'ancien programme était une séquence rejouable.
  const a = buildEntropy(20);
  const b = buildEntropy(20);

  assert.notStrictEqual(a, b);
});

test("parsePromoCodes lit les lignes bien formées", () => {
  const { promoCodes, skippedLines } = parsePromoCodes(
    "PRO123456 - 10% OFF\nPRO999999 - 30% OFF\n"
  );

  assert.deepStrictEqual(promoCodes, [
    { code: "PRO123456", discount: "10% OFF" },
    { code: "PRO999999", discount: "30% OFF" },
  ]);
  assert.deepStrictEqual(skippedLines, []);
});

test("parsePromoCodes ignore les lignes vides et l'espace de remplissage", () => {
  // L'ancien programme écrivait la remise dans un PIC X(15), donc suivie
  // d'espaces.
  const { promoCodes } = parsePromoCodes(
    "\n  PRO123456 - 10% OFF        \n\n"
  );

  assert.deepStrictEqual(promoCodes, [
    { code: "PRO123456", discount: "10% OFF" },
  ]);
});

test("parsePromoCodes écarte les lignes parasites sans lever", () => {
  // Le code d'origine faisait un .trim() sur undefined ici, ce qui tuait le
  // process depuis le callback de exec plutôt que de produire une 500.
  const { promoCodes, skippedLines } = parsePromoCodes(
    "libcob: warning: implicit CLOSE\nPRO123456 - 10% OFF\nligne sans separateur\n"
  );

  assert.deepStrictEqual(promoCodes, [
    { code: "PRO123456", discount: "10% OFF" },
  ]);
  assert.deepStrictEqual(skippedLines, [
    "libcob: warning: implicit CLOSE",
    "ligne sans separateur",
  ]);
});

test("generatePromoCodes rend autant de codes que demandé", async () => {
  const codes = await generatePromoCodes(12, FAKE);

  assert.strictEqual(codes.length, 12);
  for (const { code, discount } of codes) {
    assert.match(code, /^PRO\d{6}$/);
    assert.ok(["10% OFF", "20% OFF", "30% OFF"].includes(discount));
  }
});

test("generatePromoCodes couvre les trois paliers", async () => {
  // La remise dérivait de FUNCTION CURRENT-DATE lue une seule fois avant la
  // boucle : les codes d'une même réponse portaient tous le même palier.
  const codes = await generatePromoCodes(60, FAKE);
  const tiers = new Set(codes.map((c) => c.discount));

  assert.strictEqual(tiers.size, 3);
});

test("generatePromoCodes rend des codes distincts d'un appel à l'autre", async () => {
  const a = await generatePromoCodes(20, FAKE);
  const b = await generatePromoCodes(20, FAKE);
  const communs = a.filter((x) => b.some((y) => y.code === x.code));

  assert.deepStrictEqual(communs, []);
});

test("generatePromoCodes survit à une sortie polluée", async () => {
  process.env.FAKE_NOISE = "1";
  try {
    const codes = await generatePromoCodes(3, FAKE);
    assert.strictEqual(codes.length, 3);
  } finally {
    delete process.env.FAKE_NOISE;
  }
});

test("generatePromoCodes rejette quand le programme échoue", async () => {
  process.env.FAKE_FAIL = "1";
  try {
    await assert.rejects(() => generatePromoCodes(3, FAKE));
  } finally {
    delete process.env.FAKE_FAIL;
  }
});

test("generatePromoCodes rejette quand le programme est introuvable", async () => {
  await assert.rejects(() => generatePromoCodes(3, "/inexistant/promo_generator"));
});
