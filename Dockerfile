#  Dockerfile

FROM ubuntu:22.04

# Éviter les interactions pendant l'installation
ENV DEBIAN_FRONTEND=noninteractive

# Mettre à jour les paquets et installer les dépendances
RUN apt-get update && \
    apt-get install -y \
    build-essential \
    gnucobol \
    nodejs \
    npm \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Définir le répertoire de travail
WORKDIR /app

# Copier tous les fichiers
COPY . .

# Script de débogage et compilation
COPY debug_cobol.sh /app/debug_cobol.sh
RUN chmod +x /app/debug_cobol.sh

# Exécuter le script de débogage
RUN /app/debug_cobol.sh

# Copier package.json et installer les dépendances Node.js
COPY nodejs/package*.json ./nodejs/
RUN cd nodejs && npm install

# Créer un volume pour la base de données
VOLUME /app/database

# Exposer le port
EXPOSE 3000

# Commande de démarrage
CMD ["node", "nodejs/server.js"]