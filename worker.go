package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
)

// Paramètres pour la grille de jeu de la vie
type Params struct {
	Width  int
	Height int
}

// Structure de la requête pour un calcul segmenté
type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

// Structure de réponse pour le calcul d'un segment
type SegmentResponse struct {
	NewSegment [][]byte
}

// Worker représente la structure principale du worker
type Worker struct{}

// Fonction pour compter les voisins vivants autour d’une cellule
func countAliveNeighbors(world [][]byte, x, y int) int {
	aliveCount := 0
	directions := []struct{ dx, dy int }{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1},         {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	for _, dir := range directions {
		neighborX := (x + dir.dx + len(world[0])) % len(world[0])
		neighborY := (y + dir.dy + len(world)) % len(world)
		if world[neighborY][neighborX] == 255 {
			aliveCount++
		}
	}
	return aliveCount
}

// Fonction pour calculer l’état suivant pour un segment
func (w *Worker) CalculateNextState(req SegmentRequest, res *SegmentResponse) error {
	newSegment := make([][]byte, req.End-req.Start)
	for i := range newSegment {
		newSegment[i] = make([]byte, req.Params.Width)
	}

	for y := req.Start; y < req.End; y++ {
		for x := 0; x < req.Params.Width; x++ {
			aliveNeighbors := countAliveNeighbors(req.World, x, y)
			if req.World[y][x] == 255 {
				if aliveNeighbors == 2 || aliveNeighbors == 3 {
					newSegment[y-req.Start][x] = 255
				} else {
					newSegment[y-req.Start][x] = 0
				}
			} else {
				if aliveNeighbors == 3 {
					newSegment[y-req.Start][x] = 255
				} else {
					newSegment[y-req.Start][x] = 0
				}
			}
		}
	}

	res.NewSegment = newSegment
	return nil
}

// Fonction pour s’enregistrer auprès du serveur
func registerWithServer(serverAddr string, workerAddr string) error {
	client, err := rpc.Dial("tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("Erreur de connexion au serveur : %v", err)
	}
	defer client.Close()

	var success bool
	err = client.Call("Engine.RegisterWorker", workerAddr, &success)
	if err != nil || !success {
		return fmt.Errorf("Erreur d'enregistrement du worker : %v", err)
	}
	fmt.Println("Worker enregistré avec succès :", workerAddr)
	return nil
}

// Fonction pour démarrer le listener RPC du worker
func startWorkerServer(port string) {
	worker := new(Worker)
	rpc.Register(worker)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Erreur lors du démarrage du worker : %v", err)
	}
	defer listener.Close()

	fmt.Println("Worker démarré sur le port", port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Erreur lors de l'acceptation de connexion : ", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run worker.go <workerPort> <serverIP> <serverPort>")
		os.Exit(1)
	}
	workerPort := os.Args[1]
	serverIP := os.Args[2]
	serverPort := os.Args[3]

	// Adresse du serveur et du worker pour l'enregistrement
	serverAddr := net.JoinHostPort(serverIP, serverPort)
	workerAddr := "localhost:" + workerPort

	// Essayer de s’enregistrer auprès du serveur
	for {
		err := registerWithServer(serverAddr, workerAddr)
		if err == nil {
			break
		}
		fmt.Println("Tentative de reconnexion dans 5 secondes...")
		time.Sleep(5 * time.Second)
	}

	// Démarrer le serveur RPC du worker
	startWorkerServer(workerPort)
}
//go run worker.go 8081 127.0.0.1 8080