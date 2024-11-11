package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
)

// Params contains game parameters
type Params struct {
	Width  int
	Height int
}

// SegmentRequest contains segment computation request information
type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

// SegmentResponse contains segment computation response information
type SegmentResponse struct {
	NewSegment [][]byte
}

// Worker handles computation tasks
type Worker struct {
	rpcClient *rpc.Client // Maintains connection with the server
}

// Ping checks if the Worker is online
func (w *Worker) Ping(request bool, response *bool) error {
	log.Println("Received ping request")
	*response = true
	return nil
}

// countAliveNeighbors computes the number of live neighbors around a cell
func countAliveNeighbors(world [][]byte, x, y int) int {
	aliveCount := 0
	directions := []struct{ dx, dy int }{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	height := len(world)
	width := len(world[0])

	for _, dir := range directions {
		neighborX := (x + dir.dx + width) % width
		neighborY := (y + dir.dy + height) % height
		if world[neighborY][neighborX] == 255 {
			aliveCount++
		}
	}
	return aliveCount
}

// State computes the new state of the cells
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
	log.Printf("Worker received computation request: Segment [%d-%d], Grid Size: %dx%d",
		req.Start, req.End, req.Params.Width, req.Params.Height)

	// Validate input
	if req.World == nil {
		return fmt.Errorf("input world is nil")
	}

	if req.Start < 0 || req.End > len(req.World) {
		return fmt.Errorf("invalid segment range: [%d-%d], world height: %d",
			req.Start, req.End, len(req.World))
	}

	// Create new segment
	segmentHeight := req.End - req.Start
	newSegment := make([][]byte, segmentHeight)
	for i := range newSegment {
		newSegment[i] = make([]byte, req.Params.Width)
	}

	// Compute the new state for each cell
	for y := req.Start; y < req.End; y++ {
		for x := 0; x < req.Params.Width; x++ {
			aliveNeighbors := countAliveNeighbors(req.World, x, y)
			segY := y - req.Start

			// Apply Game of Life rules
			if req.World[y][x] == 255 {
				if aliveNeighbors == 2 || aliveNeighbors == 3 {
					newSegment[segY][x] = 255
				}
			} else if aliveNeighbors == 3 {
				newSegment[segY][x] = 255
			}
		}
	}

	log.Printf("Worker completed computation of segment [%d-%d]", req.Start, req.End)
	res.NewSegment = newSegment
	return nil
}

// startWorkerServer starts the RPC server for the Worker
func startWorkerServer(port string, worker *Worker) error {
	if err := rpc.Register(worker); err != nil {
		return fmt.Errorf("failed to register Worker: %v", err)
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to start Worker listener: %v", err)
	}

	log.Printf("Worker started successfully, listening on port: %s", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		log.Printf("Accepted new connection from: %v", conn.RemoteAddr())
		go rpc.ServeConn(conn)
	}
}

// registerWithServer registers the Worker with the server
func registerWithServer(serverAddr, workerAddr string) error {
	// Connect to the server
	client, err := rpc.Dial("tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	defer client.Close()

	// Register Worker
	var success bool
	if err := client.Call("Engine.RegisterWorker", workerAddr, &success); err != nil {
		return fmt.Errorf("Worker registration failed: %v", err)
	}

	if !success {
		return fmt.Errorf("server rejected registration")
	}

	log.Printf("Worker successfully registered with server: %s", workerAddr)
	return nil
}

// retryWithBackoff retries registration with a backoff strategy
func retryWithBackoff(serverAddr, workerAddr string) {
	maxRetries := 5
	for retry := 0; retry < maxRetries; retry++ {
		err := registerWithServer(serverAddr, workerAddr)
		if err == nil {
			log.Println("Worker registered successfully")
			return
		}

		backoff := time.Duration(retry+1) * 5 * time.Second
		log.Printf("Registration failed, retrying in %v (%d/%d): %v",
			backoff, retry+1, maxRetries, err)
		time.Sleep(backoff)
	}
	log.Fatal("Reached maximum retries, registration failed")
}

// getLocalIP retrieves the local IP address
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

func main() {
	// Check command-line arguments
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run worker.go <worker_port> <server_ip>")
		os.Exit(1)
	}

	workerPort := os.Args[1]
	serverIP := os.Args[2]

	// Get local IP
	localIP, err := getLocalIP()
	if err != nil {
		log.Fatalf("Failed to get local IP: %v", err)
	}

	// Build addresses
	workerAddr := net.JoinHostPort(localIP, workerPort)
	serverAddr := net.JoinHostPort(serverIP, "8080")

	log.Printf("Worker address: %s", workerAddr)
	log.Printf("Server address: %s", serverAddr)

	// Create worker instance
	worker := new(Worker)

	// Start RPC server
	go func() {
		if err := startWorkerServer(workerPort, worker); err != nil {
			log.Fatalf("Worker server failed to start: %v", err)
		}
	}()

	// Wait for RPC server to start
	time.Sleep(time.Second)

	// Register with server
	retryWithBackoff(serverAddr, workerAddr)

	// Keep running
	select {}
}
