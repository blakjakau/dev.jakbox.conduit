# dev.jakbox.conduit
(experimental) proxy for xterm integration in cadence

After failing to complile a standalone executable in nodejs using `pkg` or `SEA`, and an unecessarily long jaunt in Dart (which refused to hand off the pty session with BASH properly) I've landed on Go as the language for Conduit (for now).

Precompiled binaries for major platform come in at ~5md each (compared to 8mb for Dart and 120mb for Node SEA). It's fast and stable. 

The node and dart code is still in this repo for future reference, and possible work.

## working with Dart
```bash
# fetch dependancies
dart pub get

# run the server
dart run server.dart

# compile the sever
dart compile exe server.dart -o conduit-linux
```

## working with go
```bash
# fetch dependancies
go get

# run the server
go run .

#compile the server
# For standard 64-bit Linux (most common)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o conduit-linux-x64 .

# For 64-bit ARM Linux (like Raspberry Pi 4, some servers)
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o conduit-linux-arm64 .

# For Intel Macs
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o conduit-macos-x64 .

# For Apple Silicon (M1/M2/M3) Macs
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o conduit-macos-arm64 .

# For 64-bit Windows
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o conduit-windows-x64.exe .

# one shot the all of the above? why not
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o conduit-linux-x64 .; GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o conduit-linux-arm64 .; GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o conduit-macos-x64 .; GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o conduit-macos-arm64 .; GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o conduit-windows-x64.exe .;
```
