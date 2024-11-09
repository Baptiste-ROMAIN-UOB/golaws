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

// Fonction pour calculer l’état suivant pour un segment
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
	// Débogage : afficher les détails de la requête
	fmt.Printf("[DEBUG] Calcul de l'état suivant pour le segment [%d-%d] de la grille de %d x %d\n", req.Start, req.End, req.Params.Width, req.Params.Height)

	// Juste renvoyer les données sans les modifier, comme spécifié
	res.NewSegment = req.World[req.Start:req.End]

	return nil
}

// Définir la structure du serveur
type Server struct {
	workers map[string]*rpc.Client
}

// Enregistrer un worker
func (s *Server) RegisterWorker(workerAddr string, success *bool) error {
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("Erreur de connexion au worker : %v", err)
	}
	s.workers[workerAddr] = client
	*success = true
	return nil
}

// Appel RPC pour calculer l'état suivant en utilisant le worker
func (s *Server) CalculateNextState(req SegmentRequest, res *SegmentResponse) error {
	// Rechercher un worker disponible
	var workerAddr string
	for workerAddr = range s.workers {
		break
	}

	if workerAddr == "" {
		return fmt.Errorf("Aucun worker disponible")
	}

	// Appeler la méthode State du worker
	client := s.workers[workerAddr]
	err := client.Call("Worker.State", req, res)
	if err != nil {
		return fmt.Errorf("Erreur lors de l'appel RPC au worker : %v", err)
	}

	return nil
}

// Démarrer le serveur
func startServer(port string) {
	server := &Server{
		workers: make(map[string]*rpc.Client),
	}
	rpc.Register(server)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Erreur lors du démarrage du serveur : %v", err)
	}
	defer listener.Close()

	fmt.Println("Serveur démarré sur le port", port)
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
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run server.go <serverPort>")
		os.Exit(1)
	}
	serverPort := os.Args[1] // Port du serveur

	// Démarrer le serveur
	startServer(serverPort)
}
