package gol

// Params provides the configuration for running the Game of Life and which image to load.
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// ioCommand is an enumerated type representing various commands for I/O operations.
type ioCommand uint8

// Enumeration of possible I/O commands.
const (
	ioOutput ioCommand = iota  // Command to output data.
	ioInput                    // Command to input data.
	ioCheckIdle                // Command to check if I/O is idle.
)

// ioChannels holds all channels used for communication with the I/O goroutine.
type ioChannels struct {
	command  chan ioCommand // Channel for I/O commands.
	idle     chan bool      // Channel to signal if I/O is idle.
	filename chan string    // Channel for filenames.
	output   chan uint8     // Channel for output data.
	input    chan uint8     // Channel for input data.
}

// Run initiates the Game of Life processing with distributed computation.
// p: Game parameters, events: Channel to send game events, keyPresses: Channel for key input commands,
// engineAddress: Address of the engine for distributed computation.
func Run(p Params, events chan<- Event, keyPresses <-chan rune) {
    address := "localhost:8080"
 
	// Initialize channels for I/O operations.
	ioCommand := make(chan ioCommand)
	ioIdle := make(chan bool)
	ioFilename := make(chan string)
	ioOutput := make(chan uint8)
	ioInput := make(chan uint8)

	ioChannels := ioChannels{
		command:  ioCommand,
		idle:     ioIdle,
		filename: ioFilename,
		output:   ioOutput,
		input:    ioInput,
	}

	// Channels required for the distributor's operations.
	distributorChannels := distributorChannels{
		events:    events,
		ioCommand: ioCommand,
		ioIdle:    ioIdle,
	}

	// Start the I/O goroutine with the provided parameters and channels.
	go startIo(p, ioChannels)

	// Start the distributor which handles the main game logic, using the provided channels and parameters.
	distributor(p, distributorChannels, ioInput, ioOutput, ioFilename, keyPresses, address)
}

func Run_Adress(p Params, events chan<- Event, keyPresses <-chan rune, engineAddress *string) {
    address := "localhost:8080" // Adresse par défaut
    if engineAddress != nil {
        address = *engineAddress
    }
	// Initialize channels for I/O operations.
	ioCommand := make(chan ioCommand)
	ioIdle := make(chan bool)
	ioFilename := make(chan string)
	ioOutput := make(chan uint8)
	ioInput := make(chan uint8)

	ioChannels := ioChannels{
		command:  ioCommand,
		idle:     ioIdle,
		filename: ioFilename,
		output:   ioOutput,
		input:    ioInput,
	}

	// Channels required for the distributor's operations.
	distributorChannels := distributorChannels{
		events:    events,
		ioCommand: ioCommand,
		ioIdle:    ioIdle,
	}

	// Start the I/O goroutine with the provided parameters and channels.
	go startIo(p, ioChannels)

	// Start the distributor which handles the main game logic, using the provided channels and parameters.
	distributor(p, distributorChannels, ioInput, ioOutput, ioFilename, keyPresses, address)
}
// NewParams creates a new Params instance with the specified parameters.
// turns: Number of game turns, threads: Number of threads for computation,
// imageWidth: Width of the game image, imageHeight: Height of the game image.
func NewParams(turns, threads, imageWidth, imageHeight int) Params {
	return Params{
		Turns:       turns,
		Threads:     threads,
		ImageWidth:  imageWidth,
		ImageHeight: imageHeight,
	}
}
