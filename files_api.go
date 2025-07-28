package main

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
)


// fileAPIRoot is a global variable set in main.go

// --- File API message structs ---

type fileRequest struct {
	Action  string `json:"action"` // "list", "read", "write", "watch"
	Path    string `json:"path"`
	Content string `json:"content,omitempty"` // Base64 encoded content for "write"
}

type fileResponse struct {
	Action string      `json:"action"`
	Path   string      `json:"path"`
	Error  string      `json:"error,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

type fileInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // Unix timestamp
}

// --- File Watcher ---

// watcherManager manages fsnotify watchers and WebSocket subscribers.
type watcherManager struct {
	watcher     *fsnotify.Watcher
	subscribers map[*websocket.Conn]map[string]bool // map[client]map[path]bool
	mu          sync.Mutex
}

// Global instance of the watcher manager.
var fileWatcher *watcherManager

func init() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	fileWatcher = &watcherManager{
		watcher:     watcher,
		subscribers: make(map[*websocket.Conn]map[string]bool),
	}
}

// run starts the watcher loop to process and broadcast file events.
func (wm *watcherManager) run() {
	for {
		select {
		case event, ok := <-wm.watcher.Events:
			if !ok {
				return
			}
			wm.broadcastEvent(event)
		case err, ok := <-wm.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)
		}
	}
}

func (wm *watcherManager) addSubscription(client *websocket.Conn, path string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, ok := wm.subscribers[client]; !ok {
		wm.subscribers[client] = make(map[string]bool)
	}
	if !wm.subscribers[client][path] {
		wm.subscribers[client][path] = true
		wm.watcher.Add(path)
	}
}

func (wm *watcherManager) removeClient(client *websocket.Conn) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	// In a real app, you might want to check if a path has no more subscribers
	// and remove it from the underlying fsnotify watcher to save resources.
	delete(wm.subscribers, client)
}

func (wm *watcherManager) broadcastEvent(event fsnotify.Event) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for client, paths := range wm.subscribers {
		// Check if the client is subscribed to the event's directory
		if paths[filepath.Dir(event.Name)] {
			resp := fileResponse{
				Action: "notify",
				Path:   event.Name,
				Data:   event.Op.String(), // e.g., "WRITE", "CREATE"
			}
			client.WriteJSON(resp)
		}
	}
}

// --- Main Handler ---

// filesApiHandler routes requests to either REST or WebSocket handlers.
func filesApiHandler(w http.ResponseWriter, r *http.Request) {
	// The upgrader's CheckOrigin function handles WebSocket connections.
	// For consistency, update it to use our new shared authorization function.
	upgrader.CheckOrigin = func(req *http.Request) bool {
		return checkRequestAuthorization(req)
	}
	if websocket.IsWebSocketUpgrade(r) {
		handleFileWs(w, r)
		return
	}

	// For REST calls, use the shared authorization logic.
	if !checkRequestAuthorization(r) {
		// checkRequestAuthorization logs the reason for denial internally.
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	handleFileRest(w, r)
}

// --- Security Helper ---

// securePath cleans and validates a path against the root directory.
func securePath(path string) (string, error) {
	// 1. Get absolute path of the root
	absRoot, err := filepath.Abs(fileAPIRoot)
	if err != nil {
		return "", err
	}
	// 2. Resolve symlinks for the root itself, important if root is a symlink
	absRoot, err = filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err // e.g., root doesn't exist or permissions issue
	}
	// 3. Join and clean the requested path relative to the root
	absPath := filepath.Join(absRoot, filepath.Clean(path))
	if !strings.HasPrefix(absPath, absRoot) {
		return "", os.ErrPermission
	}
	return absPath, nil
}

// --- REST Implementation ---

func handleFileRest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	fullPath, err := securePath(path)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleRestGet(w, fullPath, path)
	case http.MethodPost:
		handleRestPost(w, r, fullPath)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleRestGet(w http.ResponseWriter, fullPath, reqPath string) {
	stat, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var respData interface{}

	if stat.IsDir() {
		// List directory
		files, err := ioutil.ReadDir(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fileList := make([]fileInfo, 0, len(files))
		for _, f := range files {
			fileList = append(fileList, fileInfo{
				Name: f.Name(), IsDir: f.IsDir(), Size: f.Size(), ModTime: f.ModTime().Unix(),
			})
		}
		respData = fileList
	} else {
		// Read file
		content, err := ioutil.ReadFile(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respData = base64.StdEncoding.EncodeToString(content)
	}

	resp := fileResponse{Action: "read", Path: reqPath, Data: respData}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRestPost(w http.ResponseWriter, r *http.Request, fullPath string) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read body", http.StatusBadRequest)
		return
	}
	// Assumes raw binary content in POST body for simplicity.
	// A JSON-based approach might wrap it: {"content": "base64data"}
	err = ioutil.WriteFile(fullPath, body, 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// --- WebSocket Implementation ---

func handleFileWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("File WS upgrade failed: %v", err)
		return
	}
	defer ws.Close()
	defer fileWatcher.removeClient(ws)

	for {
		var req fileRequest
		if err := ws.ReadJSON(&req); err != nil {
			break
		}
		handleWsRequest(ws, req)
	}
}

func handleWsRequest(ws *websocket.Conn, req fileRequest) {
	fullPath, err := securePath(req.Path)
	if err != nil {
		ws.WriteJSON(fileResponse{Action: req.Action, Path: req.Path, Error: "Forbidden"})
		return
	}

	var resp fileResponse
	resp.Action = req.Action
	resp.Path = req.Path

	switch req.Action {
	case "list":
		files, err := ioutil.ReadDir(fullPath)
		if err != nil {
			resp.Error = err.Error()
		} else {
			fileList := make([]fileInfo, len(files))
			for i, f := range files {
				fileList[i] = fileInfo{Name: f.Name(), IsDir: f.IsDir(), Size: f.Size(), ModTime: f.ModTime().Unix()}
			}
			resp.Data = fileList
		}
	case "read":
		content, err := ioutil.ReadFile(fullPath)
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Data = base64.StdEncoding.EncodeToString(content)
		}
	case "write":
		data, err := base64.StdEncoding.DecodeString(req.Content)
		if err != nil {
			resp.Error = "Invalid base64 content"
		} else if err := ioutil.WriteFile(fullPath, data, fs.FileMode(0644)); err != nil {
			resp.Error = err.Error()
		}
	case "watch":
		fileWatcher.addSubscription(ws, fullPath)
		// No immediate response needed for watch, confirmations are implicit
		return
	default:
		resp.Error = "Unknown action"
	}

	ws.WriteJSON(resp)
}
