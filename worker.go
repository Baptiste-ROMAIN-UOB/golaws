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

// Fonction pour calculer l’état suivant pour un segment
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
    // Débogage : afficher les détails de la requête
    fmt.Printf("[DEBUG] Calcul de l'état suivant pour le segment [%d-%d] de la grille de %d x %d\n", req.Start, req.End, req.Params.Width, req.Params.Height)

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

    // Débogage : afficher les résultats du calcul
    fmt.Printf("[DEBUG] Calcul terminé pour le segment [%d-%d], nouvelles cellules vivantes : %d\n", req.Start, req.End, countAliveCells(newSegment))

    res.NewSegment = newSegment
    return nil
}

// Fonction pour compter le nombre de cellules vivantes dans un segment
func countAliveCells(segment [][]byte) int {
    count := 0
    for _, row := range segment {
        for _, cell := range row {
            if cell == 255 {
                count++
            }
        }
    }
    return count
}

// Fonction pour s’enregistrer auprès du serveur
func registerWithServer(serverAddr string, workerAddr string) error {
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

    fmt.Println("[DEBUG] Worker enregistré avec succès :", workerAddr)
    return nil
}

// Fonction pour démarrer le listener RPC du worker
func startWorkerServer(port string) {
    worker := new(Worker)
    rpc.RegisterName("Worker", worker)

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
        fmt.Printf("[DEBUG] Connexion acceptée depuis : %v\n", conn.RemoteAddr())
        go rpc.ServeConn(conn)
    }
}

func main() {
    if len(os.Args) < 3 {
        fmt.Println("Usage: go run worker.go <workerPort> <serverIP>")
        os.Exit(1)
    }
    workerPort := os.Args[1]
    serverIP := os.Args[2]

    serverPort := "8080"
    serverAddr := net.JoinHostPort(serverIP, serverPort)
    workerAddr := "localhost:" + workerPort

    // Démarrer le serveur RPC du worker
    go startWorkerServer(workerPort)

    // Attendre quelques secondes pour s'assurer que le serveur est prêt
    time.Sleep(2 * time.Second)

    // Essayer de s’enregistrer auprès du serveur
    for {
        err := registerWithServer(serverAddr, workerAddr)
        if err == nil {
            break
        }
        fmt.Println("[DEBUG] Tentative de reconnexion dans 5 secondes...")
        time.Sleep(5 * time.Second)
    }
}
