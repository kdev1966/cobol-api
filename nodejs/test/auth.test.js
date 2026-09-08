// nodejs/test/auth.test.js

const test = require("node:test");
const assert = require("node:assert");

const { sameSecret, createApiKeyGuard } = require("../auth");

// Double minimal de la paire requête/réponse d'Express.
function fakeExchange(header) {
  const res = {
    statusCode: null,
    body: null,
    status(code) {
      this.statusCode = code;
      return this;
    },
    json(payload) {
      this.body = payload;
      return this;
    },
  };
  const req = { get: (name) => (name === "X-API-Key" ? header : undefined) };
  return { req, res };
}

function run(guard, header) {
  const { req, res } = fakeExchange(header);
  let passed = false;
  guard(req, res, () => {
    passed = true;
  });
  return { passed, res };
}

test("sameSecret reconnaît deux valeurs identiques", () => {
  assert.ok(sameSecret("secret-partage", "secret-partage"));
});

test("sameSecret rejette des valeurs différentes, y compris de longueurs inégales", () => {
  // Le condensat de longueur fixe permet de comparer sans lever ni divulguer
  // la longueur attendue.
  assert.ok(!sameSecret("court", "une-cle-nettement-plus-longue"));
  assert.ok(!sameSecret("secret-partage", "secret-partagE"));
});

test("la bonne clé passe", () => {
  const { passed, res } = run(createApiKeyGuard("bonne-cle"), "bonne-cle");

  assert.ok(passed);
  assert.strictEqual(res.statusCode, null);
});

test("une mauvaise clé est refusée en 401", () => {
  const { passed, res } = run(createApiKeyGuard("bonne-cle"), "mauvaise-cle");

  assert.ok(!passed);
  assert.strictEqual(res.statusCode, 401);
  assert.strictEqual(res.body.status, "error");
});

test("l'absence d'en-tête est refusée en 401", () => {
  const { passed, res } = run(createApiKeyGuard("bonne-cle"), undefined);

  assert.ok(!passed);
  assert.strictEqual(res.statusCode, 401);
});

test("un préfixe de la bonne clé est refusé", () => {
  const { passed, res } = run(createApiKeyGuard("bonne-cle"), "bonne");

  assert.ok(!passed);
  assert.strictEqual(res.statusCode, 401);
});

test("sans clé configurée, le garde laisse passer", () => {
  // Ce mode n'est tolere qu'en developpement : le serveur refuse de demarrer
  // en production sans API_KEY.
  const { passed, res } = run(createApiKeyGuard(undefined), undefined);

  assert.ok(passed);
  assert.strictEqual(res.statusCode, null);
});
