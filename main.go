package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"github.com/creack/pty"
	"golang.org/x/net/websocket"
)

// --- Configuration ---
const version = "0.1.0"
const port = "3022"

var allowedOrigins = map[string]bool{
	"https://code.jakbox.dev": true,
	"http://localhost:8083":  true,
	"http://localhost":       true,
}

// --- Server State ---
var activeConnections int32
var sessionIdCounter int32

// A simple struct to handle JSON messages from the client
type wsMessage struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

// --- Main Handler ---
func terminalServer(ws *websocket.Conn) {
	// Increment counters and log the connection
	atomic.AddInt32(&activeConnections, 1)
	sessionId := atomic.AddInt32(&sessionIdCounter, 1)
	defer atomic.AddInt32(&activeConnections, -1)

	// Start the PTY process
	shell := "bash"
	if os.Getenv("OS") == "Windows_NT" {
		shell = "powershell.exe"
	}
	c := exec.Command(shell)
	// Explicitly set the TERM variable for full 256-color support.
	c.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.Start(c)
	if err != nil {
		log.Printf("ERROR: Failed to start PTY for session #%d: %v", sessionId, err)
		return
	}
	defer ptmx.Close()

	// Log the successful connection with PID
	timestamp := time.Now().UTC().Format(time.RFC3339)
	log.Printf("[%s] Client #%d (PID: %d) connected (active: %d)", timestamp, sessionId, c.Process.Pid, activeConnections)
	defer log.Printf("[%s] Client #%d (PID: %d) disconnected (active: %d)", time.Now().UTC().Format(time.RFC3339), sessionId, c.Process.Pid, activeConnections)

	// Handle PTY -> WebSocket
	go func() {
		for {
			buffer := make([]byte, 4096) // Use a larger buffer
			n, readErr := ptmx.Read(buffer)
			if readErr != nil { // This usually means the PTY process has exited.
				ws.Close()
				return
			}
			// The robust, hybrid solution:
			// Try to send as valid UTF-8 first. If that fails, use the latin1 trick
			// as a fallback for corrupt/raw byte sequences.
			var writeErr error
			if utf8.Valid(buffer[:n]) {
				writeErr = websocket.Message.Send(ws, string(buffer[:n]))
			} else {
				runes := make([]rune, n)
				for i := 0; i < n; i++ { runes[i] = rune(buffer[i]) }
				writeErr = websocket.Message.Send(ws, string(runes))
			}
			if writeErr != nil { // This usually means the client has disconnected.
				return
			}
		}
	}()

	// Handle WebSocket -> PTY
	for {
		var msg wsMessage
		// Read assumes text frames, for binary use websocket.Message.Receive
		err := websocket.JSON.Receive(ws, &msg)
		if err != nil {
			c.Process.Kill()
			break
		}
		switch msg.Type {
		case "resize":
			pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(msg.Rows), Cols: uint16(msg.Cols)})
		case "data":
			ptmx.Write([]byte(msg.Content))
		}
	}
}

func main() {
	// http.Handle("/ws", websocket.Handler(terminalServer))
	log.Printf("Conduit v%s - listening for WS connections (localhost:%s)", version, port)
	log.Println("------------------------------------------------------------")

	// We wrap the main handler to add our origin check for security
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !allowedOrigins[origin] {
			log.Printf("[SECURITY] Denied connection from invalid origin: %s", origin)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		// If origin is OK, serve the WebSocket endpoint
		if r.URL.Path == "/terminal" {
			websocket.Handler(terminalServer).ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	err := http.ListenAndServe(":"+port, handler)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
