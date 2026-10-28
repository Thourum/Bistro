// Native S6 driver. stdout is JSONL; never request TCC permissions or enable sources.
#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <Carbon/Carbon.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <unistd.h>

static const int PollMilliseconds = 100;
static const NSUInteger MaximumCommandBytes = 16384;
static volatile sig_atomic_t ExitSignal = 0;
static void onSignal(int number) { ExitSignal = number; }

static BOOL emit(NSDictionary *record) {
    NSError *error = nil;
    NSData *json = [NSJSONSerialization dataWithJSONObject:record options:0 error:&error];
    if (!json) return NO;
    NSMutableData *line = [json mutableCopy];
    [line appendBytes:"\n" length:1];
    const uint8_t *bytes = line.bytes;
    NSUInteger remaining = line.length;
    // A wedged JSONL reader must not prevent input-source restoration.
    const CFTimeInterval OutputDeadlineSeconds = 1.0;
    CFAbsoluteTime deadline = CFAbsoluteTimeGetCurrent() + OutputDeadlineSeconds;
    while (remaining && !ExitSignal) {
        ssize_t written = write(STDOUT_FILENO, bytes, remaining);
        if (written < 0 && errno == EINTR) continue;
        if (written < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
            if (CFAbsoluteTimeGetCurrent() >= deadline) { ExitSignal = SIGPIPE; return NO; }
            struct pollfd descriptor = {STDOUT_FILENO, POLLOUT, 0};
            poll(&descriptor, 1, PollMilliseconds);
            continue;
        }
        if (written <= 0) { ExitSignal = SIGPIPE; return NO; }
        bytes += written;
        remaining -= (NSUInteger)written;
    }
    if (remaining) return NO;
    return YES;
}

static pid_t frontmostPID(void) { return NSWorkspace.sharedWorkspace.frontmostApplication.processIdentifier; }
static BOOL guardPID(pid_t expected, NSString **error) {
    pid_t actual = frontmostPID();
    if (expected > 0 && actual == expected && kill(expected, 0) == 0) return YES;
    *error = [NSString stringWithFormat:@"frontmost PID mismatch: expected %d, actual %d; run aborted", expected, actual];
    return NO;
}
static BOOL parsePID(NSString *text, pid_t *pid) {
    if (!text.length || [text rangeOfCharacterFromSet:NSCharacterSet.decimalDigitCharacterSet.invertedSet].location != NSNotFound) return NO;
    unsigned long long value = text.longLongValue;
    if (!value || value > INT_MAX) return NO;
    *pid = (pid_t)value;
    return YES;
}
static BOOL parseModifiers(id modifiers, CGEventFlags *flags, NSString **error) {
    if (![modifiers isKindOfClass:NSArray.class]) { *error = @"modifiers must be an array"; return NO; }
    NSDictionary *names = @{@"cmd":@(kCGEventFlagMaskCommand), @"meta":@(kCGEventFlagMaskCommand),
        @"ctrl":@(kCGEventFlagMaskControl), @"control":@(kCGEventFlagMaskControl),
        @"alt":@(kCGEventFlagMaskAlternate), @"option":@(kCGEventFlagMaskAlternate), @"shift":@(kCGEventFlagMaskShift)};
    *flags = 0;
    for (id name in modifiers) {
        if (![name isKindOfClass:NSString.class] || !names[name]) { *error = @"unknown modifier (use cmd, ctrl, alt, shift)"; return NO; }
        *flags |= [names[name] unsignedLongLongValue];
    }
    return YES;
}
#include "input-sources.h"

