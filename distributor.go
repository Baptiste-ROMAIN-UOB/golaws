package gol

import (
	"fmt"
	//"strconv"
	"time"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events    chan<- Event
	ioCommand chan<- ioCommand
	ioIdle    <-chan bool
}

type WorkerRequest struct {
	Start   int      `json:"start"`
	End     int      `json:"end"`
	World   [][]byte `json:"world"`
	Params  Params   `json:"params"`
}
// Fonction askWorker qui envoie une requête à AWS et attend la réponse
func askWorker(requestData WorkerRequest) ([][]byte, error) {
	// URL de ton serveur sur l'instance EC2
	url := "http://184.72.165.154:8080/worker"

	// Conversion des données en JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("Erreur de conversion en JSON: %v", err)
	}

	// Envoi de la requête POST
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("Erreur lors de l'envoi de la requête: %v", err)
	}
	defer resp.Body.Close()

	// Vérification de la réponse
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Erreur du serveur: %s", resp.Status)
	}

	// Lire la réponse du serveur
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Erreur de lecture de la réponse: %v", err)
	}

	// Décode la réponse en JSON et retourne les résultats
	var newWorld [][]byte
	err = json.Unmarshal(body, &newWorld)
	if err != nil {
		return nil, fmt.Errorf("Erreur de décodage de la réponse: %v", err)
	}

	return newWorld, nil
}

// Distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, input <-chan uint8, output chan<- uint8, filename chan string, keyPresses <-chan rune) {

	// Créer une slice 2D pour stocker le monde.
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	// Recevoir les données d'entrée.
	c.ioCommand <- ioInput
	myfile := fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	filename <- myfile
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-input
		}
	}

	// Pour toutes les cellules initialement vivantes, envoyer un événement CellFlipped.
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

	// Diviser le travail entre les workers
	sectionDone := make(chan bool, p.Threads)
	sectionResults := make([]chan [][]byte, p.Threads)
	for i := 0; i < p.Threads; i++ {
		sectionResults[i] = make(chan [][]byte, 1)
	}
	segmentWidth := p.ImageWidth / p.Threads
	extraWidth := p.ImageWidth % p.Threads
	ticker := time.NewTicker(2 * time.Second)

	for turn < p.Turns {
		select {
		case <-ticker.C:
			cells := len(calculateAliveCells(p, world))
			c.events <- AliveCellsCount{turn, cells}
		default:
			select {
			case key := <-keyPresses:
				switch key {
				case 's':
					makePGM(p, filename, c, turn, output, world)
				case 'q':
					makePGM(p, filename, c, turn, output, world)
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					c.events <- StateChange{turn, Quitting}
					time.Sleep(500 * time.Millisecond)
					close(c.events)
				case 'p':
					fmt.Println("Game paused on turn:", turn)
					for i := 0; i == 0; {
						select {
						case press := <-keyPresses:
							if press == 'p' {
								i = 1
								break
							}
						default:
							time.Sleep(100 * time.Millisecond)
						}
					}
				}
			default:
				// Demander le calcul à AWS au lieu d'exécuter le worker localement
				for i := 0; i < p.Threads-1; i++ {
					go func(i int) {
						requestData := WorkerRequest{
							Start:   i * segmentWidth,
							End:     (i + 1) * segmentWidth,
							World:   world,
							Params:  p,
						}
						newWorld, err := askWorker(requestData)
						if err != nil {
							fmt.Printf("Erreur de requête à AWS pour la section %d: %v\n", i, err)
							return
						}

						sectionResults[i] <- newWorld
						sectionDone <- true
					}(i)
				}

				// Dernier worker (avec largeur supplémentaire si nécessaire)
				go func(i int) {
					requestData := WorkerRequest{
						Start:   i * segmentWidth,
						End:     (i + 1) * segmentWidth + extraWidth,
						World:   world,
						Params:  p,
					}
					newWorld, err := askWorker(requestData)
					if err != nil {
						fmt.Printf("Erreur de requête à AWS pour la section %d: %v\n", i, err)
						return
					}

					sectionResults[i] <- newWorld
					sectionDone <- true
				}(p.Threads - 1)

				// Attendre que tous les workers terminent
				for i := 0; i < p.Threads; i++ {
					<-sectionDone
				}

				// Combiner les résultats des workers
				for i := 0; i < p.Threads-1; i++ {
					newThreadSlice := <-sectionResults[i]
					for y := 0; y < p.ImageHeight; y++ {
						for x := i * segmentWidth; x < (i+1)*segmentWidth; x++ {
							world[y][x] = newThreadSlice[y][x]
						}
					}
				}
				newThreadSlice := <-sectionResults[p.Threads-1]
				for y := 0; y < p.ImageHeight; y++ {
					for x := (p.Threads-1)*segmentWidth; x < (p.Threads)*segmentWidth+extraWidth; x++ {
						world[y][x] = newThreadSlice[y][x]
					}
				}

				// Calculer les nouvelles cellules vivantes
				aliveCells = calculateAliveCells(p, world)
				cells := difference(aliveCells, calculateAliveCells(p, world))

				for i := range cells {
					c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: cells[i].X, Y: cells[i].Y}}
				}

				// Incrémenter le tour et envoyer l'événement
				turn++
				c.events <- TurnComplete{CompletedTurns: turn}
			}
		}
	}

	// Finaliser l'état
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: aliveCells}
	makePGM(p, filename, c, turn, output, world)

	// Assurer que le système I/O a terminé avant de quitter
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Changer l'état pour quitter
	c.events <- StateChange{turn, Quitting}

	// Fermer les canaux pour arrêter la goroutine SDL
	close(c.events)
}



func countAliveNeighbors(world [][]byte, x, y int) int {
	aliveCount := 0
	directions := []struct{ dx, dy int }{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1},           {0, 1},
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

func calculateNextState(start, end int, p Params, world [][]byte) [][]byte {
	newWorld := make([][]byte, p.ImageHeight)
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageWidth)
	}
	for y := 0; y < p.ImageHeight; y++ {
		for x := start; x < end; x++ {
			neighbours := countAliveNeighbors(world, x , y)
			if world[y][x] == 255 {
				if neighbours == 2 || neighbours == 3 {
					newWorld[y][x] = 255
				} else {
					newWorld[y][x] = 0
				}
			} else {
				if neighbours == 3 {
					newWorld[y][x] = 255
				} else {
					newWorld[y][x] = 0
				}
			}
		}
	}
	return newWorld
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


func makePGM(p Params, filename chan string, c distributorChannels, turn int, output chan<- uint8, world [][]byte) {
	filenames := fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, turn)
	c.ioCommand <- ioOutput
	filename <- filenames
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			output <- world[y][x]
		}
	}
}

// difference betwen cells in a but not b, et reverse.
func difference(a, b []util.Cell) []util.Cell {

	cellSet := make(map[util.Cell]struct{})

	for _, cell := range b {
		cellSet[cell] = struct{}{}
	}


    var result []util.Cell

	for _, cell := range a {
		if _, found := cellSet[cell]; !found {
			result = append(result, cell)
		}
	}

	for _, cell := range b {
		if _, found := cellSet[cell]; found {
			continue
		}
		result = append(result, cell)
	}

	return result
}



func worker(start, end int, turnDone chan<- bool, sectionResults chan<- [][]byte, world [][]byte, p Params) {
	newWorld := calculateNextState(start, end, p, world)
	sectionResults <- newWorld
	turnDone <- true
 }
