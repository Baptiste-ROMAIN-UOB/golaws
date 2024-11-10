**Rapport sur l'implémentation parallèle du Jeu de la Vie**

*COMS20008 - Baptiste ROMAIN - tz23682*



### Implémentation Parallèle

#### Fonctionnalités et conception

Dans cette implémentation du Jeu de la Vie en Go, on a paralléliser les différentes taches du programme pour optimiser son exécution. Le programme est organisé en plusieurs fichiers : `gol.go`, `distributor.go`, `io.go` et `event.go`.

L'objectif de la parallélisation est de diviser l'image d'entrée en plusieurs parties égales, que chaque goroutine traite indépendamment pour accélérer le calcul du nouvel état du plateau. 

Le fichier principal `gol.go` gère l'entrée et l'initialisation des canaux, puis lance les goroutines et fonctions nécessaires, notamment celles pour la gestion des entrées/sorties (avec `startIO` pour la lecture et l’écriture des fichiers PGM) et la distribution du travail (`distributor`).

Le fichier `distributor.go`, cœur de l'implémentation, est responsable de l'organisation du travail entre les différents threads. Ce fichier calcule l'état de la grille à chaque tour et gère également les interactions avec l'utilisateur, comme les commandes pour mettre en pause, quitter ou enregistrer l'état du plateau sous forme d'image.

#### Conception modulaire et parallèle

Le cœur du programme repose sur la capacité à distribuer la charge de travail entre plusieurs goroutines. Lorsque le nombre de tours à exécuter et le nombre de threads sont spécifiés, le programme divise l'image en plusieurs morceaux. Chaque goroutine est ensuite responsable de calculer l'état des cellules dans sa propre section du tableau.

Cependant, un problème majeur a été rencontré lors de la tentative de division uniforme de l'image en sections égales. En effet, la largeur de l'image n’est pas toujours divisée de manière égale par le nombre de threads, ce qui nécessite un ajustement du code pour gérer ces cas.

Le processus se déroule ainsi : chaque goroutine reçoit une partie de la grille à traiter, puis modifie une copie locale du tableau pour éviter les conditions de concurrence (race conditions). Une fois le calcul terminé, chaque goroutine envoie son résultat dans un canal de sortie, et l'état global de la grille est reconstruit à partir des résultats des goroutines.

Une fois que tous les threads ont terminé leur exécution, le programme se synchronise pour reconstruire l'état final du plateau, puis si la situation le nécessite le programme répondre aux commandes utilisateur.

#### Analyse critique

L'efficacité de cette approche dépend fortement de plusieurs facteurs : la taille de l'image, le nombre de tours à exécuter, et surtout le nombre de threads utilisés. La parallélisation permet d’accélérer le calcul, mais au-delà d'un certain nombre de threads, l'efficacité stagne. En effet, la performance est limitée par le nombre de cœurs du processeur, et une fois que le nombre de threads dépasse la capacité du matériel, les threads supplémentaires ne sont plus exécutés en parallèle. Cela entraîne une stagnation de la performance, et dans certains cas, peut même provoquer des défaillances matérielles.

Dans mon cas de test avec une image de 512x512 et 1000 tours, j’ai observé que l’efficacité augmentait avec le nombre de threads jusqu’à un certain point, puis plafonnait lorsque le nombre de threads dépassait celui des cœurs disponibles sur la machine. Une fois ce seuil dépassé, les threads supplémentaires n’apportaient plus d’amélioration et, dans certains cas, réduisaient même l’efficacité.

#### Détails de l'implémentation

Le fichier `distributor.go` est central dans cette implémentation, car il calcule l'état de la grille à chaque tour et traite les différentes commandes utilisateur. Le programme a été organisé de manière modulaire afin de faciliter la gestion des différentes étapes de l’exécution.

La fonction principale du fichier `distributor.go` initialise l'état du monde et entre dans une boucle où elle gère les tours successifs. À chaque tour, elle distribue le calcul du nouvel état de la grille entre les goroutines et s'assure que toutes les tâches sont bien terminées avant de passer au tour suivant. Cette gestion est rendue possible grâce à l'utilisation de canaux de communication entre les goroutines.

Les fonctions auxiliaires sont utilisées pour éviter les répétitions de code et faciliter le calcul de l'état suivant de la grille. Ces fonctions traitent des tâches spécifiques, comme le calcul du nombre de cellules vivantes, la gestion des cellules voisines ou la création de fichiers PGM.

### Conclusion

En conclusion, cette implémentation parallèle du Jeu de la Vie en Go permet d'exécuter le programme de manière plus efficace en répartissant le travail entre plusieurs goroutines. Cependant, la performance de cette parallélisation dépend fortement des ressources matérielles disponibles et du nombre de threads utilisés. Lorsque le nombre de threads dépasse la capacité du processeur, l'efficacité commence à stagner, ce qui limite les gains possibles. 


### CSA Coursework: Game of Life skeleton (Go)

All documentation is available [here](https://uob-csa.github.io/gol-docs/)