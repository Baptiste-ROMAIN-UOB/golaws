// server.go
package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

// Worker holds an RPC client to communicate with a worker node.
type Worker struct {
	Client *rpc.Client
}

// Worker management variables.
var (
	workers            = make([]*Worker, 0) // Connected worker nodes
	workerMutex        sync.Mutex           // Controls access to workers list
	currentWorkerIndex int                  // Round-robin index for task assignment
)

// Params defines the grid dimensions for the task.
type Params struct {
	Width  int
	Height int
}

// SegmentRequest represents a part of the world grid to be processed.
type SegmentRequest struct {
	Start  int      // Start row of the segment
	End    int      // End row of the segment
	World  [][]byte // Grid segment data
	Params Params   // Grid dimensions
}

// SegmentResponse holds the processed segment data.
type SegmentResponse struct {
	NewSegment [][]byte // Processed grid segment
}

// DistributedTask represents the complete task with grid data and dimensions.
type DistributedTask struct {
	World  [][]byte
	Params struct {
		Width  int
		Height int
	}
}

// Engine is the main server for managing task distribution.
type Engine struct{}

// getAvailableWorker selects a worker in round-robin order.
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	numWorkers := len(workers)
	if numWorkers == 0 {
		return nil, fmt.Errorf("no available worker")
	}

	worker := workers[currentWorkerIndex]
	currentWorkerIndex = (currentWorkerIndex + 1) % numWorkers

	return worker, nil
}

// State divides the grid and assigns segments to workers for processing.
func (e *Engine) State(req DistributedTask, res *[][]byte) error {
	if req.World == nil || len(req.World) != req.Params.Height {
		return fmt.Errorf("invalid world grid input")
	}

	for i, row := range req.World {
		if len(row) != req.Params.Width {
			return fmt.Errorf("row %d width mismatch", i)
		}
	}

	workerMutex.Lock()
	numWorkers := len(workers)
	workerMutex.Unlock()

	if numWorkers == 0 {
		return fmt.Errorf("no available worker")
	}

	segmentHeight := req.Params.Height / numWorkers
	if segmentHeight < 1 {
		segmentHeight = 1
	}

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
	for start := 0; start < req.Params.Height; start += segmentHeight {
		end := start + segmentHeight
		if end > req.Params.Height {
			end = req.Params.Height
		}
		numSegments++

		go func(startY, endY int) {
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
				errChan <- err
				return
			}

			var response SegmentResponse
			err = worker.Client.Call("Worker.State", segReq, &response)
			if err != nil {
				errChan <- err
				return
			}

			segmentChan <- struct {
				startY  int
				endY    int
				segment [][]byte
			}{startY, endY, response.NewSegment}

		}(start, end)
	}

	for i := 0; i < numSegments; i++ {
		select {
		case err := <-errChan:
			return err
		case segment := <-segmentChan:
			for y := segment.startY; y < segment.endY; y++ {
				copy((*res)[y], segment.segment[y-segment.startY])
			}
		}
	}

	return nil
}

// RegisterWorker registers a new worker and verifies connectivity.
func (e *Engine) RegisterWorker(workerAddr string, success *bool) error {
	worker := &Worker{
		Client: nil,
	}

	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to worker: %v", err)
	}

	var pingRes bool
	err = client.Call("Worker.Ping", true, &pingRes)
	if err != nil {
		client.Close()
		return fmt.Errorf("worker ping test failed: %v", err)
	}

	worker.Client = client

	workerMutex.Lock()
	workers = append(workers, worker)
	workerMutex.Unlock()

	*success = true
	return nil
}

// startServer initializes the server and listens on port 8080.
func startServer() {
	engine := new(Engine)
	rpc.Register(engine)

	fmt.Println("Starting RPC server on port 8080")

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("connection error: %v\n", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// main launches the server.
func main() {
	startServer()
}
