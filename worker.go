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

// Ping permet de vérifier la connexion avec le worker
func (w *Worker) Ping(request bool, response *bool) error {
	*response = true
	return nil
}

// Fonction pour calculer l’état suivant pour un segment
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
	// Débogage : afficher les détails de la requête
	fmt.Printf("[DEBUG] Reçu segment pour le calcul [%d-%d] de la grille de %d x %d\n", req.Start, req.End, req.Params.Width, req.Params.Height)

	// On ne fait que copier le segment sans modification
	newSegment := make([][]byte, req.End-req.Start)
	for i := range newSegment {
		newSegment[i] = make([]byte, req.Params.Width)
		copy(newSegment[i], req.World[req.Start+i])
	}

	res.NewSegment = newSegment
	return nil
}

// Fonction pour s’enregistrer auprès du serveur
func registerWithServer(serverAddr string, workerAddr string) error {
	// Débogage : tentative d'enregistrement du worker
	fmt.Printf("[DEBUG] Tentative d'enregistrement du worker à l'adresse %s\n", workerAddr)

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

	// Débogage : confirmation de l'enregistrement
	fmt.Println("[DEBUG] Worker enregistré avec succès :", workerAddr)
	return nil
}

// Fonction pour démarrer le listener RPC du worker
func startWorkerServer(port string) {
	worker := new(Worker)
	rpc.Register(worker)

	// Débogage : démarrage du serveur RPC
	fmt.Printf("[DEBUG] Démarrage du serveur RPC du worker sur le port %s\n", port)

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
		// Débogage : nouvelle connexion acceptée
		fmt.Printf("[DEBUG] Connexion acceptée depuis : %v\n", conn.RemoteAddr())
		go rpc.ServeConn(conn)
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run worker.go <workerPort> <serverIP>")
		os.Exit(1)
	}
	workerPort := os.Args[1] // Port du worker
	serverIP := os.Args[2]   // Adresse du serveur

	// On suppose que le serveur écoute sur le port 8080
	serverPort := "8080"
	serverAddr := net.JoinHostPort(serverIP, serverPort)
	workerAddr := "localhost:" + workerPort

	// Essayer de s’enregistrer auprès du serveur
	for {
		err := registerWithServer(serverAddr, workerAddr)
		if err == nil {
			break
		}
		// Débogage : attendre avant de réessayer l'enregistrement
		fmt.Println("[DEBUG] Tentative de reconnexion dans 5 secondes...")
		time.Sleep(5 * time.Second)
	}

	// Démarrer le serveur RPC du worker
	startWorkerServer(workerPort)
}
