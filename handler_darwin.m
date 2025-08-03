#import "handler_darwin.h"
#import <stdlib.h>

// handle is a Go function exported to C via //export
extern void handle(char* u);

// URLHandlerDelegate handles the URL event from macOS
@interface URLHandlerDelegate : NSObject
- (void)handleEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)replyEvent;
@end

@implementation URLHandlerDelegate
- (void)handleEvent:(NSAppleEventDescriptor *)event withReplyEvent:(NSAppleEventDescriptor *)replyEvent {
    NSString *urlString = [[event paramDescriptorForKeyword:keyDirectObject] stringValue];
    if (urlString) {
        handle((char*)[urlString UTF8String]);
    }
}
@end

// registerURLHandler sets up the event handler
void registerURLHandler() {
    @autoreleasepool {
        static URLHandlerDelegate *delegate = nil;
        if (delegate == nil) {
            delegate = [[URLHandlerDelegate alloc] init];
        }
        
        [[NSAppleEventManager sharedAppleEventManager]
            setEventHandler:delegate
            andSelector:@selector(handleEvent:withReplyEvent:)
            forEventClass:kInternetEventClass
            andEventID:kAEGetURL];
    }
}

// activateApp makes the app the frontmost application.
void activateApp() {
    @autoreleasepool {
        // This call is what signals to the OS that we are "active".
        [NSApp activateIgnoringOtherApps:YES];
    }
}
