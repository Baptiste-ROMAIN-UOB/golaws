package main

import (
    "fmt"
    "log"
    "net"
    "net/rpc"
    "sync"
    "time"
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
                // Si un worker échoue, on peut essayer avec un autre worker ou calculer localement
                // Dans ce cas, on calcule localement le segment
                e.calculateSegmentLocally(segmentReq, &response)
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
        copy(result[start:end], responses[i].NewSegment)
    }

    *res = result
    return nil
}

// Fonction pour calculer un segment localement si aucun worker ne répond
func (e *Engine) calculateSegmentLocally(req SegmentRequest, res *SegmentResponse) {
    fmt.Printf("[DEBUG] Calcul local pour le segment [%d-%d]\n", req.Start, req.End)
    newSegment := make([][]byte, req.End-req.Start)
    for i := range newSegment {
        newSegment[i] = make([]byte, req.Params.Width)
    }

    // Calcul de l'état suivant pour chaque cellule du segment
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
}

// Fonction pour compter les voisins vivants autour d’une cellule
func countAliveNeighbors(world [][]byte, x, y int) int {
    aliveCount := 0
    directions := []struct{ dx, dy int }{
        {-1, -1}, {-1, 0}, {-1, 1},
        {0, -1}, {0, 1},
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
        fmt.Printf("[DEBUG] Connexion acceptée depuis : %v\n", conn.RemoteAddr())
        go rpc.ServeConn(conn)
    }
}

func main() {
    // Démarrer le serveur
    startServer()
}
