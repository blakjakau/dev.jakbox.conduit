//go:build darwin
// +build darwin
package main
/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include "handler_darwin.h"
*/
import "C"
import (
	"log"
	"net/url"
	"sync"
)
var (
	urlHandleFunc func(*url.URL)
	urlLock       sync.Mutex
	urlRunLoop    = make(chan bool)
)
// setURLHandler assigns the function to be called when a URL is received.
func setURLHandler(h func(*url.URL)) {
	urlLock.Lock()
	defer urlLock.Unlock()
	urlHandleFunc = h
}
//export handle
func handle(u *C.char) {
	urlLock.Lock()
	defer urlLock.Unlock()
	uri, err := url.Parse(C.GoString(u))
	if err != nil {
		return
	}
	if urlHandleFunc != nil {
		urlHandleFunc(uri)
	}
}
// registerAndRunURLHandler registers the custom URL protocol handler with macOS
// and then blocks indefinitely, waiting for events.
func registerAndRunURLHandler() {
	C.registerURLHandler()
	// Block forever to keep the run loop active for URL events
	<-urlRunLoop
}
func main() {
	// On macOS, we must explicitly activate the application to signal to the
	// operating system that we have launched and are ready. This is what
	// dismisses the "opening application" dialog immediately.
	C.activateApp()

	var startServerOnce sync.Once
	setURLHandler(func(url *url.URL) {
		log.Printf("Received URL via macOS protocol handler: %s", url.String())
		startServerOnce.Do(func() { go runConduitServer() })
	})
	startServerOnce.Do(func() { go runConduitServer() })
	registerAndRunURLHandler()
}
