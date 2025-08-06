package main

import (
	"encoding/json"
	"log"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
	"github.com/gorilla/websocket"
)

// --- Server State ---
var activeConnections int32
var sessionIdCounter int32
var startTime time.Time

// A simple struct to handle JSON messages from the client
type wsMessage struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`  // Used by client for "data"
	Cols    int    `json:"cols,omitempty"`     // Used by client for "resize"
	Rows    int    `json:"rows,omitempty"`     // Used by client for "resize"
	Hostname string `json:"hostname,omitempty"` // Used by server for "terminalInfo"
	Cwd      string `json:"cwd,omitempty"`      // Used by server for "terminalInfo"
}

// Struct for the /up status response
type statusResponse struct {
	Status            string  `json:"status"`
	Version           string  `json:"version"`
	UptimeSeconds     float64 `json:"uptime_seconds"`
	ActiveConnections int32   `json:"active_connections"`
	IsInstalled       bool    `json:"is_installed"`
}

// Gorilla WebSocket upgrader with origin check
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			return true
		}
		log.Printf("[SECURITY] Denied connection from invalid origin: %s", origin)
		return false
	},
}

// checkRequestAuthorization checks the origin or API key for a request.
// It returns true if the request is authorized, false otherwise.
func checkRequestAuthorization(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	if debugLogging {
		log.Printf("[DEBUG] Auth check: method=%s, path=%s, origin='%s'", r.Method, r.URL.Path, origin)
	}

	if origin != "" {
		// If an Origin header is present, enforce CORS based on allowedOrigins.
		if allowedOrigins[origin] {
			return true
		}
		log.Printf("[SECURITY] Denied: Invalid Origin '%s' from %s", origin, r.RemoteAddr)
		return false
	}

	// If no Origin header, check if it's a loopback address.
	// We split RemoteAddr to get just the IP, ignoring the port.
	remoteIP := r.RemoteAddr
	if colon := strings.LastIndex(remoteIP, ":"); colon != -1 {
		remoteIP = remoteIP[:colon]
	}
	if remoteIP == "127.0.0.1" || remoteIP == "[::1]" { // Check for IPv4 and IPv6 loopback
		return true // Allow loopback without Origin or API key
	}

	// No Origin header: check for required API key.
	if requiredAPIKey != "" {
		providedKey := r.Header.Get("X-Conduit-Key") // Check header first
		if providedKey == "" {
			providedKey = r.URL.Query().Get("key") // Then check query parameter
		}

		if providedKey == "" {
			log.Printf("[SECURITY] Denied: Missing API key for no-origin request from %s", r.RemoteAddr)
			return false
		}
		if providedKey != requiredAPIKey {
			log.Printf("[SECURITY] Denied: Invalid API key from %s", r.RemoteAddr)
			return false
		}
		if debugLogging {
			log.Printf("[DEBUG] Authorized: API key matched for no-origin request from %s", r.RemoteAddr)
		}
		return true
	}

	// If we reach here: no Origin header, AND no API key is required/configured.
	// Deny by default in this scenario to prevent unintended access.
	if debugLogging {
		log.Printf("[DEBUG] Denied: No Origin and no API key configured/provided for %s", r.RemoteAddr)
	}
	return false
}

// writePump pumps messages from the PTY to the websocket connection.
func writePump(ws *websocket.Conn, ptmx io.ReadCloser, sessionID int32) {
	defer ws.Close()
	buffer := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buffer)
		if err != nil {
			// If the PTY process has exited, this read will eventually return an error like EOF.
			// We close the websocket when that happens.
			// This usually means the PTY process has exited.
			return
		}
		// Use TextMessage for compatibility with clients not expecting binary frames.
		// Note: PTY output can contain non-UTF8 bytes, which might cause issues for some
		// text-only WebSocket clients if not handled. gorilla/websocket allows binary frames
		// (websocket.BinaryMessage) for raw byte streams, which is more robust, but
		// text messages are used here for compatibility as per user's earlier preference.
		if err := ws.WriteMessage(websocket.TextMessage, buffer[:n]); err != nil {
			return
		}
	}
}

// readPump pumps messages from the websocket connection to the PTY.
func readPump(ws *websocket.Conn, ptmx io.Writer, ptyCmd *exec.Cmd, sessionID int32) {

	defer func() {
		ws.Close()
		// On Windows, ptyCmd.Process can be nil because the conpty library
		// doesn't expose it. The ptmx.Close() call in the calling function
		// (terminalServer) handles killing the process correctly on all platforms.
		if ptyCmd.Process != nil {
			ptyCmd.Process.Kill()
		}
	}()

	for {
		var msg wsMessage
		// ReadJSON is a convenient helper for JSON-based APIs.
		err := ws.ReadJSON(&msg)
		if err != nil {
			// Report unexpected close errors.
			if !websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS read error for session #%d: %v", sessionID, err)
			}
			break
		}
		switch msg.Type {
		case "resize":
			// TODO: Resizing is now platform-specific. A new cross-platform
			// interface is needed to handle this properly.
			if resizePty != nil {
				resizePty(msg.Cols, msg.Rows)
			}
		case "data":
			ptmx.Write([]byte(msg.Content))
		}
	}
}

// terminalServer handles websocket requests from the peer.
func terminalServer(w http.ResponseWriter, r *http.Request) {
	// Override the upgrader's CheckOrigin to use our shared authorization logic.
	// This ensures consistency between /terminal and /files WS origins.
	upgrader.CheckOrigin = func(req *http.Request) bool {
		return checkRequestAuthorization(req)
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ERROR: Failed to upgrade connection: %v", err)
		return
	}
	defer ws.Close()

	atomic.AddInt32(&activeConnections, 1)
	sessionID := atomic.AddInt32(&sessionIdCounter, 1)
	defer atomic.AddInt32(&activeConnections, -1)

	shell := "bash"
	if os.Getenv("OS") == "Windows_NT" {
		shell = "powershell.exe"
	}

	// Set the working directory for the shell to the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("ERROR: Could not get user home directory: %v, using current directory.", err)
		homeDir = "." // Fallback to current dir if home is not found
	}
	// Use our new platform-agnostic function to start the PTY.
	// This call now handles all OS-specific logic and command creation. It also returns a resize function.
	var resizeFunc func(cols, rows int)
	ptmx, ptyCmd, resizeFunc, err := startPty(shell, homeDir)
	if err != nil {
		log.Printf("ERROR: Failed to start PTY for session #%d: %v", sessionID, err)
		return
	}
	defer ptmx.Close()

	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("Error getting hostname: %v", err)
		hostname = "unknown"
	}

	// The initial working directory of the PTY process (which was set to homeDir)
	initialCwd := homeDir

	// Send initial terminal info to the client
	infoMsg := wsMessage{
		Type:    "terminalInfo",
		Hostname: hostname,
		Cwd:     initialCwd,
	}
	ws.WriteJSON(infoMsg) // Ignore error, best effort to send initial info
	resizePty = resizeFunc // Assign the resize function for this session
	pid := -1 // Default to -1 if process info is not available (e.g., on Windows)
	if ptyCmd.Process != nil {
		pid = ptyCmd.Process.Pid
	}
	log.Printf("[%s] Client #%d (PID: %d) connected (active: %d)", timestamp, sessionID, pid, activeConnections)
	defer log.Printf("[%s] Client #%d (PID: %d) disconnected (active: %d)", time.Now().UTC().Format(time.RFC3339), sessionID, pid, activeConnections)
	// Start a goroutine to wait for the PTY process to exit. When it exits,
	// we close the WebSocket connection. This is crucial for reliably
	// handling the "exit" command from within the shell, especially on Windows.
	if ptyCmd.Process != nil {
		go func() {
			ptyCmd.Process.Wait()
			ws.Close()
		}()
	}	
	go writePump(ws, ptmx, sessionID)
	readPump(ws, ptmx, ptyCmd, sessionID)
}

// upcheckHandler provides a simple health check endpoint.
func upcheckHandler(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime).Seconds()
	connections := atomic.LoadInt32(&activeConnections)

	resp := statusResponse{
		Status:            "running",
		Version:           version,
		UptimeSeconds:     uptime,
		ActiveConnections: connections,
		IsInstalled:       checkIfInstalled(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// resizePty stores the current session's PTY resize function.
var resizePty func(cols, rows int)
