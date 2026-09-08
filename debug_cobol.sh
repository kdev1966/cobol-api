#!/bin/bash

# Script de débogage pour la compilation COBOL

# Afficher le répertoire de travail
echo "Répertoire de travail actuel : $(pwd)"

# Lister tous les fichiers
echo "Contenu du répertoire racine :"
ls -la

# Vérifier le répertoire COBOL
echo "Contenu du répertoire COBOL :"
ls -la cobol/

# Compiler le programme COBOL avec des informations détaillées
echo "Compilation du programme COBOL :"
cd cobol
cobc -x -free hello.cbl -o hello_cobol
COMPILE_STATUS=$?

# Vérifier le statut de la compilation
if [ $COMPILE_STATUS -eq 0 ]; then
    echo "✅ Compilation COBOL réussie"
    ls -l hello_cobol
else
    echo "❌ Échec de la compilation COBOL"
    cobc -x -free hello.cbl
fi

# Retourner au répertoire racine
cd ..