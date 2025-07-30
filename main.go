package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"path/filepath"
	"strings"
	"time"
)

// --- Configuration ---
const version = "0.1.1"
const port = "3022"

var allowedOrigins = map[string]bool{
	"https://code.jakbox.dev": true,
	"http://localhost:8083":  true,
	"http://localhost":       true,
}

var rootFlag string // New flag for file API root
var keyFlag bool // New flag for key management
var installUserFlag bool
var installServiceFlag bool // New flag for service installation
var uninstallFlag bool      // New flag for uninstallation
var noIdleShutdownFlag bool // New flag to prevent idle shutdown
var debugLogging bool
var requiredAPIKey string // Stores the API key if --key is used. Empty if no key is required.
var isCompiledBuild bool  // flag to indicate running from compiled binary
var fileAPIRoot string  // fileAPIRoot is set by main based on CLI args or defaults to user's home dir.
var lastActivityTimestamp atomic.Int64 // Tracks the unix timestamp of the last activity
// updateLastActivity sets the last activity timestamp to the current time.
func updateLastActivity() {
	lastActivityTimestamp.Store(time.Now().Unix())
}
// activityMiddleware wraps an HTTP handler to update the last activity timestamp on every request.
func activityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateLastActivity()
		next.ServeHTTP(w, r)
	})
}
// startIdleShutdownManager runs a background task to shut down the server if idle.
func startIdleShutdownManager(timeout time.Duration) {
	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()
	for range ticker.C {
		// Only check for shutdown if there are no active connections.
		if atomic.LoadInt32(&activeConnections) == 0 {
			lastActivity := lastActivityTimestamp.Load()
			idleDuration := time.Since(time.Unix(lastActivity, 0))
			if idleDuration >= timeout {
				log.Printf("Shutting down due to inactivity for over %v.", timeout)
				os.Exit(0)
			}
		}
	}
}
// installationHandler wraps installer functions to provide a localhost-only HTTP endpoint.
func installationHandler(handlerFunc func() (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Strict loopback check for installation actions
		remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if remoteIP != "127.0.0.1" && remoteIP != "::1" {
			log.Printf("[SECURITY] Denied installation request from remote address: %s", r.RemoteAddr)
			http.Error(w, "Forbidden: Installation actions are only allowed from localhost.", http.StatusForbidden)
			return
		}

		msg, err := handlerFunc()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err != nil {
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(msg))
	}
}

func getIsCompiled() {
	// Determine if running as a compiled build based on the executable name.
	exePath, err := os.Executable()
	
	// log.Printf("exePath %s", exePath)
	if err != nil {
		log.Printf("Warning: Could not determine executable path: %v", err)
	} else {
		exeName := filepath.Base(exePath)
		// log.Printf("exeName %s", exeName)
		// Compiled builds are expected to be named "conduit-[os]-[arch]"
		
		isCompiledBuild = strings.HasPrefix(exeName, "conduit-") && (len(strings.Split(exeName, "-"))>2)
	}
}

func main() {
	
	getIsCompiled()
	
	flag.BoolVar(&debugLogging, "debug", false, "Enable debug logging")
	flag.BoolVar(&keyFlag, "key", false, "Manage and print the API key for no-origin requests, then exit.")
	flag.BoolVar(&installUserFlag, "install-user", false, "Install Conduit as a user-level application and protocol handler.")
	flag.BoolVar(&installServiceFlag, "install-service", false, "Install Conduit as a systemd service (requires root).")
	flag.BoolVar(&uninstallFlag, "uninstall", false, "Uninstall user and/or system Conduit installations.")
	flag.StringVar(&rootFlag, "root", "", "Set the root directory for the file API (defaults to user's home directory).")
	flag.BoolVar(&noIdleShutdownFlag, "no-idle-shutdown", false, "Disable automatic shutdown due to inactivity. Recommended for services.")
	flag.Parse()
	
	// Always attempt to load the API key at startup.
	// The `manageAPIKey` function itself will handle conditional generation/printing.
	manageAPIKey(keyFlag) // Pass keyFlag to indicate if we're in "manage and exit" mode

	// If --key flag is present, manage the API key and exit.
	if keyFlag {
		os.Exit(0)
	}

	// Handle installation flags
	if installUserFlag {
		msg, err := InstallUser()
		log.Println(msg)
		if err != nil { os.Exit(1) }
		os.Exit(0)
	}
	// Handle install-service flag
	if installServiceFlag {
		msg, err := InstallService() // Call the InstallService function
		log.Println(msg)
		if err != nil { os.Exit(1) }
		os.Exit(0)
	}
	// Handle uninstall flag
	if uninstallFlag {
		msg, err := Uninstall() // Call the Uninstall function
		log.Println(msg)
		if err != nil { os.Exit(1) }
		os.Exit(0)
	} else if len(flag.Args()) > 0 && flag.Args()[0] == "kill" { // This handles `conduit kill` for direct shutdown
		// This path is less common for an HTTP service, but
		// if the user types `conduit kill` on the command line
		log.Println("Shutting down Conduit via command line kill command.")
		// Perform any necessary cleanup before exiting
		os.Exit(0)

	}

	// Initialize and start the global file watcher

	// Set the file API root
	if rootFlag != "" {
		fileAPIRoot = rootFlag
	} else {
		homeDir, err := os.UserHomeDir()
		if err == nil { fileAPIRoot = homeDir } else { fileAPIRoot = "." } // Fallback to current dir if home not found
	}
	go fileWatcher.run()
	// Initialize activity timestamp.
	updateLastActivity()
	// Start the idle shutdown manager only if the flag is not set.
	if !noIdleShutdownFlag {
		go startIdleShutdownManager(60 * time.Minute)
	}
	startTime = time.Now()
	mux := http.NewServeMux()
	log.Printf("Running as compiled build: %t", isCompiledBuild)
	mux.HandleFunc("/terminal", terminalServer)
	mux.HandleFunc("/up", upcheckHandler)
	mux.HandleFunc("/files", filesApiHandler)
	mux.HandleFunc("/kill", installationHandler(killHandler)) // Add /kill endpoint
	// Add installation handlers
	mux.HandleFunc("/install-service", installationHandler(InstallService)) // HTTP endpoint for service install
	mux.HandleFunc("/uninstall", installationHandler(Uninstall))           // HTTP endpoint for uninstall
	mux.HandleFunc("/install-user", installationHandler(InstallUser))

	log.Printf("File API Root: %s", fileAPIRoot)
	log.Printf("Conduit v%s - listening for WS connections (localhost:%s)", version, port)
	log.Println("------------------------------------------------------------")

	// Wrap the main handler with the activity-tracking middleware.
	err := http.ListenAndServe(":"+port, activityMiddleware(mux))

	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// killHandler provides a localhost-only endpoint to gracefully shut down the application.
func killHandler() (string, error) {
	if noIdleShutdownFlag {
		return "Kill command is disabled when running with --no-idle-shutdown.", fmt.Errorf("kill command disabled")
	}
	log.Println("Received /kill request. Shutting down application.")
	// A short delay to ensure the HTTP response is sent before exiting.
	go func() { time.Sleep(100 * time.Millisecond); os.Exit(0) }()
	return "Conduit server is shutting down.", nil
}