@interface Session : NSObject
@property(nonatomic) pid_t pid;
@property(nonatomic) pid_t parentPID;
@property(nonatomic, strong) id originalSource;
@property(nonatomic, strong) id originalLayout;
@property(nonatomic) BOOL changedSource;
- (BOOL)selectSource:(NSString *)identifier error:(NSString **)error;
- (NSDictionary *)inject:(NSDictionary *)command error:(NSString **)error;
- (BOOL)restore;
@end
@implementation Session
- (instancetype)init {
    if ((self = [super init])) {
        _originalSource = CFBridgingRelease(TISCopyCurrentKeyboardInputSource());
        _originalLayout = CFBridgingRelease(TISCopyCurrentKeyboardLayoutInputSource());
        _parentPID = getppid();
    }
    return self;
}
- (BOOL)selectSource:(NSString *)identifier error:(NSString **)error {
    if (!guardPID(self.pid, error)) return NO;
    if (![identifier isKindOfClass:NSString.class] || !identifier.length) { *error = @"source must be a nonempty TIS ID"; return NO; }
    NSArray *sources = installedSources();
    TISInputSourceRef source = findSource(identifier, sources);
    if (!source || ![property(source, kTISPropertyInputSourceIsEnabled) boolValue] ||
        ![property(source, kTISPropertyInputSourceIsSelectCapable) boolValue]) {
        *error = [NSString stringWithFormat:@"source %@ missing, disabled, or unselectable; no sources enabled automatically", identifier];
        return NO;
    }
    // Restore even if a failed selection partially changed the underlying IME layout.
    if (ExitSignal || getppid() != self.parentPID) { *error = @"session interrupted or parent exited"; return NO; }
    if (!guardPID(self.pid, error)) return NO;
    self.changedSource = YES;
    OSStatus status = TISSelectInputSource(source);
    if (status != noErr) { *error = [NSString stringWithFormat:@"TISSelectInputSource failed: %d", (int)status]; return NO; }
    id selected = CFBridgingRelease(TISCopyCurrentKeyboardInputSource());
    if (![sourceID((__bridge TISInputSourceRef)selected) isEqual:identifier]) { *error = @"selected source did not match requested ID"; return NO; }
    return guardPID(self.pid, error);
}
- (NSDictionary *)inject:(NSDictionary *)command error:(NSString **)error {
    if (!guardPID(self.pid, error)) return nil;
    if (!CGPreflightPostEventAccess()) { *error = @"PostEvent permission missing (no permission requested)"; return nil; }
    id key = command[@"key"];
    if (![key isKindOfClass:NSString.class] || ![key length]) { *error = @"key must be a nonempty string"; return nil; }
    CGEventFlags flags;
    if (!parseModifiers(command[@"modifiers"] ?: @[], &flags, error)) return nil;
    if (![self selectSource:command[@"source"] error:error]) return nil;
    id layout = CFBridgingRelease(TISCopyCurrentKeyboardLayoutInputSource());
    CGKeyCode code;
    if (!resolveKey(key, (__bridge TISInputSourceRef)layout, &code, &flags, error)) return nil;
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, code, true);
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, code, false);
    if (!down || !up) {
        if (down) CFRelease(down);
        if (up) CFRelease(up);
        *error = @"CGEvent allocation failed"; return nil;
    }
    CGEventSetFlags(down, flags);
    CGEventSetFlags(up, flags);
    BOOL isGuarded = guardPID(self.pid, error) && !ExitSignal && getppid() == self.parentPID;
    if (isGuarded) CGEventPost(kCGHIDEventTap, down);
    // No further injection on a focus mismatch, including keyUp.
    isGuarded = isGuarded && guardPID(self.pid, error) && !ExitSignal && getppid() == self.parentPID;
    if (isGuarded) CGEventPost(kCGHIDEventTap, up);
    CFRelease(down);
    CFRelease(up);
    if (!isGuarded) { if (!*error) *error = @"interrupted before injection completed"; return nil; }
    return @{@"posted":@YES, @"keycode":@(code), @"flags":@(flags), @"source":command[@"source"],
             @"layout":sourceID((__bridge TISInputSourceRef)layout), @"key":key};
}
- (BOOL)restore {
    if (!self.changedSource) return YES;
    // Restore the underlying keyboard layout before re-selecting an original IME mode.
    OSStatus layoutStatus = self.originalLayout ? TISSelectInputSource((__bridge TISInputSourceRef)self.originalLayout) : noErr;
    OSStatus sourceStatus = self.originalSource ? TISSelectInputSource((__bridge TISInputSourceRef)self.originalSource) : paramErr;
    id selected = CFBridgingRelease(TISCopyCurrentKeyboardInputSource());
    BOOL restored = sourceStatus == noErr && layoutStatus == noErr &&
        [sourceID((__bridge TISInputSourceRef)selected) isEqual:sourceID((__bridge TISInputSourceRef)self.originalSource)];
    emit(@{@"event":@"restored", @"ok":@(restored), @"sourceStatus":@(sourceStatus), @"layoutStatus":@(layoutStatus),
           @"source":sourceID((__bridge TISInputSourceRef)selected)});
    self.changedSource = NO;
    return restored;
}
@end

static BOOL guardSession(Session *session) {
    NSString *error = nil;
    if (guardPID(session.pid, &error)) return YES;
    emit(@{@"event":@"aborted", @"ok":@NO, @"error":error});
    ExitSignal = SIGTERM;
    return NO;
}

