// nodejs/auth.js

const crypto = require("crypto");

// Comparaison à temps constant. Les condensats ont une longueur fixe, ce qui
// évite de divulguer celle de la clé attendue.
function sameSecret(a, b) {
  const digest = (v) => crypto.createHash("sha256").update(v).digest();
  return crypto.timingSafeEqual(digest(a), digest(b));
}

// Rend un middleware qui exige l'en-tête X-API-Key. Sans clé configurée, il
// laisse passer : le serveur refuse alors de démarrer en production.
function createApiKeyGuard(apiKey) {
  return function requireApiKey(req, res, next) {
    if (!apiKey) return next();

    const provided = req.get("X-API-Key");
    if (provided && sameSecret(provided, apiKey)) return next();

    res.status(401).json({
      status: "error",
      message: "Clé d'API absente ou invalide",
    });
  };
}

module.exports = { sameSecret, createApiKeyGuard };
