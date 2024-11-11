package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
	"time"
)

// Worker struct holds worker details, including RPC client, address, and current load.
type Worker struct {
	Client *rpc.Client
	Addr   string
	Load   int
}

var (
	workers     = make([]*Worker, 0) // Worker pool to manage registered workers.
	workerMutex sync.Mutex           // Mutex to synchronize access to the worker pool.
)

// Params defines the game parameters like grid dimensions.
type Params struct {
	Width  int
	Height int
}

// SegmentRequest represents a request for segment calculation.
type SegmentRequest struct {
	Start  int      // Starting Y-coordinate of the segment.
	End    int      // Ending Y-coordinate of the segment.
	World  [][]byte // The world grid data.
	Params Params   // Parameters including grid dimensions.
}

// SegmentResponse represents the response after segment calculation.
type SegmentResponse struct {
	NewSegment [][]byte // The calculated new segment data.
}

// DistributedTask contains the world grid and parameters to be distributed to workers.
type DistributedTask struct {
	World  [][]byte // The world grid data.
	Params Params   // Parameters including grid dimensions.
}

// Engine is the main struct that handles incoming RPC calls.
type Engine struct{}

// getAvailableWorker selects the worker with the lowest load.
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	if len(workers) == 0 {
		return nil, fmt.Errorf("No available workers")
	}

	// Find the worker with the minimum load.
	selectedWorker := workers[0]
	for _, worker := range workers {
		if worker.Load < selectedWorker.Load {
			selectedWorker = worker
		}
	}
	selectedWorker.Load++
	log.Printf("[INFO] Selected worker at %s with current load %d", selectedWorker.Addr, selectedWorker.Load)
	return selectedWorker, nil
}

// releaseWorkerLoad decreases the load of a worker after task completion.
func releaseWorkerLoad(worker *Worker) {
	workerMutex.Lock()
	worker.Load--
	workerMutex.Unlock()
	log.Printf("[INFO] Worker at %s load reduced to %d", worker.Addr, worker.Load)
}

// distributeSegment assigns segment computation to the least-loaded worker.
func distributeSegment(startY, endY int, req DistributedTask, errChan chan<- error, segmentChan chan<- struct {
	startY  int
	endY    int
	segment [][]byte
}) {
	worker, err := getAvailableWorker()
	if err != nil {
		log.Printf("[ERROR] Failed to get available worker: %v", err)
		errChan <- err
		return
	}
	defer releaseWorkerLoad(worker)

	// Prepare the segment request.
	segReq := SegmentRequest{
		Start:  startY,
		End:    endY,
		World:  req.World,
		Params: req.Params,
	}

	var response SegmentResponse
	// Call the worker's State method to compute the segment.
	err = worker.Client.Call("Worker.State", segReq, &response)
	if err != nil {
		log.Printf("[ERROR] Worker at %s failed to process segment [%d-%d]: %v", worker.Addr, startY, endY, err)
		errChan <- err
		return
	}

	log.Printf("[INFO] Successfully processed segment [%d-%d] by worker at %s", startY, endY, worker.Addr)
	// Send the computed segment back through the channel.
	segmentChan <- struct {
		startY  int
		endY    int
		segment [][]byte
	}{startY, endY, response.NewSegment}
}

// collectResults collects all segment results from workers.
func collectResults(numSegments int, res *[][]byte, errChan <-chan error, segmentChan <-chan struct {
	startY  int
	endY    int
	segment [][]byte
}) error {
	log.Printf("[INFO] Waiting for %d segments to complete", numSegments)

	for i := 0; i < numSegments; i++ {
		select {
		case err := <-errChan:
			return fmt.Errorf("Segment processing failed: %v", err)
		case segment := <-segmentChan:
			log.Printf("[INFO] Received result for segment [%d-%d]", segment.startY, segment.endY)
			// Merge the segment data into the result grid.
			for y := segment.startY; y < segment.endY; y++ {
				copy((*res)[y], segment.segment[y-segment.startY])
			}
		case <-time.After(10 * time.Second):
			return fmt.Errorf("Timeout waiting for segment results")
		}
	}

	log.Println("[INFO] All segments processed successfully")
	return nil
}

// State calculates the new state for each segment of the world grid.
func (e *Engine) State(req DistributedTask, res *[][]byte) error {
	log.Printf("[INFO] Server received computation request: grid size %dx%d", req.Params.Width, req.Params.Height)

	if req.World == nil {
		return fmt.Errorf("World input is empty")
	}

	workerMutex.Lock()
	numWorkers := len(workers)
	workerMutex.Unlock()

	if numWorkers == 0 {
		return fmt.Errorf("No available workers")
	}

	log.Printf("[INFO] Preparing to distribute task to %d workers", numWorkers)

	// Determine the height of each segment.
	segmentHeight := req.Params.Height / numWorkers
	if segmentHeight < 1 {
		segmentHeight = 1
	}

	// Initialize the result grid.
	*res = make([][]byte, req.Params.Height)
	for i := range *res {
		(*res)[i] = make([]byte, req.Params.Width)
	}

	errChan := make(chan error, numWorkers)
	segmentChan := make(chan struct {
		startY  int
		endY    int
		segment [][]byte
	}, numWorkers)

	numSegments := 0
	// Distribute segments to workers.
	for start := 0; start < req.Params.Height; start += segmentHeight {
		end := start + segmentHeight
		if end > req.Params.Height {
			end = req.Params.Height
		}
		numSegments++
		go distributeSegment(start, end, req, errChan, segmentChan)
	}

	// Collect the results from workers.
	return collectResults(numSegments, res, errChan, segmentChan)
}

// RegisterWorker registers a worker and adds it to the worker pool.
func (e *Engine) RegisterWorker(workerAddr string, success *bool) error {
	log.Printf("[INFO] Registering new worker at address: %s", workerAddr)

	// Connect to the worker via RPC.
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("Failed to connect to worker at %s: %v", workerAddr, err)
	}

	// Perform a ping test to ensure the worker is responsive.
	var pingRes bool
	err = client.Call("Worker.Ping", true, &pingRes)
	if err != nil {
		client.Close()
		return fmt.Errorf("Worker at %s failed ping test: %v", workerAddr, err)
	}

	worker := &Worker{
		Client: client,
		Addr:   workerAddr,
		Load:   0,
	}

	// Add the worker to the worker pool.
	workerMutex.Lock()
	workers = append(workers, worker)
	workerMutex.Unlock()

	*success = true
	log.Printf("[INFO] Worker at %s registered successfully. Total workers: %d", workerAddr, len(workers))
	return nil
}

// handleConnection handles each incoming RPC connection.
func handleConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("[INFO] Handling RPC connection from %v", conn.RemoteAddr())
	rpc.ServeConn(conn)
	log.Printf("[INFO] Finished handling RPC connection from %v", conn.RemoteAddr())
}

func main() {
	engine := new(Engine)
	rpc.Register(engine)

	log.Println("[INFO] Starting the Distributed Engine RPC server on port 8080")

	// Listen for incoming connections on port 8080.
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("[ERROR] Failed to start the server: %v", err)
	}
	defer listener.Close()

	log.Println("[INFO] Server is up and running, waiting for client connections...")

	for {
		// Accept an incoming connection.
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[ERROR] Failed to accept a connection: %v", err)
			continue
		}
		log.Printf("[INFO] New client connected from %v", conn.RemoteAddr())
		// Handle the connection in a new goroutine.
		go handleConnection(conn)
	}
}
