package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

// Structure pour représenter un worker
type Worker struct {
	Client *rpc.Client
}

var (
	workers     = make([]*Worker, 0) // Liste des workers
	workerMutex sync.Mutex           // Mutex pour garantir un accès sécurisé aux workers
)

type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

type Params struct {
	Width  int
	Height int
}

type Engine struct{}

// Fonction pour attribuer un travail à un worker disponible
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	// Cherche un worker qui est enregistré
	if len(workers) > 0 {
		return workers[0], nil
	}
	return nil, fmt.Errorf("Aucun worker disponible")
}

// Fonction qui calcule l'état suivant du tableau sur un worker
func (e *Engine) State(req SegmentRequest, res *[][]byte) error {
	// Obtention d'un worker disponible
	worker, err := getAvailableWorker()
	if err != nil {
		return fmt.Errorf("Erreur lors de l'obtention d'un worker : %v", err)
	}

	// Envoi de la requête de calcul au worker
	err = worker.Client.Call("Worker.State", req, res)
	if err != nil {
		return fmt.Errorf("Erreur lors de l'appel RPC au worker : %v", err)
	}

	return nil
}

// Fonction pour enregistrer un worker auprès du serveur
func (e *Engine) RegisterWorker(workerAddr string, success *bool) error {
	worker := &Worker{
		Client: nil,
	}

	// Tentative de connexion au worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("Erreur de connexion au worker : %v", err)
	}

	worker.Client = client

	// Enregistrer le worker
	workerMutex.Lock()
	workers = append(workers, worker)
	workerMutex.Unlock()

	*success = true
	fmt.Printf("[DEBUG] Worker enregistré : %s\n", workerAddr)
	return nil
}

// Démarrer le serveur RPC
func startServer() {
	engine := new(Engine)
	rpc.Register(engine)

	// Démarrage du serveur
	fmt.Println("[DEBUG] Démarrage du serveur RPC sur le port 8080")

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Erreur lors du démarrage du serveur : %v", err)
	}
	defer listener.Close()

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
	// Démarrer le serveur
	startServer()
}
