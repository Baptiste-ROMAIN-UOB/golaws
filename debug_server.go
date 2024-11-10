package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

// Worker struct to hold the RPC client connection for each worker node
type Worker struct {
	Client *rpc.Client
}

var (
	workers            = make([]*Worker, 0) // List of connected workers
	workerMutex        sync.Mutex           // Mutex for safe access to workers list
	currentWorkerIndex int                  // Index for round-robin task distribution
)

// Params defines the grid dimensions for the Game of Life
type Params struct {
	Width  int // Grid width
	Height int // Grid height
}

// SegmentRequest represents a request for processing a segment of the grid
type SegmentRequest struct {
	Start  int      // Starting row of the segment
	End    int      // Ending row of the segment
	World  [][]byte // Grid data for the segment
	Params Params   // Grid dimensions
}

// SegmentResponse holds the processed data of a grid segment
type SegmentResponse struct {
	NewSegment [][]byte // Processed segment of the grid
}

// DistributedTask represents the complete grid task with parameters
type DistributedTask struct {
	World  [][]byte
	Params struct {
		Width  int // Grid width
		Height int // Grid height
	}
}

// Engine struct for managing distributed tasks
type Engine struct{}

// getAvailableWorker selects an available worker in a round-robin manner
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	numWorkers := len(workers)
	if numWorkers == 0 {
		return nil, fmt.Errorf("no available worker")
	}

	// Select worker using round-robin
	worker := workers[currentWorkerIndex]
	currentWorkerIndex = (currentWorkerIndex + 1) % numWorkers

	fmt.Printf("[DEBUG] Selected worker %d of %d\n", currentWorkerIndex, numWorkers)
	return worker, nil
}

// State processes the grid by dividing it into segments and assigning them to workers
func (e *Engine) State(req DistributedTask, res *[][]byte) error {
	fmt.Printf("[DEBUG] Server received computation request: grid size %dx%d\n", req.Params.Width, req.Params.Height)

	// Validate input
	if req.World == nil {
		return fmt.Errorf("input world is nil")
	}

	if len(req.World) != req.Params.Height {
		return fmt.Errorf("world height mismatch: expected %d, got %d", req.Params.Height, len(req.World))
	}

	for i, row := range req.World {
		if len(row) != req.Params.Width {
			return fmt.Errorf("row %d width mismatch: expected %d, got %d", i, req.Params.Width, len(row))
		}
	}

	workerMutex.Lock()
	numWorkers := len(workers)
	workerMutex.Unlock()

	if numWorkers == 0 {
		return fmt.Errorf("no available worker")
	}

	fmt.Printf("[DEBUG] Preparing to assign tasks to %d workers\n", numWorkers)

	// Calculate number of rows each worker should process
	segmentHeight := req.Params.Height / numWorkers
	if segmentHeight < 1 {
		segmentHeight = 1
	}

	// Create the result grid
	*res = make([][]byte, req.Params.Height)
	for i := range *res {
		(*res)[i] = make([]byte, req.Params.Width)
	}

	// Channels for collecting errors and completed segments
	errChan := make(chan error, numWorkers)
	segmentChan := make(chan struct {
		startY  int
		endY    int
		segment [][]byte
	}, numWorkers)

	numSegments := 0
	// Create segments and assign tasks
	for start := 0; start < req.Params.Height; start += segmentHeight {
		end := start + segmentHeight
		if end > req.Params.Height {
			end = req.Params.Height
		}
		numSegments++

		go func(startY, endY int) {
			fmt.Printf("[DEBUG] Server preparing to process segment [%d-%d]\n", startY, endY)

			segReq := SegmentRequest{
				Start: startY,
				End:   endY,
				World: req.World,
				Params: Params{
					Width:  req.Params.Width,
					Height: req.Params.Height,
				},
			}

			worker, err := getAvailableWorker()
			if err != nil {
				fmt.Printf("[ERROR] Failed to retrieve worker: %v\n", err)
				errChan <- err
				return
			}

			fmt.Printf("[DEBUG] Sending segment [%d-%d] to worker for processing\n", startY, endY)
			var response SegmentResponse
			err = worker.Client.Call("Worker.State", segReq, &response)
			if err != nil {
				fmt.Printf("[ERROR] Worker failed to process segment [%d-%d]: %v\n", startY, endY, err)
				errChan <- err
				return
			}

			fmt.Printf("[DEBUG] Worker successfully completed segment [%d-%d]\n", startY, endY)
			segmentChan <- struct {
				startY  int
				endY    int
				segment [][]byte
			}{startY, endY, response.NewSegment}

		}(start, end)
	}

	fmt.Printf("[DEBUG] Waiting for %d segments to complete\n", numSegments)

	// Collect results
	for i := 0; i < numSegments; i++ {
		select {
		case err := <-errChan:
			fmt.Printf("[ERROR] Segment processing failed: %v\n", err)
			return err
		case segment := <-segmentChan:
			fmt.Printf("[DEBUG] Received computed segment [%d-%d]\n", segment.startY, segment.endY)
			// Copy segment results to the final result
			for y := segment.startY; y < segment.endY; y++ {
				copy((*res)[y], segment.segment[y-segment.startY])
			}
		}
	}

	fmt.Println("[DEBUG] All segments completed, returning final result")
	return nil
}

