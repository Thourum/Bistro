#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

static WKWebView *findWebView(NSView *view) {
    if ([view isKindOfClass:[WKWebView class]]) return (WKWebView *)view;
    for (NSView *child in view.subviews) {
        WKWebView *found = findWebView(child);
        if (found) return found;
    }
    return nil;
}

void installProbe(void *handle, const char *source) {
    NSWindow *window = (NSWindow *)handle;
    WKWebView *view = findWebView(window.contentView);
    if (!view) { fprintf(stderr, "missing Wails WKWebView\n"); exit(2); }
    [view.configuration.userContentController addUserScript:[[WKUserScript alloc]
        initWithSource:[NSString stringWithUTF8String:source]
        injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES]];
    if (@available(macOS 13.3, *)) view.inspectable = YES;
    // Match the Gate 2 content viewport, independent of Wails' native frame conversion.
    [window setContentSize:NSMakeSize(1600, 900)];
    [view setFrame:NSMakeRect(0, 0, 1600, 900)];
    [window center];
}

void reportProbe(void *handle) {
    NSWindow *window = (NSWindow *)handle;
    WKWebView *view = findWebView(window.contentView);
    if (!view) { fprintf(stderr, "missing Wails WKWebView\n"); exit(2); }
    [window setContentMinSize:NSMakeSize(1600, 900)];
    [window setContentMaxSize:NSMakeSize(1600, 900)];
    [window setContentSize:NSMakeSize(1600, 900)];
    [view setFrame:NSMakeRect(0, 0, 1600, 900)];
    NSArray *targets = @[
        @[view, @"_webProcessIdentifier", @"webcontent_pid"],
        @[view, @"_gpuProcessIdentifier", @"gpu_pid"],
        @[view.configuration.websiteDataStore, @"_networkProcessIdentifier", @"networking_pid"]
    ];
    // Throwaway evidence-only private SPI: launchd ancestry cannot attribute WebKit XPCs.
    for (NSArray *target in targets) {
        if (![target[0] respondsToSelector:NSSelectorFromString(target[1])]) {
            printf("{\"event\":\"pid_unavailable\"}\n"); continue;
        }
        NSNumber *pid = [target[0] valueForKey:target[1]];
        printf("{\"event\":\"%s\",\"pid\":%d}\n", [target[2] UTF8String], pid.intValue);
    }
    NSDictionary *state = @{@"event": @"window_state", @"isKey": @(window.isKeyWindow),
        @"isVisible": @((BOOL)((window.occlusionState & NSWindowOcclusionStateVisible) != 0)),
        @"width": @(view.bounds.size.width), @"height": @(view.bounds.size.height),
        @"originURL": view.URL.absoluteString ?: @"", @"isInspectable": @(view.inspectable)};
    NSData *bytes = [NSJSONSerialization dataWithJSONObject:state options:0 error:nil];
    fwrite(bytes.bytes, 1, bytes.length, stdout); printf("\n"); fflush(stdout);
}
