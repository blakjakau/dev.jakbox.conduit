#ifndef HANDLER_DARWIN_H
#define HANDLER_DARWIN_H

#import <Cocoa/Cocoa.h>

// registerURLHandler is defined in handler_darwin.m and called from main_darwin.go
void registerURLHandler();
// activateApp brings the application to the foreground.
void activateApp();

#endif // HANDLER_DARWIN_H
