# Conduit API: Implementation Guide for AI

This document details the Conduit API, designed for clear, direct implementation by an AI. All communication is via standard HTTP/WebSocket, with JSON for structured data.

## Authentication & Authorization

All API endpoints (/terminal, /files, /up) use the same authorization logic.

-   **Browser Clients (Cross-Origin):**
    -   The browser automatically sends an Origin header (e.g., https://code.jakbox.dev, http://localhost:8083).
    -   This Origin *must* be in the server's allowedOrigins list.
    -   No API key is required if a valid Origin is present.

-   **Non-Browser Clients (or no Origin header):**
    -   If no Origin header is sent (e.g., curl, custom client, file:// loaded HTML), an API key is required *unless* the request is from 127.0.0.1 or [::1] (localhost).
    -   Provide the API key in:
        -   **HTTP Header:** X-Conduit-Key: YOUR_API_KEY
        -   **Query Parameter (less secure for GET/WS):** key=YOUR_API_KEY (e.g., /files?path=.&key=YOUR_API_KEY)

## CLI Configuration Flags
-   `--install-user`: Installs Conduit for the current user. See Installation section.
-   `--install-service`: Installs Conduit as a systemd service (Linux only, requires root).
-   `--uninstall`: Removes user and/or system installations.
-   `--no-idle-shutdown`: Disables the default 60-minute idle shutdown timer. This is automatically used when installing as a service.


## 1. Terminal API (/terminal)

**Purpose:** Provides a full-duplex WebSocket connection to a pseudo-terminal (shell).
**Endpoint:** ws://<host>:<port>/terminal (e.g., ws://localhost:3022/terminal)
**Protocol:** WebSocket only.

### Client-to-Server Messages (JSON Text)

-   **User Input:**
    { "type": "data", "content": "ls -l\r" }
    content is raw string input to the PTY. \r (carriage return) is commonly used to send commands.
-   **Terminal Resize:**
    { "type": "resize", "cols": 120, "rows": 40 }
    cols and rows are integers specifying the new terminal dimensions.

### Server-to-Client Messages (Raw Text)

-   The server sends raw terminal output directly as WebSocket text messages.
-   **Example:** If client sends ls -l\r, server sends back the ASCII output of ls -l in chunks.
-   The client (e.g., xterm.js) should write this data directly to its terminal instance.

## 2. Files API (/files)

**Purpose:** Access, manipulate, and monitor files within a server-defined root directory.
**Endpoint:** http://<host>:<port>/files (for REST) or ws://<host>:<port>/files (for WebSocket)
**Root Directory:** All paths are relative to the server's configured root directory (defaults to user's home). Path traversal (..) is strictly forbidden.

### Common Data Structures

-   **FileInfo Object (JSON):**
    {
      "name": "filename.txt",  // Basename of the file/directory
      "isDir": false,           // true if directory, false if file
      "size": 1024,             // File size in bytes (0 for directories)
      "modTime": 1678886400     // Unix timestamp of last modification
    }

### 2.1. Files REST API

**Method:** GET /files
**Query Parameter:** path (string, required): Relative path to file or directory. Use . for the root.

-   **Read File:**
    -   **Request:** GET /files?path=my_document.txt
    -   **Response (200 OK):**
        {
          "action": "read",
          "path": "my_document.txt",
          "data": "SGVsbG8sIFdvcmxkIQ==" // Base64 encoded file content
        }
    -   **Error (404 Not Found):** If file not found.

-   **List Directory:**
    -   **Request:** GET /files?path=my_folder
    -   **Response (200 OK):**
        {
          "action": "list",
          "path": "my_folder",
          "data": [
            { "name": "file1.txt", "isDir": false, "size": 100, "modTime": 1678886400 },
            { "name": "sub_dir", "isDir": true, "size": 0, "modTime": 1678886500 }
          ]
        }
    -   **Error (404 Not Found):** If directory not found.

**Method:** POST /files
**Query Parameter:** path (string, required): Relative path to file to create/overwrite.
**Request Body:** Raw binary/text content to write to the file.

-   **Write/Create File:**
    -   **Request:** POST /files?path=new_file.txt (Body: This is new content.)
    -   **Response (201 Created):** Empty body on success.
    -   **Error (500 Internal Server Error):** If write fails (e.g., permissions).

### 2.2. Files WebSocket API

**Endpoint:** ws://<host>:<port>/files

### Client-to-Server Messages (JSON)

All requests require action and path. content is specific to write.

-   **List Directory:**
    { "action": "list", "path": "docs/" }
-   **Read File:**
    { "action": "read", "path": "docs/report.pdf" }
-   **Write File:**
    { "action": "write", "path": "temp/draft.txt", "content": "UGxhaW4gdGV4dCBpbiBCYXNlNjQ=" }
    content *must* be Base64 encoded.
-   **Watch Directory for Changes:**
    { "action": "watch", "path": "watched_folder/" }
    No immediate response. Server will send notify messages for changes.

### Server-to-Client Messages (JSON)

All responses include action, path, and optionally error or data.

-   **Response to 'list' Action:**
    {
      "action": "list",
      "path": "docs/",
      "error": "",
      "data": [{"name":"report.pdf","isDir":false,"size":102400,"modTime":1678886400}]
    }
-   **Response to 'read' Action:**
    {
      "action": "read",
      "path": "docs/report.pdf",
      "error": "",
      "data": "JVBERi0xLjQKJdPr6eUK..." // Base64 encoded file content
    }
-   **Response to 'write' Action:**
    {
      "action": "write",
      "path": "temp/draft.txt",
      "error": "", // Empty string for no error
      "data": null
    }
-   **Error Response (for any action):**
    {
      "action": "read",
      "path": "nonexistent.txt",
      "error": "open nonexistent.txt: no such file or directory",
      "data": null
    }
-   **Asynchronous File System Notification ('notify' action):**
    -   Sent by server when changes occur in a watched directory.
    {
      "action": "notify",
      "path": "watched_folder/new_file.txt",
      "data": "CREATE" // Or "WRITE", "REMOVE", "RENAME"
    }

## 3. Upcheck API (/up)

**Purpose:** Health check endpoint.
**Endpoint:** http://<host>:<port>/up (e.g., http://localhost:3022/up)
**Method:** GET
**Response (200 OK):**
{
  "status": "running",
  "version": "0.1.0",
  "uptime_seconds": 3600.123,
  "active_connections": 5
}

## Error Handling

-   **HTTP REST:** Standard HTTP status codes (e.g., 401 Unauthorized, 403 Forbidden, 404 Not Found, 500 Internal Server Error) with descriptive plaintext or JSON bodies.
-   **WebSocket:** Error messages are embedded in the error field of the fileResponse object.
-   **Authorization Failures:** 401 Unauthorized for REST, WebSocket connection will fail during upgrade with appropriate logging server-side.

## 4. Installation & Uninstallation APIs

These APIs facilitate user-level and system-level installation of Conduit.

**Security Note:** All installation/uninstallation HTTP endpoints are **strictly limited to requests originating from 127.0.0.1 (localhost)**, regardless of API key presence, for security reasons.

### 4.1. Install User (CLI & HTTP)

**Purpose:** Installs Conduit as a user-level application and registers its `conduit://` protocol handler. Does not require elevated permissions.

**CLI Usage:**
```bash
./conduit --install-user
```

**HTTP Endpoint:**
-   **Endpoint:** http://<host>:<port>/install-user (e.g., http://localhost:3022/install-user)
-   **Method:** GET
-   **Response (200 OK):** A plaintext message detailing the installation steps and outcome.
-   **Error (403 Forbidden):** If not from localhost.
-   **Error (500 Internal Server Error):** If installation fails (e.g., not a compiled build, OS not supported).

### 4.2. Uninstall (CLI & HTTP)

**Purpose:** Attempts to remove both user-level and (if applicable and run with root) system-level installations of Conduit.

**CLI Usage:**
```bash
./conduit --uninstall
```
-   **Endpoint:** http://<host>:<port>/uninstall (e.g., http://localhost:3022/uninstall)
-   **Method:** GET
-   **Response (200 OK):** A plaintext message detailing the uninstallation steps and outcome.
-   **Error (403 Forbidden):** If not from localhost.
-   **Error (500 Internal Server Error):** If uninstallation encounters issues (e.g., OS not supported).

### 4.3. Launching Installed Conduit from PWA

Once Conduit is installed with its custom protocol handler (`conduit://`), you can launch it directly from your PWA using `window.open()`:

```javascript
window.open('conduit://', '_blank');
// You can pass data via the URL, e.g., 'conduit://open-terminal?cwd=/path/to/project'
```
**Note:** The `_blank` target is often required by browsers for custom URL schemes to prevent navigation away from the current PWA context. The specific behavior (e.g., prompt for user permission) depends on the user's operating system and browser settings.

## 5. Kill Switch API (/kill)

**Purpose:** Forcefully shuts down the Conduit server process.

**Security Note:** This endpoint is **strictly limited to requests originating from 127.0.0.1 (localhost)**. It is **disabled** if the server was started with the `--no-idle-shutdown` flag.

-   **Endpoint:** http://<host>:<port>/kill (e.g., http://localhost:3022/kill)
-   **Method:** GET
-   **Response (200 OK):** A plaintext message confirming shutdown initiated.
-   **Error (403 Forbidden):** If not from localhost.
-   **Error (500 Internal Server Error):** Should not typically occur unless `os.Exit` itself has issues.
