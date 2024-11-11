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

func makeMatrix(height, width int) [][]byte {
	matrix := make([][]byte, height)
	for i := range matrix {
		matrix[i] = make([]byte, width)
	}
	return matrix
}

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

func calculateNeighbours(width, height int, world [][]byte, x, y int) int {
	neighbours := 0
	// Define relative coordinate offsets for neighbors
	offsets := [8][2]int{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	for _, offset := range offsets {
		newX := (x + offset[0] + width) % width
		newY := (y + offset[1] + height) % height
		if world[newY][newX] == 255 {
			neighbours++
		}
	}
	return neighbours
}

// determineCellState returns the new state based on current state and number of neighbours
func determineCellState(wasAlive bool, neighbours int) byte {
	if wasAlive {
		if neighbours == 2 || neighbours == 3 {
			return 255 // Remain alive
		}
		return 0 // Die
	} else {
		if neighbours == 3 {
			return 255 // Become alive
		}
	}
	return 0 // Remain dead
}

// updateWorld updates to the next state based on the current world state, while recording cells that have changed
func updateWorld(width, height int, world [][]byte) ([][]byte, []util.Cell) {
	newWorld := makeMatrix(height, width) // New world matrix
	var flippedCells []util.Cell          // Record positions of flipped cells

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			neighbours := calculateNeighbours(width, height, world, x, y)
			wasAlive := world[y][x] == 255
			newState := determineCellState(wasAlive, neighbours)

			// If state changes, record this cell flip
			if newState != world[y][x] {
				flippedCells = append(flippedCells, util.Cell{X: x, Y: y})
			}
			newWorld[y][x] = newState
		}
	}
	return newWorld, flippedCells
}

// calculateNextState computes the next world state
func calculateNextState(width, height int, world [][]byte) ([][]byte, []util.Cell) {
	return updateWorld(width, height, world)
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

// Output PGM file
func outputPGM(c distributorChannels, p Params, filename chan string, output chan<- uint8, world [][]byte, turn int) {
	c.ioCommand <- ioOutput
	file := fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, turn)
	filename <- file
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			output <- world[y][x]
		}
	}

	// Wait for I/O to complete
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{turn, file}
	fmt.Printf("Image saved as %s.pgm\n", file)
}

// Function to shut down all distributed components cleanly
func shutDownDistributedComponents() {
	fmt.Println("Shutting down distributed components...")
	fmt.Println("All distributed components have been shut down successfully.")
}

func distributor(p Params, c distributorChannels, input <-chan uint8, output chan<- uint8, filename chan string, keyPresses <-chan rune, engineAddress string) {

	isShutDown := false

	// Attempt to connect to the distributed server
	fmt.Println("Starting to connect to the distributed server... hope the network is in good condition.")
	client, err := rpc.Dial("tcp", engineAddress)
	var useDistributed bool
	if err != nil {
		fmt.Printf("Oops! Couldn't connect to the distributed server: %v\nSwitching to local computation.\n", err)
		useDistributed = false
	} else {
		defer client.Close()
		useDistributed = true
		fmt.Println("Successfully connected to the distributed server.")

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
		err = client.Call("Engine.State", testTask, &dummy)
		if err != nil {
			fmt.Printf("Hmm, the RPC test call failed: %v\n", err)
			useDistributed = false
		} else {
			fmt.Println("Great! RPC test call succeeded.")
		}
	}

	world := makeMatrix(p.ImageHeight, p.ImageWidth)

	c.ioCommand <- ioInput
	file := fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	filename <- file
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-input
		}
	}

	// For all initially alive cells, send a CellFlipped Event.
	turn := 0
	aliveCells := calculateAliveCells(p, world)
	for _, cell := range aliveCells {
		c.events <- CellFlipped{
			CompletedTurns: turn,
			Cell:           cell,
		}
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	handleShutdown := func() {
		if !isShutDown {
			fmt.Println("Initiating shutdown sequence... please wait.")
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
			fmt.Printf("Turn %d: Currently, there are %d alive cells.\n", turn, len(alive))
			c.events <- AliveCellsCount{turn, len(alive)}
		case key := <-keyPresses:
			switch key {
			case 'q':
				if !isShutDown {
					fmt.Println("Quit command received. Exiting gracefully...")
					outputPGM(c, p, filename, output, world, turn)
					aliveCells := calculateAliveCells(p, world)
					c.events <- FinalTurnComplete{turn, aliveCells}
					c.events <- StateChange{turn, Quitting}
					close(c.events)
					return
				}
			case 's':
				if !isShutDown {
					fmt.Println("Save command received. Saving current state...")
					outputPGM(c, p, filename, output, world, turn)
				}
			case 'k':
				handleShutdown()
				break
			case 'p':
				if !isShutDown {
					c.events <- StateChange{turn, Paused}
					fmt.Printf("Paused at turn %d. Press 'p' again to resume.\n", turn)
					for {
						key = <-keyPresses
						if key == 'p' {
							c.events <- StateChange{turn, Executing}
							fmt.Println("Resuming execution...")
							break
						} else if key == 'q' {
							fmt.Println("Quit command received during pause. Exiting...")
							outputPGM(c, p, filename, output, world, turn)
							aliveCells := calculateAliveCells(p, world)
							c.events <- FinalTurnComplete{turn, aliveCells}
							c.events <- StateChange{turn, Quitting}
							close(c.events)
							return
						} else if key == 'k' {
							handleShutdown()
							break
						} else if key == 's' {
							fmt.Println("Save command received during pause. Saving current state...")
							outputPGM(c, p, filename, output, world, turn)
						}
					}
				}
			}

		default:
			if !isShutDown {
				var newWorld [][]byte
				var flipped []util.Cell

				if useDistributed {
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
					if err != nil {
						fmt.Printf("Oh no! Distributed computation failed: %v\nSwitching to local computation.\n", err)
						useDistributed = false
						newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
					} else {
						if response == nil {
							fmt.Println("Hmm, the server returned an empty result. Switching to local computation.")
							useDistributed = false
							newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
						} else {
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

	aliveCells = calculateAliveCells(p, world)
	c.events <- FinalTurnComplete{turn, aliveCells}
	outputPGM(c, p, filename, output, world, turn)

	// Ensure all I/O operations are complete
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	fmt.Println("All done! Exiting now!")
	close(c.events)
}
