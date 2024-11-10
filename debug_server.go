// server.go
package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

// Worker structure definition
type Worker struct {
	Client *rpc.Client
}

var (
	workers            = make([]*Worker, 0) // list of workers
	workerMutex        sync.Mutex           // mutex for worker access
	currentWorkerIndex int                  // index for round-robin selection
)

// Game parameters structure
type Params struct {
	Width  int
	Height int
}

// Segment calculation request structure
type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

// Segment calculation response structure
type SegmentResponse struct {
	NewSegment [][]byte
}

// Distributed task request
type DistributedTask struct {
	World  [][]byte
	Params struct {
		Width  int
		Height int
	}
}

// Engine structure
type Engine struct{}

// Get an available worker
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	numWorkers := len(workers)
	if numWorkers == 0 {
		return nil, fmt.Errorf("no available worker")
	}

	// Select worker using round-robin method
	worker := workers[currentWorkerIndex]
	currentWorkerIndex = (currentWorkerIndex + 1) % numWorkers

	fmt.Printf("[debug] Selected worker %d / %d\n", currentWorkerIndex, numWorkers)
	return worker, nil
}

// Calculate the state of a single segment
func (e *Engine) State(req DistributedTask, res *[][]byte) error {
	fmt.Printf("[debug] Server received calculation request: grid size %dx%d\n",
		req.Params.Width, req.Params.Height)

	// Input validation
	if req.World == nil {
		return fmt.Errorf("input world is nil")
	}

	if len(req.World) != req.Params.Height {
		return fmt.Errorf("world height mismatch: expected %d, got %d",
			req.Params.Height, len(req.World))
	}

	for i, row := range req.World {
		if len(row) != req.Params.Width {
			return fmt.Errorf("row %d width mismatch: expected %d, got %d",
				i, req.Params.Width, len(row))
		}
	}

	workerMutex.Lock()
	numWorkers := len(workers)
	workerMutex.Unlock()

	if numWorkers == 0 {
		return fmt.Errorf("no available worker")
	}

	fmt.Printf("[debug] Preparing to assign task to %d workers\n", numWorkers)

	// Calculate the number of rows each worker should handle
	segmentHeight := req.Params.Height / numWorkers
	if segmentHeight < 1 {
		segmentHeight = 1
	}

	// Create the result array
	*res = make([][]byte, req.Params.Height)
	for i := range *res {
		(*res)[i] = make([]byte, req.Params.Width)
	}

	// Use channels to collect errors and completed segments
	errChan := make(chan error, numWorkers)
	segmentChan := make(chan struct {
		startY  int
		endY    int
		segment [][]byte
	}, numWorkers)

	numSegments := 0
	// Create segments and assign work
	for start := 0; start < req.Params.Height; start += segmentHeight {
		end := start + segmentHeight
		if end > req.Params.Height {
			end = req.Params.Height
		}
		numSegments++

		go func(startY, endY int) {
			fmt.Printf("[debug] Server preparing to handle segment [%d-%d]\n", startY, endY)

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
				fmt.Printf("[error] Failed to get worker: %v\n", err)
				errChan <- err
				return
			}

			fmt.Printf("[debug] Sending segment [%d-%d] to worker for processing\n", startY, endY)
			var response SegmentResponse
			err = worker.Client.Call("Worker.State", segReq, &response)
			if err != nil {
				fmt.Printf("[error] Worker failed to process segment [%d-%d]: %v\n", startY, endY, err)
				errChan <- err
				return
			}

			fmt.Printf("[debug] Worker successfully processed segment [%d-%d]\n", startY, endY)
			segmentChan <- struct {
				startY  int
				endY    int
				segment [][]byte
			}{startY, endY, response.NewSegment}

		}(start, end)
	}

	fmt.Printf("[debug] Waiting for %d segments to complete\n", numSegments)

	// Collect results
	for i := 0; i < numSegments; i++ {
		select {
		case err := <-errChan:
			fmt.Printf("[error] Segment processing failed: %v\n", err)
			return err
		case segment := <-segmentChan:
			fmt.Printf("[debug] Received calculation result for segment [%d-%d]\n", segment.startY, segment.endY)
			// Copy segment results to final result
			for y := segment.startY; y < segment.endY; y++ {
				copy((*res)[y], segment.segment[y-segment.startY])
			}
		}
	}

	fmt.Println("[debug] All segments processed, returning final result")
	return nil
}

// Register a worker
func (e *Engine) RegisterWorker(workerAddr string, success *bool) error {
	worker := &Worker{
		Client: nil,
	}

	// Attempt to connect to the worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to worker: %v", err)
	}

	// Test if the connection works
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
	fmt.Printf("[debug] Worker registered successfully: %s (current worker count: %d)\n", workerAddr, len(workers))
	return nil
}

// Start the RPC server
func startServer() {
	engine := new(Engine)
	rpc.Register(engine)

	fmt.Println("[debug] Starting RPC server, port: 8080")
	fmt.Println("[debug] Registered RPC methods:")
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
		fmt.Printf("[debug] Accepted connection from %v\n", conn.RemoteAddr())
		go func(c net.Conn) {
			fmt.Printf("[debug] Starting to handle RPC connection from %v\n", c.RemoteAddr())
			rpc.ServeConn(c)
			fmt.Printf("[debug] Finished handling RPC connection from %v\n", c.RemoteAddr())
		}(conn)
	}
}

func main() {
	startServer()
}
