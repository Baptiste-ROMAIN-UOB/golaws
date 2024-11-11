package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"uk.ac.bris.cs/gameoflife/gol"
	"uk.ac.bris.cs/gameoflife/sdl"
)

// main is the function called when starting Game of Life with 'go run .'
func main() {
	fmt.Println("Starting Game...")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var params gol.Params
	var engineAddress string
	var port string

	// Define flags for parameters
	flag.IntVar(&params.Threads, "t", 8, "Specify the number of worker threads to use. Defaults to 8.")
	flag.IntVar(&params.ImageWidth, "w", 512, "Specify the width of the image. Defaults to 512.")
	flag.IntVar(&params.ImageHeight, "h", 512, "Specify the height of the image. Defaults to 512.")
	flag.IntVar(&params.Turns, "turns", 10000000000, "Specify the number of turns to process. Defaults to 10000000000.")

	headless := flag.Bool("headless", false, "Disable the SDL window for running in a headless environment.")
	flag.StringVar(&port, "Port", "8080", "Specify the port the component will use.")
	flag.StringVar(&engineAddress, "EngineAddress", "", "Specify the address of the engine. Leave empty to input manually.")

	// Parse command-line flags
	flag.Parse()

	// Check if engineAddress is empty, prompt the user to input it
	if engineAddress == "" {
		fmt.Print("Please enter the IP address of the server: ")
		fmt.Scanln(&engineAddress)
	}

	// Print the configuration
	fmt.Printf("%-10v %v\n", "Threads", params.Threads)
	fmt.Printf("%-10v %v\n", "Width", params.ImageWidth)
	fmt.Printf("%-10v %v\n", "Height", params.ImageHeight)
	fmt.Printf("%-10v %v\n", "Turns", params.Turns)
	fmt.Printf("%-10v %v\n", "EngineAddress", engineAddress)

	// Channels for handling events and key presses
	keyPresses := make(chan rune, 10)
	events := make(chan gol.Event, 1000)

	// Handle termination signals
	go handleSigterm(keyPresses)

	// Run the game logic, passing the engine address to the distributor
	go gol.Run_Adress(params, events, keyPresses, &engineAddress)

	// Run SDL if not headless
	if !(*headless) {
		sdl.Run(params, events, keyPresses)
	} else {
		sdl.RunHeadless(events)
	}
}

// handleSigterm handles termination signals (Ctrl+C or SIGTERM) and stops the game.
func handleSigterm(keyPresses chan<- rune) {
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM, syscall.SIGINT)
	<-sigterm
	keyPresses <- 'q'
}
