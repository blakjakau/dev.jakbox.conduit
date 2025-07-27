import 'dart:io';
import 'dart:convert'; // For decoding JSON messages
import 'package:shelf/shelf.dart' as shelf;
import 'package:shelf/shelf_io.dart' as io;
import 'package:shelf_web_socket/shelf_web_socket.dart';
import 'package:pty/pty.dart';
  
void main() async {
	// Define connection counters and the server version.
	var activeConnections = 0;
	var sessionIdCounter = 0;
	const version = '0.1.0';
	
	const allowedOrigins = [
		'https://code.jakbox.dev', // Your production editor
		'http://localhost:8083',
		'http://localhost'
		
	];
	// This is a Shelf middleware to check the Origin header.
	final checkOrigin = shelf.createMiddleware(requestHandler: (request) {
		final origin = request.headers['origin'];
		if (allowedOrigins.contains(origin)) {
			return null; // A null response means "proceed".
		}
		return shelf.Response.forbidden('Invalid origin: $origin');
	});
  
	// We'll remove the noisy `logRequests` middleware for a cleaner output.
	final handler = shelf.Pipeline().addMiddleware(checkOrigin).addHandler(
		webSocketHandler((webSocket) {
		activeConnections++;
		sessionIdCounter++;
		final sessionId = sessionIdCounter; // Capture the ID for this session
		final timestamp = DateTime.now().toIso8601String();
		print('[$timestamp] Client #$sessionId (of $activeConnections) connected');
		
		// Determine the shell to use based on the OS.
		final shell = Platform.isWindows ? 'powershell.exe' : 'script';
		
		// Spawn the pseudo-terminal process using the dedicated 'pty' package.
		// THE NUCLEAR OPTION: Use `script` to create a new, clean PTY for bash.
		// -q (quiet), /dev/null (don't write a log file), bash (command to run).
		final args = Platform.isWindows ? <String>[] : ['-q', '', 'bash'];
		final pty = PseudoTerminal.start(shell, args,
					environment: {'TERM': 'xterm-256color'});
		
		// Pipe data from the PTY's single output stream directly to the WebSocket.
		// So much cleaner!
		pty.out.listen(webSocket.sink.add);
		
		// Pipe data from the WebSocket to the PTY's input.
		webSocket.stream.listen(
			(data) {
				var isCommand = false;
				if (data is String) {
					try {
						final msg = json.decode(data);
						if (msg is Map) {
							// Check for a 'resize' command
							if (msg['type'] == 'resize' && msg['cols'] is int && msg['rows'] is int) {
								// This resize command is now handled silently.
								pty.resize(msg['cols'], msg['rows']);
								isCommand = true; // Mark that we've handled this message.
								// Check for a 'data' command for terminal input
							} else if (msg['type'] == 'data' && msg['content'] is String) {
								pty.write(msg['content']);
								isCommand = true;
							}
						}
					} catch (e) {/* Not valid JSON, treat as raw input below. */}
				}
				
				// If the message wasn't a command we recognized, pass it to the PTY.
				if (!isCommand) {
					pty.write(data);
				}
			},
			onDone: () {
				// Client closed the connection, so we kill the PTY.
				// The `pty.exitCode.then` block below will handle the cleanup and logging.
				pty.kill();
			},
			onError: (error) {
				print('WebSocket error: $error. Killing PTY process.');
				pty.kill();
			},
		);
	
		// This is our single source of truth for handling disconnections.
		pty.exitCode.then((code) {
			activeConnections--;
			final timestamp = DateTime.now().toIso8601String();
			print('[$timestamp] Client #$sessionId (of $activeConnections) disconnected');
			
			try {
				// Let the client know the session has ended.
				webSocket.sink.add('[exit]');
				
				// Close the WebSocket with a standard code, not the raw PTY exit code.
				webSocket.sink.close();
			} catch (e) {
				// The WebSocket might already be closed, which is fine.
			}
		});
	}),
	);
	  
	// Use `io.serve` to start the server.
	final server = await io.serve(handler, '127.0.0.1', 3022);
	print('Conduit v$version - listening for WS connections (localhost:3022)');
	print('--------------------------------------------------------------');
}
