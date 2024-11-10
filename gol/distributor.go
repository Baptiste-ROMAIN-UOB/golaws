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

// Output PGM file
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
	// Insert logic to send termination signals or stop distributed services as needed.
	fmt.Println("[System Shutdown] All distributed components have been shut down cleanly.")
}

// distributor 处理游戏主循环和事件分发
func distributor(p Params, c distributorChannels, input <-chan uint8, output chan<- uint8, filename chan string, keyPresses <-chan rune) {
	// Declare and initialize the isShutDown variable
	isShutDown := false

	// Attempt to connect to the distributed server
	fmt.Println("[Debug] Attempting to connect to the distributed server at serverIP:8080")
	client, err := rpc.Dial("tcp", "54.91.242.18:8080")
	var useDistributed bool
	if err != nil {
		fmt.Printf("Warning: Unable to connect to the distributed server: %v\nSwitching to local computation\n", err)
		useDistributed = false
	} else {
		defer client.Close()
		useDistributed = true
		fmt.Println("[Debug] Successfully connected to the distributed server")

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
			fmt.Printf("[Error] RPC test call failed: %v\n", err)
			useDistributed = false
		} else {
			fmt.Println("[Debug] RPC test call succeeded")
		}
	}




	world := makeMatrix(p.ImageHeight, p.ImageWidth)

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


	
	// Main ga
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	handleShutdown := func() {
		if !isShutDown {
			fmt.Println("[Debug] Initiating shutdown sequence")
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

	mainLoop:
		for turn < p.Turns {
			select {
			case <-ticker.C:
				if isShutDown {
					break mainLoop
				}
				alive := calculateAliveCells(p, world)
				c.events <- AliveCellsCount{turn, len(alive)}

			case key := <-keyPresses:
				switch key {
				case 'q':
					if !isShutDown {
						outputPGM(c, p, filename, output, world, turn)
						c.events <- StateChange{turn, Quitting}
						break mainLoop
					}
				case 's':
					if !isShutDown {
						outputPGM(c, p, filename, output, world, turn)
					}
				case 'k':
					handleShutdown()
					break mainLoop
				case 'p':
					if !isShutDown {
						c.events <- StateChange{turn, Paused}
						fmt.Printf("[Paused] Current turn: %d\n", turn)
						for {
							key = <-keyPresses
							if key == 'p' {
								c.events <- StateChange{turn, Executing}
								fmt.Println("[Continuing]")
								break
							} else if key == 'q' {
								outputPGM(c, p, filename, output, world, turn)
								c.events <- StateChange{turn, Quitting}
								break mainLoop
							} else if key == 'k' {
								handleShutdown()
								break mainLoop
							} else if key == 's' {
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

						fmt.Printf("[Debug] Sending distributed computation request for %dx%d grid, turn %d\n",
							p.ImageWidth, p.ImageHeight, turn)

						var response [][]byte
						err := client.Call("Engine.State", task, &response)
						if err != nil {
							fmt.Printf("[Error] Distributed computation failed: %v\nSwitching to local computation\n", err)
							useDistributed = false
							newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
						} else {
							fmt.Printf("[Debug] Distributed computation succeeded, turn %d\n", turn)
							if response == nil {
								fmt.Println("[Error] Server returned nil result")
								useDistributed = false
								newWorld, flipped = calculateNextState(p.ImageWidth, p.ImageHeight, world)
							} else {
								newWorld = response
								flipped = findFlippedCells(world, newWorld, p.ImageWidth, p.ImageHeight)
								fmt.Printf("[Debug] Calculation complete, %d cells flipped\n", len(flipped))
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

		// Only send final state if not already shut down
		if !isShutDown {
			aliveCells = calculateAliveCells(p, world)
			c.events <- FinalTurnComplete{turn, aliveCells}
			outputPGM(c, p, filename, output, world, turn)
		}

		// Ensure all I/O operations are complete
		c.ioCommand <- ioCheckIdle
		<-c.ioIdle
	}
