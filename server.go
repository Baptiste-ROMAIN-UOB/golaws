package main

import (
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"time"
)

type Worker struct {
	Client *rpc.Client
	Active bool
	LastPing time.Time
}

var (
	workers     = make([]*Worker, 0) // Liste des workers avec état
	workerMutex sync.Mutex           // Mutex pour garantir un accès sécurisé aux workers
)

type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

type Params struct {
	// Paramètres nécessaires pour le calcul du tableau (taille, règles, etc.)
	Width  int
	Height int
}

type Engine struct{}

// Fonction pour attribuer un travail à un worker libre
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	// Cherche un worker actif
	for _, worker := range workers {
		if worker.Active {
			return worker, nil
		}
	}
	return nil, fmt.Errorf("Aucun worker actif disponible")
}

// Fonction qui calcule l'état suivant du tableau sur un worker
func (e *Engine) CalculateNextState(req SegmentRequest, res *[][]byte) error {
	// Obtention d'un worker disponible
	worker, err := getAvailableWorker()
	if err != nil {
		return fmt.Errorf("Erreur lors de l'obtention d'un worker : %v", err)
	}

	// Envoi de la requête de calcul au worker
	err = worker.Client.Call("Worker.Calculate", req, res)
	if err != nil {
		worker.Active = false // Marquer le worker comme inactif s'il échoue
		return fmt.Errorf("Erreur lors de l'appel RPC au worker : %v", err)
	}
	return nil
}

// Fonction pour enregistrer un worker auprès du serveur
func (e *Engine) RegisterWorker(workerAddr string, res *bool) error {
	// Créer un client RPC pour le worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("Erreur lors de la connexion au worker : %v", err)
	}

	// Enregistrer le worker dans la liste comme actif
	workerMutex.Lock()
	workers = append(workers, &Worker{Client: client, Active: true, LastPing: time.Now()})
	workerMutex.Unlock()

	*res = true
	fmt.Println("Worker enregistré :", workerAddr)
	return nil
}

// Fonction pour vérifier périodiquement l'état des workers
func (e *Engine) MonitorWorkers() {
	for {
		time.Sleep(5 * time.Second)
		workerMutex.Lock()
		for _, worker := range workers {
			if worker.Active {
				var pingRes bool
				err := worker.Client.Call("Worker.Ping", true, &pingRes)
				if err != nil || !pingRes {
					// Marquer le worker comme inactif s'il ne répond pas
					fmt.Println("Worker inactif détecté :", worker.Client)
					worker.Active = false
				} else {
					// Mettre à jour l'horodatage du dernier ping réussi
					worker.LastPing = time.Now()
				}
			}
		}
		workerMutex.Unlock()
	}
}

// Fonction pour démarrer le serveur
func startServer(port string) {
	engine := new(Engine)
	rpc.Register(engine)

	// Lancer le monitoring des workers
	go engine.MonitorWorkers()

	// Écoute les connexions RPC sur le port spécifié
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Erreur lors de la création du serveur : ", err)
		return
	}
	defer listener.Close()

	fmt.Println("Serveur démarré sur le port", port)
	for {
		// Accepte les connexions entrantes et les traite avec des goroutines
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Erreur lors de l'acceptation de la connexion : ", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

func main() {
	startServer("8080")
}
