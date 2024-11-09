package main

import (
	"fmt"
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

	// Débogage : afficher les workers disponibles
	fmt.Printf("[DEBUG] Nombre de workers enregistrés : %d\n", len(workers))
	for i, worker := range workers {
		fmt.Printf("[DEBUG] Worker %d : %v\n", i, worker.Client)
	}

	// Cherche un worker qui est enregistré
	if len(workers) > 0 {
		return workers[0], nil
	}
	return nil, fmt.Errorf("Aucun worker disponible")
}

// Fonction qui calcule l'état suivant du tableau sur un worker
// Fonction qui calcule l'état suivant du tableau sur un worker
func (e *Engine) CalculateNextState(req SegmentRequest, res *[][]byte) error {
	// Débogage : afficher les détails de la requête
	fmt.Printf("[DEBUG] Demande de calcul pour la plage [%d-%d]\n", req.Start, req.End)

	// Obtention d'un worker disponible
	worker, err := getAvailableWorker()
	if err != nil {
		return fmt.Errorf("Erreur lors de l'obtention d'un worker : %v", err)
	}

	// Débogage : afficher le worker utilisé pour la tâche
	fmt.Printf("[DEBUG] Envoi de la requête au worker : %v\n", worker.Client)

	// Envoi de la requête de calcul au worker (ici on appelle la méthode correcte)
	err = worker.Client.Call("Worker.CalculateNextState", req, res)  // Appel à la méthode correcte
	if err != nil {
		return fmt.Errorf("Erreur lors de l'appel RPC au worker : %v", err)
	}

	// Débogage : tâche terminée
	fmt.Printf("[DEBUG] Calcul terminé pour la plage [%d-%d]\n", req.Start, req.End)
	return nil
}


// Fonction pour enregistrer un worker auprès du serveur
func (e *Engine) RegisterWorker(workerAddr string, res *bool) error {
	// Débogage : afficher l'adresse du worker
	fmt.Printf("[DEBUG] Tentative de connexion au worker à l'adresse : %s\n", workerAddr)

	// Créer un client RPC pour le worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("Erreur lors de la connexion au worker : %v", err)
	}

	// Enregistrer le worker dans la liste
	workerMutex.Lock()
	workers = append(workers, &Worker{Client: client})
	workerMutex.Unlock()

	// Débogage : confirmation d'enregistrement
	*res = true
	fmt.Printf("[DEBUG] Worker enregistré : %s\n", workerAddr)
	return nil
}

// Fonction pour accepter un client et l'ajouter à la liste des workers
func (e *Engine) AcceptClient(workerAddr string, res *bool) error {
	// Tentative de connexion au worker
	fmt.Printf("[DEBUG] Acceptation du worker à l'adresse : %s\n", workerAddr)

	// Créer un client RPC pour le worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("Erreur lors de la connexion au worker : %v", err)
	}

	// Enregistrer le worker dans la liste
	workerMutex.Lock()
	workers = append(workers, &Worker{Client: client})
	workerMutex.Unlock()

	// Débogage : confirmation d'enregistrement
	*res = true
	fmt.Printf("[DEBUG] Worker accepté et enregistré : %s\n", workerAddr)
	return nil
}

// Fonction pour démarrer le serveur
func startServer(port string) {
	engine := new(Engine)
	rpc.Register(engine)

	// Débogage : affichage du démarrage du serveur
	fmt.Printf("[DEBUG] Démarrage du serveur sur le port %s...\n", port)

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

		// Débogage : connexion acceptée
		fmt.Printf("[DEBUG] Connexion acceptée : %v\n", conn.RemoteAddr())

		// Traiter la connexion avec une goroutine
		go rpc.ServeConn(conn)
	}
}

func main() {
	startServer("8080")
}
