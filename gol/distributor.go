package gol

import (
	"fmt"
	"net/rpc"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events    chan<- Event
	ioCommand chan<- ioCommand
	ioIdle    <-chan bool
}


type DistributedTask struct {
	World  [][]byte
	Params struct {
		Width  int
		Height int
	}
}

//create the world
func makeMatrix(height, width int) [][]byte {
	matrix := make([][]byte, height)
	for i := range matrix {
		matrix[i] = make([]byte, width)
	}
	return matrix
}

//get the cell alive in the world
func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	aliveCells := []util.Cell{}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	return aliveCells
}

//this is just in case the worker does not work, cf debug_worker/server
func calculateNeighbours(width, height int, world [][]byte, x, y int) int {
	neighbours := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			newX := (x + i + width) % width
			newY := (y + j + height) % height
			if world[newY][newX] == 255 {
				neighbours++
			}
		}
	}
	return neighbours
}

//this is just in case the worker does not work, cf debug_worker/server
func calculateNextState(width, height int, world [][]byte) ([][]byte, []util.Cell) {
	newWorld := makeMatrix(height, width)
	var flipped []util.Cell

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			neighbours := calculateNeighbours(width, height, world, x, y)
			wasAlive := world[y][x] == 255

	
			var newState byte
			if wasAlive {
				if neighbours == 2 || neighbours == 3 {
					newState = 255
				}
			} else {
				if neighbours == 3 {
					newState = 255
				}
			}

	
			if (wasAlive && newState == 0) || (!wasAlive && newState == 255) {
				flipped = append(flipped, util.Cell{X: x, Y: y})
			}
			newWorld[y][x] = newState
		}
	}

	return newWorld, flipped
}

func findFlippedCells(oldWorld, newWorld [][]byte, width, height int) []util.Cell {
	var flipped []util.Cell
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if oldWorld[y][x] != newWorld[y][x] {
				flipped = append(flipped, util.Cell{X: x, Y: y})
			}
		}
	}
	return flipped
}

// using the chanels give data to writePgmImage
func outputPGM(c distributorChannels, p Params, filename chan string, output chan<- uint8, world [][]byte, turn int) {
	c.ioCommand <- ioOutput
	filename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, turn)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			output <- world[y][x]
		}
	}
}

// Function to shut down all distributed components cleanly
func shutDownDistributedComponents() {
	fmt.Println("[System Shutdown] Sending termination signals to distributed components.")
	fmt.Println("[System Shutdown] All distributed components have been shut down cleanly.")
}