// calculateSegmentLocally processes a segment locally when no workers are available
func (e *Engine) calculateSegmentLocally(req SegmentRequest, res *SegmentResponse) {
	fmt.Printf("[DEBUG] Processing segment [%d-%d] locally\n", req.Start, req.End)

	// Create a new segment
	res.NewSegment = make([][]byte, req.End-req.Start)
	for i := range res.NewSegment {
		res.NewSegment[i] = make([]byte, req.Params.Width)
	}

	// Compute the next state for each cell in the segment
	for y := req.Start; y < req.End; y++ {
		for x := 0; x < req.Params.Width; x++ {
			aliveNeighbors := countAliveNeighbors(req.World, x, y)
			segY := y - req.Start

			if req.World[y][x] == 255 {
				// Rules for live cells
				if aliveNeighbors == 2 || aliveNeighbors == 3 {
					res.NewSegment[segY][x] = 255
				}
			} else {
				// Rules for dead cells
				if aliveNeighbors == 3 {
					res.NewSegment[segY][x] = 255
				}
			}
		}
	}
}

// countAliveNeighbors calculates the number of alive neighbors around a cell
func countAliveNeighbors(world [][]byte, x, y int) int {
	aliveCount := 0
	height := len(world)
	width := len(world[0])

	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			neighborX := (x + i + width) % width
			neighborY := (y + j + height) % height
			if world[neighborY][neighborX] == 255 {
				aliveCount++
			}
		}
	}
	return aliveCount
}

// RegisterWorker registers a new worker and verifies connectivity
func (e *Engine) RegisterWorker(workerAddr string, success *bool) error {
	worker := &Worker{
		Client: nil,
	}

	// Attempt to connect to the worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to worker: %v", err)
	}

	// Test if connection is healthy
	var pingRes bool
	err = client.Call("Worker.Ping", true, &pingRes)
	if err != nil {
		client.Close()
		return fmt.Errorf("Worker ping test failed: %v", err)
	}

	worker.Client = client

	workerMutex.Lock()
	workers = append(workers, worker)
	workerMutex.Unlock()

	*success = true
	fmt.Printf("[DEBUG] Worker registered successfully: %s (current worker count: %d)\n", workerAddr, len(workers))
	return nil
}

// startServer initializes and starts the RPC server
func startServer() {
	engine := new(Engine)
	rpc.Register(engine)

	fmt.Println("[DEBUG] Starting RPC server on port 8080")
	fmt.Println("[DEBUG] Registered RPC methods:")
	fmt.Println(" - Engine.State")
	fmt.Println(" - Engine.RegisterWorker")

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer listener.Close()

	fmt.Println("Server started, waiting for connections...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v\n", err)
			continue
		}
		fmt.Printf("[DEBUG] Accepted connection from %v\n", conn.RemoteAddr())
		go func(c net.Conn) {
			fmt.Printf("[DEBUG] Starting RPC session with %v\n", c.RemoteAddr())
			rpc.ServeConn(c)
			fmt.Printf("[DEBUG] Ending RPC session with %v\n", c.RemoteAddr())
		}(conn)
	}
}

func main() {
	startServer()
}
