#!/bin/bash

# Définir les tailles, les nombres de threads et les tours
sizes=(16 64 128 256 512)     # Tailles d'image (ajouté 32x32)
threads=(1 2 4 8 16 32)          # Nombre de threads élargi (ajouté 1, 16 et 32)
turns=(1000 500 100 10 1)        # Nombre de tours avec des valeurs plus variées

# Ouvrir le fichier de log pour écrire
logfile="result.log"
> $logfile  # Vider le fichier avant d'écrire

# Boucles pour parcourir toutes les combinaisons possibles
for size in "${sizes[@]}"; do
  echo "Taille: $size" >> $logfile  # Ecrire la taille dans le fichier de log
  for thread in "${threads[@]}"; do
    for turn in "${turns[@]}"; do
      go run . -w $size -h $size -t $thread -turns $turn >> $logfile
    done
  done
done