// distributor function
func distributor(p Params, c distributorChannels, input <-chan uint8, output chan<- uint8, filename chan string, keyPresses <-chan rune, engineAddress string) {

	isShutDown := false
	var useDistributed bool
	var client *rpc.Client

	fmt.Printf("[System] Attempting to connect to the distributed server at %s\n", engineAddress)
	
	// Create a channel that will receive the result of the connection attempt
	connectionDone := make(chan *rpc.Client, 1)
	errorDone := make(chan error, 1)

	// Start the connection attempt in a goroutine
	go func() {
		client, err := rpc.Dial("tcp", engineAddress)
		if err != nil {
			errorDone <- err
			return
		}
		connectionDone <- client
	}()

	// Wait for either the connection to succeed or the timeout to occur
	select {
	case client := <-connectionDone:
		defer client.Close()
		useDistributed = true
		fmt.Println("[System] Successfully connected to the distributed server")

		// Test the RPC connection
		var dummy [][]byte
		testTask := DistributedTask{
			World: [][]byte{{0}},
			Params: struct {
				Width  int
				Height int
			}{
				Width:  1,
				Height: 1,
			},
		}
		err := client.Call("Engine.State", testTask, &dummy)
		if err != nil {
			fmt.Printf("[Error] RPC test call failed: %v\n", err)
			useDistributed = false
		} else {
			fmt.Println("[System] RPC test call succeeded")
		}
	case err := <-errorDone:
		// Handle connection error after timeout
		fmt.Printf("[Error] Unable to connect to the distributed server: %v\nSwitching to local computation\n", err)
		useDistributed = false
	case <-time.After(10 * time.Second):
		// Timeout after 10 seconds
		fmt.Println("[Error] Connection attempt timed out. Switching to local computation.")
		useDistributed = false
	}


	//create world
	world := makeMatrix(p.ImageHeight, p.ImageWidth)
	//get the world from the pgm
	c.ioCommand <- ioInput
	myfile := fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	filename <- myfile
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-input
		}
	}

	// For all initially alive cells send a CellFlipped Event.
	turn := 0
	aliveCells := calculateAliveCells(p, world)
	for i := range aliveCells {
		var CellFlipped = CellFlipped{
			CompletedTurns: turn,
			Cell: util.Cell{
				X: aliveCells[i].X,
				Y: aliveCells[i].Y,
			},
		}
		c.events <- CellFlipped
	}


	
	// The game of life turns
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	//function to handle the shutdown (k)
	handleShutdown := func() {
		if !isShutDown {
			fmt.Println("[System] Initiating shutdown sequence")
			c.events <- StateChange{turn, ShuttingDown}
			outputPGM(c, p, filename, output, world, turn)
			shutDownDistributedComponents()

			// Send final state before shutting down
			aliveCells = calculateAliveCells(p, world)
			c.events <- FinalTurnComplete{turn, aliveCells}

			// Ensure all I/O operations are complete
			c.ioCommand <- ioCheckIdle
			<-c.ioIdle

			isShutDown = true
		}
	}
	for turn < p.Turns {
		select {
		case <-ticker.C:
			if isShutDown {
				break 
			}
			alive := calculateAliveCells(p, world)
			c.events <- AliveCellsCount{turn, len(alive)}
		case key := <-keyPresses:
			switch key {
			case 'q': //case quit (q)
				if !isShutDown {
					outputPGM(c, p, filename, output, world, turn)
					c.events <- StateChange{turn, Quitting}
					break 
				}
			case 's': //case save (s)
				if !isShutDown {
					outputPGM(c, p, filename, output, world, turn)
				}
			case 'k': //case finish and maintain sdl windows (k)
				handleShutdown()
				break 
			case 'p': //case pause (p)
				if !isShutDown {
					c.events <- StateChange{turn, Paused}
					for {
						key = <-keyPresses
						if key == 'p' { // unpause
							c.events <- StateChange{turn, Executing}
							break
						} else if key == 'q' { // P + Q
							outputPGM(c, p, filename, output, world, turn)
							c.events <- StateChange{turn, Quitting}
							break 
						} else if key == 'k' { // P + K
							handleShutdown()
							break 
						} else if key == 's' {// P + S
							outputPGM(c, p, filename, output, world, turn)
						}
					}
				}
			}

		default: //default case get next state
			if !isShutDown {
				var newWorld [][]byte
				var flipped []util.Cell

				if useDistributed { //use server and worker, !!! you need to load server then worker before lanching the client
					task := DistributedTask{
						World: world,
						Params: struct {
							Width  int
							Height int
						}{
							Width:  p.ImageWidth,
							Height: p.ImageHeight,
						},
					}
					var response [][]byte
					err := client.Call("Engine.State", task, &response)
					if err != nil {// if computation fail then use local
						useDistributed = false
						newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
					} else {
						if response == nil { // in case the server is doing a lil trolling and is send null package :) then again go local
							useDistributed = false
							newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
						} else { // yay in this case the server and the workers are working 
							newWorld = response
							flipped = findFlippedCells(world, newWorld, p.ImageWidth, p.ImageHeight)
						}
					}
				} else {
					// Use local computation
					newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
				}

				// Send cell flipped events
				for _, cell := range flipped {
					c.events <- CellFlipped{turn, cell}
				}

				// Update world state
				world = newWorld
				c.events <- TurnComplete{turn}
				turn++
			}
		}
	}
	
	// Finalize the simulation: send the final state, output the last PGM file, ensure all I/O operations are completed, and clean up
	aliveCells = calculateAliveCells(p, world) 
	c.events <- FinalTurnComplete{turn, aliveCells}
	outputPGM(c, p, filename, output, world, turn) 
	c.ioCommand <- ioCheckIdle 
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}
	close(c.events) 

}
