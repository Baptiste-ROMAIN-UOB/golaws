// worker.go
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

// SegmentRequest contains information for segment calculation requests
type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

// SegmentResponse contains segment calculation response information
type SegmentResponse struct {
	NewSegment [][]byte
}

// Worker handles computation tasks
type Worker struct{}

// Ping checks if the Worker is online
func (w *Worker) Ping(request bool, response *bool) error {
	*response = true
	return nil
}

// countAliveNeighbors counts the number of alive cells around a cell
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

// State calculates the new state of cells
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
	log.Printf("[debug] Worker received calculation request: segment [%d-%d], grid size: %dx%d", req.Start, req.End, req.Params.Width, req.Params.Height)

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

	log.Printf("[debug] Worker completed calculation for segment [%d-%d]", req.Start, req.End)
	res.NewSegment = newSegment
	return nil
}

// registerWithServer tries to register the Worker with the server
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
	log.Printf("[debug] Worker registered successfully: %s", workerAddr)
	return nil
}

// startWorkerServer starts the RPC server for the Worker
func startWorkerServer(port string, worker *Worker) error {
	if err := rpc.Register(worker); err != nil {
		return fmt.Errorf("Worker registration failed: %v", err)
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
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// retryWithBackoff tries to register with the server, using exponential backoff
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
		fmt.Println("Usage: go run worker.go <worker port> <server IP>")
		os.Exit(1)
	}
	workerPort := os.Args[1]
	serverIP := os.Args[2]

	localIP, err := getLocalIP()
	if err != nil {
		log.Fatalf("Failed to get local IP address: %v", err)
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
