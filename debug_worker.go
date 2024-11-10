package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
)

// Params defines the parameters for the Game of Life grid
type Params struct {
	Width  int // Grid width
	Height int // Grid height
}

// SegmentRequest represents a request for processing a segment of the grid
type SegmentRequest struct {
	Start  int      // Starting row of the segment
	End    int      // Ending row of the segment
	World  [][]byte // Segment data for the grid
	Params Params   // Grid dimensions
}

// SegmentResponse holds the processed data for a grid segment
type SegmentResponse struct {
	NewSegment [][]byte // Processed segment of the grid
}

// Worker is responsible for handling computation tasks
type Worker struct{}

// Ping checks if the Worker is online
func (w *Worker) Ping(request bool, response *bool) error {
	*response = true
	return nil
}

// countAliveNeighbors calculates the number of live neighbors around a cell
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

// State calculates the next state for a segment of cells in the grid
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
	log.Printf("[DEBUG] Worker received computation request for segment [%d-%d], grid size: %dx%d", req.Start, req.End, req.Params.Width, req.Params.Height)

	newSegment := make([][]byte, req.End-req.Start)
	for i := range newSegment {
		newSegment[i] = make([]byte, req.Params.Width)
	}

	for y := req.Start; y < req.End; y++ {
		for x := 0; x < req.Params.Width; x++ {
			aliveNeighbors := countAliveNeighbors(req.World, x, y)
			segY := y - req.Start

			if req.World[y][x] == 255 {
				if aliveNeighbors == 2 || aliveNeighbors == 3 {
					newSegment[segY][x] = 255
				}
			} else if aliveNeighbors == 3 {
				newSegment[segY][x] = 255
			}
		}
	}

	log.Printf("[DEBUG] Worker completed computation for segment [%d-%d]", req.Start, req.End)
	res.NewSegment = newSegment
	return nil
}

// registerWithServer attempts to register the Worker with the server
func registerWithServer(serverAddr, workerAddr string) error {
	client, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer client.Close()

	rpcClient := rpc.NewClient(client)
	var success bool
	if err := rpcClient.Call("Engine.RegisterWorker", workerAddr, &success); err != nil || !success {
		return fmt.Errorf("Worker registration failed: %v", err)
	}
	log.Printf("[DEBUG] Worker successfully registered: %s", workerAddr)
	return nil
}

// startWorkerServer starts the Worker’s RPC server
func startWorkerServer(port string, worker *Worker) error {
	if err := rpc.Register(worker); err != nil {
		return fmt.Errorf("failed to register Worker: %v", err)
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to start Worker listener: %v", err)
	}
	defer listener.Close()

	log.Printf("Worker started, listening on port: %s", port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("error accepting connection: %v", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// retryWithBackoff attempts to register with the server using exponential backoff
func retryWithBackoff(serverAddr, workerAddr string) {
	const maxRetries = 5
	for i := 0; i < maxRetries; i++ {
		if err := registerWithServer(serverAddr, workerAddr); err == nil {
			return
		}
		backoff := time.Duration(i+1) * 5 * time.Second
		log.Printf("Registration failed, retrying in %v (%d/%d)", backoff, i+1, maxRetries)
		time.Sleep(backoff)
	}
	log.Fatalf("Registration failed, maximum retries reached")
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run worker.go <workerPort> <serverIP>")
		os.Exit(1)
	}
	workerPort := os.Args[1]
	serverIP := os.Args[2]

	localIP, err := getLocalIP()
	if err != nil {
		log.Fatalf("Failed to obtain local IP address: %v", err)
	}
	workerAddr := net.JoinHostPort(localIP, workerPort)
	serverAddr := net.JoinHostPort(serverIP, "8080")

	worker := new(Worker)
	go func() {
		if err := startWorkerServer(workerPort, worker); err != nil {
			log.Fatalf("Failed to start Worker server: %v", err)
		}
	}()
	retryWithBackoff(serverAddr, workerAddr)

	select {}
}

// getLocalIP retrieves the local non-loopback IP address
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String(), nil
		}
	}
	return "localhost", nil
}
