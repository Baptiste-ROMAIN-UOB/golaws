package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
)

type Params struct {
	ImageWidth  int `json:"imageWidth"`
	ImageHeight int `json:"imageHeight"`
	Threads     int `json:"threads"`
	Turns       int `json:"turns"`
}

func calculateNextState(start, end int, p Params, world [][]byte) [][]byte {
	// Implémentation simplifiée de calculateNextState
	newWorld := make([][]byte, p.ImageHeight)
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageWidth)
	}
	for y := 0; y < p.ImageHeight; y++ {
		for x := start; x < end; x++ {
			// Simuler une modification de l'état des cellules
			newWorld[y][x] = world[y][x] ^ 255
		}
	}
	return newWorld
}

func worker(start, end int, p Params, world [][]byte) [][]byte {
	return calculateNextState(start, end, p, world)
}

// Fonction pour gérer les connexions TCP
func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Println("Nouvelle connexion établie.")

	var p Params
	decoder := json.NewDecoder(conn)

	// Lire la requête envoyée par le client
	if err := decoder.Decode(&p); err != nil {
		log.Println("Erreur de décodage JSON:", err)
		conn.Write([]byte("Erreur de décodage JSON"))
		return
	}

	// Créer un monde initial
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	// Canal pour collecter les résultats des workers
	sectionResults := make(chan [][]byte, p.Threads)

	// Exécuter le worker dans des goroutines
	var wg sync.WaitGroup
	segmentWidth := p.ImageWidth / p.Threads
	for i := 0; i < p.Threads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := i * segmentWidth
			end := (i + 1) * segmentWidth
			if i == p.Threads-1 {
				end = p.ImageWidth // Dernier segment pour couvrir toute la largeur
			}
			// Appeler le worker
			newWorld := worker(start, end, p, world)
			sectionResults <- newWorld
		}(i)
	}

	// Attendre que tous les workers aient terminé
	wg.Wait()
	close(sectionResults)

	// Fusionner les résultats des workers
	finalWorld := make([][]byte, p.ImageHeight)
	for i := 0; i < p.ImageHeight; i++ {
		finalWorld[i] = make([]byte, p.ImageWidth)
	}
	for i := 0; i < p.Threads; i++ {
		newWorld := <-sectionResults
		start := i * segmentWidth
		end := (i + 1) * segmentWidth
		if i == p.Threads-1 {
			end = p.ImageWidth
		}
		for y := 0; y < p.ImageHeight; y++ {
			for x := start; x < end; x++ {
				finalWorld[y][x] = newWorld[y][x]
			}
		}
	}

	// Répondre au client avec l'état final du monde
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(finalWorld); err != nil {
		log.Println("Erreur d'encodage de la réponse:", err)
		conn.Write([]byte("Erreur d'encodage de la réponse"))
		return
	}
	log.Println("Réponse envoyée au client.")
}

func main() {
	// Créer un écouteur TCP
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("Erreur d'écoute TCP:", err)
	}
	defer listener.Close()

	fmt.Println("Serveur TCP en écoute sur le port 8080...")

	// Accepter les connexions entrantes
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Erreur de connexion:", err)
			continue
		}
		// Gérer la connexion dans une goroutine
		go handleConnection(conn)
	}
}