static CGEventRef tapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *context) {
    (void)proxy;
    Session *session = (__bridge Session *)context;
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        emit(@{@"event":@"tap_disabled", @"type":@(type)});
        ExitSignal = SIGTERM;
        return event;
    }
    pid_t foreground = frontmostPID();
    pid_t target = (pid_t)CGEventGetIntegerValueField(event, kCGEventTargetUnixProcessID);
    if (!guardSession(session)) return event;
    UniChar characters[64];
    UniCharCount count = 0;
    CGEventKeyboardGetUnicodeString(event, sizeof(characters) / sizeof(characters[0]), &count, characters);
    NSDictionary *types = @{@(kCGEventKeyDown):@"keydown", @(kCGEventKeyUp):@"keyup", @(kCGEventFlagsChanged):@"flagsChanged"};
    emit(@{@"event":@"cg_event", @"type":types[@(type)] ?: @"unknown", @"timestampNs":@(CGEventGetTimestamp(event)),
        @"keycode":@(CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode)), @"flags":@(CGEventGetFlags(event)),
        @"unicode":[NSString stringWithCharacters:characters length:count], @"frontmostPID":@(foreground),
        @"targetPID":@(target), @"sourcePID":@(CGEventGetIntegerValueField(event, kCGEventSourceUnixProcessID)),
        @"autorepeat":@(CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat))});
    return event;
}
static CFMachPortRef createTap(Session *session) {
    CGEventMask mask = CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventKeyUp) | CGEventMaskBit(kCGEventFlagsChanged);
    return CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionListenOnly,
                           mask, tapCallback, (__bridge void *)session);
}
static int runTap(Session *session) {
    CFMachPortRef tap = createTap(session);
    if (!tap) { emit(@{@"ok":@NO, @"error":@"listen-only CGEventTap unavailable"}); return 1; }
    CFRunLoopSourceRef loopSource = CFMachPortCreateRunLoopSource(NULL, tap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), loopSource, kCFRunLoopCommonModes);
    emit(@{@"event":@"tap_ready", @"pid":@(session.pid), @"listenOnly":@YES});
    while (!ExitSignal && getppid() == session.parentPID) {
        if (!guardSession(session)) break;
        CFRunLoopRunInMode(kCFRunLoopDefaultMode, (double)PollMilliseconds / 1000.0, false);
    }
    CFRunLoopRemoveSource(CFRunLoopGetCurrent(), loopSource, kCFRunLoopCommonModes);
    CFRelease(loopSource);
    CFMachPortInvalidate(tap);
    CFRelease(tap);
    return ExitSignal ? 128 + ExitSignal : 0;
}

