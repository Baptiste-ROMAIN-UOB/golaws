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

type SegmentResponse struct {
    NewSegment [][]byte
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
func (e *Engine) State(req SegmentRequest, res *SegmentResponse) error {
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

// Nouvelle méthode CalculateNextState
// Divise la grille en plusieurs segments et demande à chaque worker de calculer un segment
func (e *Engine) CalculateNextState(req SegmentRequest, res *[][]byte) error {
    numWorkers := len(workers)
    if numWorkers == 0 {
        return fmt.Errorf("Aucun worker disponible pour calculer l'état suivant")
    }

    // Découper la grille en segments
    segmentHeight := req.Params.Height / numWorkers
    responses := make([]*SegmentResponse, numWorkers)
    var wg sync.WaitGroup
    wg.Add(numWorkers)

    // Envoi des requêtes aux workers
    for i := 0; i < numWorkers; i++ {
        start := i * segmentHeight
        end := (i + 1) * segmentHeight
        if i == numWorkers-1 {
            // Le dernier worker prend la fin du tableau
            end = req.Params.Height
        }

        segmentReq := SegmentRequest{
            Start:  start,
            End:    end,
            World:  req.World,
            Params: req.Params,
        }

        go func(i int, segmentReq SegmentRequest) {
            defer wg.Done()

            // Calculer l'état suivant pour ce segment
            var response SegmentResponse
            err := e.State(segmentReq, &response)
            if err != nil {
                log.Printf("[ERROR] Erreur lors du calcul de l'état pour le segment %d-%d: %v", segmentReq.Start, segmentReq.End, err)
                return
            }

            responses[i] = &response
        }(i, segmentReq)
    }

    // Attendre que tous les workers aient terminé
    wg.Wait()

    // Combiner les résultats des workers
    // Construire la grille complète en combinant les segments calculés
    result := make([][]byte, req.Params.Height)
    for i := 0; i < numWorkers; i++ {
        start := i * segmentHeight
        end := (i + 1) * segmentHeight
        if i == numWorkers-1 {
            end = req.Params.Height
        }

        // Copie les données du segment calculé dans la grille finale
        for y := start; y < end; y++ {
            result[y] = responses[i].NewSegment[y-start]
        }
    }

    *res = result
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
