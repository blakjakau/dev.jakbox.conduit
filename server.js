const express = require('express');
const http = require('http');
const expressWs = require('express-ws');
const pty = require('node-pty');
const os = require('os');

// Determine shell based on OS, default to bash
const shell = os.platform() === 'win32' ? 'powershell.exe' : 'bash';

// --- Server Setup ---
const app = express();
const server = http.createServer(app);
expressWs(app, server);

const port = 3022;

// --- Terminal WebSocket Endpoint ---
app.ws('/terminal', (ws, req) => {
    console.log('New WebSocket connection established.');
    
    // Spawn a new pseudo-terminal
    const term = pty.spawn(shell, [], {
        name: 'xterm-256color', // The 'name' property sets the TERM variable.
        cols: 80,
        rows: 30,
        cwd: process.env.HOME,
        env: process.env
    });

    // console.log(`PTY process created with PID: ${term.pid}`); // Re-enable for debugging pty process

    // Pipe data from PTY to WebSocket
    term.on('data', (data) => {
        try {
            ws.send(data);
        } catch (ex) {
            // The WebSocket is not open, ignore
        }
    });

    // Handle incoming messages from the WebSocket client
    ws.on('message', (msg) => {
        try {
            // Attempt to parse as JSON for structured messages (resize)
            const message = JSON.parse(msg.toString()); // Parse incoming message as JSON (ensure msg is string)
            if (message.type === 'resize' && message.cols && message.rows) {
                term.resize(message.cols, message.rows);
            } else if (message.type === 'data' && typeof message.content === 'string') {
                term.write(message.content); // Write the actual terminal data to the PTY
            }
        } catch (e) {
            // Non-JSON messages are usually raw terminal input if not handled as type 'data' above,
            // but our frontend now sends all data as JSON with type 'data'.
            // So, log unexpected messages, but don't assume raw data input here.
            console.warn('Received unexpected or malformed WebSocket message (not JSON):', msg.toString(), 'Error:', e.message);
        }
    });

    // Handle WebSocket closure
    ws.on('close', () => {
        console.log(`Closing connection. Killing PTY process (PID: ${term.pid}).`);
        term.kill();
    });
});


// --- Start Server ---
server.listen(port, '127.0.0.1', () => {
    console.log(`Terminal server listening on ws://localhost:${port}/terminal`);
});
