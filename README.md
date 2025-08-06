# dev.jakbox.conduit
(experimental) proxy for xterm integration in cadence

After failing to complile a standalone executable in nodejs using `pkg` or `SEA`, and an unecessarily long jaunt in Dart (which refused to hand off the pty session with BASH properly) I've landed on Go as the language for Conduit (for now).

Precompiled binaries for major platform come in at ~5mb each (compared to 8mb for Dart and 120mb for Node SEA). It's fast and stable. 

## What does Contuit do?
Conduit provides a channel for the editor to talk to the PTY (psuedotermianl) of your computer via websockets

On Linux and macOS this is done though `github.com/creack/pty` which handles io streams from the system's native PTY functionality

On Windows `github.com/UserExistsError/conpty` handles talking to the ConPTY functionality of windows 11

There is no support for Windows 10 or older.

Conduit also provides a file access API

## how to use it?

Check [implementation-guide.md](./implementation-guide.md) for specifics, basically the running binary creates a listener on port 3022, exposing some end points and a websocket service.
