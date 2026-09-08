#  Dockerfile

# --- Construction : compilateurs COBOL et C, chaîne de build Node ------------
FROM ubuntu:24.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

# GnuCOBOL traduit en C : un compilateur C est nécessaire ici. Node.js vient de
# NodeSource car le paquet « nodejs » de la distribution est trop ancien.
# Ubuntu 24.04 plutôt que 22.04 : le binaire précompilé de sqlite3 exige
# GLIBC 2.38, absent de 22.04 qui s'arrête à 2.35.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    gnucobol \
    gnupg \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Compilation du programme COBOL, suivie d'un test de fumée : le build échoue
# si le binaire ne rend pas le format attendu.
COPY cobol/ ./cobol/
RUN mkdir -p /app/bin && \
    cobc -x -free cobol/promo-code-generator.cbl -o /app/bin/promo_generator && \
    echo "ABCDEFGHJK042" | /app/bin/promo_generator 1 | grep -q " - "

# Dépendances Node avant les sources : la couche reste en cache tant que les
# manifestes ne bougent pas. npm ci installe exactement le lockfile.
COPY nodejs/package.json nodejs/package-lock.json ./nodejs/
RUN cd nodejs && npm ci --omit=dev

COPY nodejs/ ./nodejs/

# La suite de tests tourne à la construction : une régression bloque l'image.
RUN cd nodejs && npm test

# --- Exécution : ni compilateur C, ni compilateur COBOL ----------------------
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# libcob4t64 seul suffit à exécuter le binaire ; curl sert au HEALTHCHECK.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    libcob4t64 \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/bin/ ./bin/
COPY --from=builder /app/nodejs/ ./nodejs/

# Utilisateur non privilégié, propriétaire du seul répertoire écrit.
RUN useradd --system --create-home --shell /usr/sbin/nologin cobolapi && \
    mkdir -p /app/database && \
    chown cobolapi:cobolapi /app/database

ENV COBOL_PROGRAM_PATH=/app/bin/promo_generator \
    DB_PATH=/app/database/promocodes.db \
    NODE_ENV=production \
    PORT=3000

# Créer un volume pour la base de données
VOLUME /app/database

# Exposer le port
EXPOSE 3000

USER cobolapi

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://localhost:3000/health || exit 1

# Commande de démarrage
CMD ["node", "nodejs/server.js"]