static BOOL executeCommand(Session *session, NSDictionary *command, BOOL *shouldStop) {
    NSString *error = nil;
    NSString *operation = command[@"command"];
    NSDictionary *result = nil;
    if ([operation isEqual:@"guard"]) {
        if (guardPID(session.pid, &error)) result = @{@"pid":@(session.pid)};
    } else if ([operation isEqual:@"source"]) {
        if ([session selectSource:command[@"source"] error:&error]) result = @{@"source":command[@"source"]};
    } else if ([operation isEqual:@"inject"]) {
        result = [session inject:command error:&error];
    } else if ([operation isEqual:@"stop"]) {
        *shouldStop = YES;
        result = @{};
    } else error = @"unknown command (use source, inject, guard, stop)";
    NSMutableDictionary *reply = [@{@"event":@"reply", @"ok":@(result != nil)} mutableCopy];
    if (command[@"id"]) reply[@"id"] = command[@"id"];
    if (result) reply[@"result"] = result;
    else reply[@"error"] = error ?: @"command failed";
    return emit(reply) && result != nil;
}
static int runWorker(Session *session) {
    emit(@{@"event":@"worker_ready", @"pid":@(session.pid),
           @"originalSource":sourceID((__bridge TISInputSourceRef)session.originalSource)});
    NSMutableData *pending = [NSMutableData data];
    BOOL shouldStop = NO;
    while (!shouldStop && !ExitSignal && getppid() == session.parentPID) {
        if (!guardSession(session)) break;
        struct pollfd descriptor = {STDIN_FILENO, POLLIN, 0};
        int status = poll(&descriptor, 1, PollMilliseconds);
        if (status < 0 && errno == EINTR) continue;
        if (status < 0) { emit(@{@"ok":@NO, @"error":@"stdin poll failed"}); return 1; }
        if (!status) continue;
        uint8_t bytes[4096];
        ssize_t count = read(STDIN_FILENO, bytes, sizeof(bytes));
        if (count < 0 && errno == EINTR) continue;
        if (count < 0) { emit(@{@"ok":@NO, @"error":@"stdin read failed"}); return 1; }
        if (!count) {
            if (pending.length) { emit(@{@"ok":@NO, @"error":@"incomplete JSONL command at EOF"}); return 1; }
            break;
        }
        [pending appendBytes:bytes length:(NSUInteger)count];
        while (!shouldStop && !ExitSignal && getppid() == session.parentPID) {
            if (!guardSession(session)) break;
            const uint8_t *start = pending.bytes;
            const uint8_t *newline = memchr(start, '\n', pending.length);
            if (!newline) break;
            NSUInteger length = (NSUInteger)(newline - start);
            if (length > MaximumCommandBytes) { emit(@{@"ok":@NO, @"error":@"command exceeds 16 KiB"}); return 1; }
            NSData *line = [pending subdataWithRange:NSMakeRange(0, length)];
            [pending replaceBytesInRange:NSMakeRange(0, length + 1) withBytes:NULL length:0];
            NSError *parseError = nil;
            id command = [NSJSONSerialization JSONObjectWithData:line options:0 error:&parseError];
            if (![command isKindOfClass:NSDictionary.class] || ![command[@"command"] isKindOfClass:NSString.class]) {
                emit(@{@"ok":@NO, @"error":@"command must be a JSON object with a string command"}); return 1;
            }
            if (!executeCommand(session, command, &shouldStop)) return 1;
        }
        if (pending.length > MaximumCommandBytes) { emit(@{@"ok":@NO, @"error":@"command exceeds 16 KiB"}); return 1; }
    }
    return ExitSignal ? 128 + ExitSignal : 0;
}

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        signal(SIGINT, onSignal);
        signal(SIGTERM, onSignal);
        signal(SIGHUP, onSignal);
        signal(SIGQUIT, onSignal);
        signal(SIGPIPE, SIG_IGN);
        int outputFlags = fcntl(STDOUT_FILENO, F_GETFL);
        if (outputFlags < 0 || fcntl(STDOUT_FILENO, F_SETFL, outputFlags | O_NONBLOCK) < 0) return 1;
        if (argc < 2) { emit(@{@"ok":@NO, @"error":@"usage: bistro-keys preflight|sources|self-test|guard|tap|inject|worker [--pid PID] [--source ID --key KEY --modifiers cmd,ctrl,alt,shift]"}); return 2; }
        NSString *operation = @(argv[1]);
        if ([operation isEqual:@"sources"]) { emit(sourceInventory()); return 0; }
        if ([operation isEqual:@"self-test"]) return selfTest();
        Session *session = [Session new];
        if ([operation isEqual:@"preflight"]) {
            CFMachPortRef tap = createTap(session);
            BOOL canTap = tap != NULL;
            if (tap) { CFMachPortInvalidate(tap); CFRelease(tap); }
            emit(@{@"event":@"preflight", @"listenPreflight":@(CGPreflightListenEventAccess()),
                @"passiveTapCreated":@(canTap), @"postEventAccess":@(CGPreflightPostEventAccess()),
                @"accessibilityTrusted":@(AXIsProcessTrusted()), @"frontmostPID":@(frontmostPID()),
                @"originalSource":sourceID((__bridge TISInputSourceRef)session.originalSource),
                @"inventory":sourceInventory(), @"requestedPermissions":@NO});
            return 0;
        }
        NSSet *operations = [NSSet setWithArray:@[@"guard", @"tap", @"inject", @"worker"]];
        if (![operations containsObject:operation]) { emit(@{@"ok":@NO, @"error":@"unknown CLI operation"}); return 2; }
        NSMutableDictionary *options = [NSMutableDictionary dictionary];
        NSSet *allowed = [NSSet setWithArray:@[@"--pid", @"--source", @"--key", @"--modifiers"]];
        for (int index = 2; index < argc; index += 2) {
            NSString *name = @(argv[index]);
            if (index + 1 >= argc || ![allowed containsObject:name] || options[name]) {
                emit(@{@"ok":@NO, @"error":@"invalid, duplicate, or incomplete CLI option"}); return 2;
            }
            options[name] = @(argv[index + 1]);
        }
        pid_t pid;
        if (!parsePID(options[@"--pid"], &pid)) { emit(@{@"ok":@NO, @"error":@"--pid must be a positive process ID"}); return 2; }
        session.pid = pid;
        NSString *error = nil;
        if (!guardPID(pid, &error)) { emit(@{@"ok":@NO, @"error":error}); return 1; }
        int status = 0;
        if ([operation isEqual:@"tap"]) status = runTap(session);
        else if ([operation isEqual:@"worker"]) status = runWorker(session);
        else {
            NSString *modifierText = options[@"--modifiers"] ?: @"";
            NSDictionary *command = @{@"command":operation, @"source":options[@"--source"] ?: @"",
                @"key":options[@"--key"] ?: @"", @"modifiers":modifierText.length ? [modifierText componentsSeparatedByString:@","] : @[]};
            BOOL shouldStop = NO;
            status = executeCommand(session, command, &shouldStop) ? 0 : 1;
        }
        if (![session restore]) status = 1;
        return status;
    }
}
