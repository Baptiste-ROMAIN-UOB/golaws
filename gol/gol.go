package gol

// Params provides the details of how to run the Game of Life and which image to load.
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// ioCommand allows requesting behaviour from the io (pgm) goroutine.
type ioCommand uint8

// This is a way of creating enums in Go.
const (
	ioOutput ioCommand = iota
	ioInput
	ioCheckIdle
)

// ioChannels holds all channels used for communication with the io goroutine.
type ioChannels struct {
	command  chan ioCommand // 双向通道
	idle     chan bool      // 双向通道
	filename chan string    // 双向通道
	output   chan uint8     // 双向通道
	input    chan uint8     // 双向通道
}

// Run starts the processing of Game of Life with distributed computation
func Run(p Params, events chan<- Event, keyPresses <-chan rune) {
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

	distributorChannels := distributorChannels{
		events:    events,
		ioCommand: ioCommand,
		ioIdle:    ioIdle,
	}

	go startIo(p, ioChannels)

	// Start the distributor with all necessary channels
	distributor(p, distributorChannels, ioInput, ioOutput, ioFilename, keyPresses)
}

// NewParams creates a new set of parameters for the Game of Life
func NewParams(turns, threads, imageWidth, imageHeight int) Params {
	return Params{
		Turns:       turns,
		Threads:     threads,
		ImageWidth:  imageWidth,
		ImageHeight: imageHeight,
	}
}
