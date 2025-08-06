//go:build !plan9
// +build !plan9

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

// This function now contains the core server logic, moved from main().
func runConduitServer() {
	getIsCompiled()
	
	flag.BoolVar(&debugLogging, "debug", false, "Enable debug logging")
	flag.BoolVar(&keyFlag, "key", false, "Manage and print the API key for no-origin requests, then exit.")
	flag.BoolVar(&installUserFlag, "install-user", false, "Install Conduit as a user-level application and protocol handler.")
	flag.BoolVar(&installServiceFlag, "install-service", false, "Install Conduit as a systemd service (requires root).")
	flag.BoolVar(&uninstallFlag, "uninstall", false, "Uninstall user and/or system Conduit installations.")
	flag.StringVar(&rootFlag, "root", "", "Set the root directory for the file API (defaults to user's home directory).")
	flag.BoolVar(&noIdleShutdownFlag, "no-idle-shutdown", false, "Disable automatic shutdown due to inactivity. Recommended for services.")
	flag.Parse()
	
	manageAPIKey(keyFlag)

	if keyFlag {
		os.Exit(0)
	}

	if installUserFlag {
		msg, err := InstallUser()
		log.Println(msg)
		if err != nil { os.Exit(1) }
		os.Exit(0)
	}
	if installServiceFlag {
		msg, err := InstallService()
		log.Println(msg)
		if err != nil { os.Exit(1) }
		os.Exit(0)
	}
	if uninstallFlag {
		msg, err := Uninstall()
		log.Println(msg)
		if err != nil { os.Exit(1) }
		os.Exit(0)
	} else if len(flag.Args()) > 0 && flag.Args()[0] == "kill" {
		log.Println("Shutting down Conduit via command line kill command.")
		os.Exit(0)
	}

	if rootFlag != "" {
		fileAPIRoot = rootFlag
	} else {
		homeDir, err := os.UserHomeDir()
		if err == nil { fileAPIRoot = homeDir } else { fileAPIRoot = "." }
	}
	go fileWatcher.run()
	updateLastActivity()
	if !noIdleShutdownFlag {
		go startIdleShutdownManager(60 * time.Minute)
	}
	startTime = time.Now()
	mux := http.NewServeMux()
	log.Printf("Running as compiled build: %t", isCompiledBuild)
	mux.HandleFunc("/terminal", terminalServer)
	mux.HandleFunc("/up", upcheckHandler)
	mux.HandleFunc("/files", filesApiHandler)
	mux.HandleFunc("/kill", installationHandler(killHandler))
	mux.HandleFunc("/install-service", installationHandler(InstallService))
	mux.HandleFunc("/uninstall", installationHandler(Uninstall))
	mux.HandleFunc("/install-user", installationHandler(InstallUser))

	log.Printf("File API Root: %s", fileAPIRoot)
	log.Printf("Conduit v%s - listening for WS connections (localhost:%s)", version, port)
	log.Println("------------------------------------------------------------")

	// err := http.ListenAndServe(":"+port, activityMiddleware(mux))
	err := http.ListenAndServe(":"+port, activityMiddleware(corsMiddleware(mux)))

	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// Global variables remain accessible
const version = "0.1.1"
const port = "3022"
var allowedOrigins = map[string]bool{
	"https://cadence.jakbox.dev": true,
	"https://cadence.jakbox.net": true,
	"https://code.jakbox.dev": true,
	"https://code.jakbox.net": true,
	"http://localhost:8083":  true,
	"http://localhost":       true,
}
var rootFlag string
var keyFlag bool
var installUserFlag bool
var installServiceFlag bool
var uninstallFlag bool
var noIdleShutdownFlag bool
var debugLogging bool
var requiredAPIKey string
var isCompiledBuild bool
var fileAPIRoot string
var lastActivityTimestamp atomic.Int64
// (Keep all your other helper functions like updateLastActivity, etc., here too)
// --- Helper Functions ---
func getIsCompiled() {
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Warning: Could not determine executable path: %v", err)
	} else {
		exeName := filepath.Base(exePath)
		isCompiledBuild = strings.HasPrefix(exeName, "conduit-") || exeName == "conduit" || exeName == "conduit.exe"
	}
}
func updateLastActivity() {
	lastActivityTimestamp.Store(time.Now().Unix())
}
func activityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateLastActivity()
		next.ServeHTTP(w, r)
	})
}
// corsMiddleware adds the necessary headers to handle CORS requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Conduit-Key")
		}
		// Handle preflight requests by immediately returning.
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func startIdleShutdownManager(timeout time.Duration) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
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
func installationHandler(handlerFunc func() (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
func killHandler() (string, error) {
	if noIdleShutdownFlag {
		return "Kill command is disabled when running with --no-idle-shutdown.", fmt.Errorf("kill command disabled")
	}
	log.Println("Received /kill request. Shutting down application.")
	go func() { time.Sleep(100 * time.Millisecond); os.Exit(0) }()
	return "Conduit server is shutting down.", nil
}