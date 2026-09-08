#  Dockerfile

FROM ubuntu:22.04

# Éviter les interactions pendant l'installation
ENV DEBIAN_FRONTEND=noninteractive

# GnuCOBOL traduit en C : un compilateur C est nécessaire à la construction.
# Node.js vient de NodeSource car le paquet « nodejs » d'Ubuntu 22.04 est
# Node 12, en fin de vie et trop ancien pour crypto.randomInt().
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

# Définir le répertoire de travail
WORKDIR /app

# Compilation du programme COBOL, une seule fois : le runtime n'a plus besoin
# de compiler au démarrage ni d'un accès en écriture aux sources.
COPY cobol/ ./cobol/
RUN mkdir -p /app/bin && \
    cobc -x -free cobol/promo-code-generator.cbl -o /app/bin/promo_generator && \
    echo "123456789" | /app/bin/promo_generator 1 | grep -q " - "

# Dépendances Node avant les sources : la couche reste en cache tant que les
# manifestes ne bougent pas. npm ci installe exactement le lockfile.
COPY nodejs/package.json nodejs/package-lock.json ./nodejs/
RUN cd nodejs && npm ci --omit=dev

# Copier le code applicatif
COPY nodejs/ ./nodejs/

ENV COBOL_PROGRAM_PATH=/app/bin/promo_generator \
    DB_PATH=/app/database/promocodes.db \
    NODE_ENV=production \
    PORT=3000

# Créer un volume pour la base de données
VOLUME /app/database

# Exposer le port
EXPOSE 3000

# Commande de démarrage
CMD ["node", "nodejs/server.js"]
