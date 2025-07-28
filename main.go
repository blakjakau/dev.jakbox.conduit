package main

import (
	"flag"
	"log"
	"os"
	"net/http"
	"time"
)

// --- Configuration ---
const version = "0.1.0"
const port = "3022"

var allowedOrigins = map[string]bool{
	"https://code.jakbox.dev": true,
	"http://localhost:8083":  true,
	"http://localhost":       true,
}

var rootFlag string // New flag for file API root
var keyFlag bool // New flag for key management
var debugLogging bool
var requiredAPIKey string // Stores the API key if --key is used. Empty if no key is required.

// fileAPIRoot is set by main based on CLI args or defaults to user's home dir.
var fileAPIRoot string 
func main() {
	flag.BoolVar(&debugLogging, "debug", false, "Enable debug logging")
	flag.BoolVar(&keyFlag, "key", false, "Manage and print the API key for no-origin requests, then exit.")
	flag.Parse()
	
	// Always attempt to load the API key at startup.
	// The `manageAPIKey` function itself will handle conditional generation/printing.
	manageAPIKey(keyFlag) // Pass keyFlag to indicate if we're in "manage and exit" mode

	// If --key flag is present, manage the API key and exit.
	if keyFlag {
		os.Exit(0)
	}
	// Initialize and start the global file watcher

	// Set the file API root
	flag.StringVar(&rootFlag, "root", "", "Set the root directory for the file API (defaults to user's home directory).")
	flag.Parse() // Parse flags again to get the new --root value

	if rootFlag != "" {
		fileAPIRoot = rootFlag
	} else {
		homeDir, err := os.UserHomeDir()
		if err == nil { fileAPIRoot = homeDir } else { fileAPIRoot = "." } // Fallback to current dir if home not found
	}
	go fileWatcher.run()
	startTime = time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/terminal", terminalServer)
	mux.HandleFunc("/up", upcheckHandler)
	mux.HandleFunc("/files", filesApiHandler)

	log.Printf("File API Root: %s", fileAPIRoot)
	log.Printf("Conduit v%s - listening for WS connections (localhost:%s)", version, port)
	log.Println("------------------------------------------------------------")

	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
