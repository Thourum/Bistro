// TIS inventory and passive layout translation checks for the native S6 helper.
static id property(TISInputSourceRef source, CFStringRef key) {
    return (__bridge id)TISGetInputSourceProperty(source, key);
}
static NSString *sourceID(TISInputSourceRef source) {
    return property(source, kTISPropertyInputSourceID) ?: @"";
}
static NSArray *installedSources(void) {
    return CFBridgingRelease(TISCreateInputSourceList(
        (__bridge CFDictionaryRef)@{(__bridge NSString *)kTISPropertyInputSourceCategory:
                                   (__bridge NSString *)kTISCategoryKeyboardInputSource}, true));
}
static TISInputSourceRef findSource(NSString *identifier, NSArray *sources) {
    for (id source in sources) {
        if ([sourceID((__bridge TISInputSourceRef)source) isEqual:identifier])
            return (__bridge TISInputSourceRef)source;
    }
    return NULL;
}
static NSDictionary *describeSource(TISInputSourceRef source) {
    return @{@"id":sourceID(source), @"name":property(source, kTISPropertyLocalizedName) ?: @"",
             @"type":property(source, kTISPropertyInputSourceType) ?: @"",
             @"languages":property(source, kTISPropertyInputSourceLanguages) ?: @[],
             @"enabled":property(source, kTISPropertyInputSourceIsEnabled) ?: @NO,
             @"selectable":property(source, kTISPropertyInputSourceIsSelectCapable) ?: @NO,
             @"selected":property(source, kTISPropertyInputSourceIsSelected) ?: @NO,
             @"hasLayoutData":@(property(source, kTISPropertyUnicodeKeyLayoutData) != nil)};
}
static NSDictionary *sourceInventory(void) {
    NSArray *sources = installedSources();
    NSMutableArray *records = [NSMutableArray array];
    for (id source in sources) [records addObject:describeSource((__bridge TISInputSourceRef)source)];
    NSDictionary *requirements = @{@"US":@[@"com.apple.keylayout.US"],
        @"German":@[@"com.apple.keylayout.German"], @"French-AZERTY":@[@"com.apple.keylayout.French"],
        @"Dvorak":@[@"com.apple.keylayout.Dvorak"],
        @"Japanese-active":@[@"com.apple.inputmethod.Kotoeri.RomajiTyping.Japanese",
            @"com.apple.inputmethod.Kotoeri.KanaTyping.Japanese", @"com.apple.inputmethod.Kotoeri.Japanese",
            @"com.apple.inputmethod.Kotoeri.Japanese.Hiragana"],
        @"Japanese-off":@[@"com.apple.inputmethod.Kotoeri.RomajiTyping.Roman",
            @"com.apple.inputmethod.Kotoeri.KanaTyping.Roman", @"com.apple.inputmethod.Kotoeri.Roman",
            @"com.apple.inputmethod.Kotoeri.Japanese.Roman"]};
    NSMutableDictionary *required = [NSMutableDictionary dictionary];
    for (NSString *label in requirements) {
        NSArray *identifiers = requirements[label];
        NSMutableArray *found = [NSMutableArray array], *enabled = [NSMutableArray array];
        for (NSString *identifier in identifiers) {
            TISInputSourceRef source = findSource(identifier, sources);
            if (!source) continue;
            [found addObject:identifier];
            if ([property(source, kTISPropertyInputSourceIsEnabled) boolValue] &&
                [property(source, kTISPropertyInputSourceIsSelectCapable) boolValue]) [enabled addObject:identifier];
        }
        required[label] = @{@"candidateIDs":identifiers, @"installedIDs":found,
                            @"enabledIDs":enabled, @"missingInstalled":@(found.count == 0),
                            @"missingEnabled":@(enabled.count == 0)};
    }
    return @{@"sources":records, @"required":required};
}
static NSDictionary *namedKeys(void) {
    return @{@"Enter":@(kVK_Return), @"Return":@(kVK_Return), @"Escape":@(kVK_Escape),
        @"Tab":@(kVK_Tab), @"Space":@(kVK_Space), @"Backspace":@(kVK_Delete),
        @"Delete":@(kVK_ForwardDelete), @"ArrowLeft":@(kVK_LeftArrow), @"ArrowRight":@(kVK_RightArrow),
        @"ArrowUp":@(kVK_UpArrow), @"ArrowDown":@(kVK_DownArrow), @"Home":@(kVK_Home),
        @"End":@(kVK_End), @"PageUp":@(kVK_PageUp), @"PageDown":@(kVK_PageDown)};
}

// Resolve the logical base key, not Option's output: Option+N must retain native dead-key behavior.
static BOOL resolveKey(NSString *key, TISInputSourceRef layoutSource, CGKeyCode *code,
                       CGEventFlags *flags, NSString **error) {
    NSNumber *named = namedKeys()[key];
    if (named) { *code = named.unsignedShortValue; return YES; }
    if (key.length != 1 || ([key characterAtIndex:0] >= 0xD800 && [key characterAtIndex:0] <= 0xDFFF)) {
        *error = @"key must be one BMP character or a supported named key"; return NO;
    }
    CFDataRef data = (__bridge CFDataRef)property(layoutSource, kTISPropertyUnicodeKeyLayoutData);
    if (!data) { *error = @"selected layout has no UCKeyTranslate data"; return NO; }
    const UCKeyboardLayout *layout = (const UCKeyboardLayout *)CFDataGetBytePtr(data);
    // macOS hardware virtual keys occupy 0..127; search unshifted first, then shifted.
    const UInt16 VirtualKeyCount = 128;
    for (NSUInteger shift = 0; shift <= 1; shift++) {
        for (UInt16 candidate = 0; candidate < VirtualKeyCount; candidate++) {
            UInt32 deadState = 0;
            UniChar characters[4];
            UniCharCount count = 0;
            OSStatus status = UCKeyTranslate(layout, candidate, kUCKeyActionDown,
                shift ? (shiftKey >> 8) : 0, LMGetKbdType(), kUCKeyTranslateNoDeadKeysMask,
                &deadState, sizeof(characters) / sizeof(characters[0]), &count, characters);
            if (status == noErr && count == 1 && characters[0] == [key characterAtIndex:0]) {
                *code = candidate;
                if (shift) *flags |= kCGEventFlagMaskShift;
                return YES;
            }
        }
    }
    *error = [NSString stringWithFormat:@"logical key %@ not found in layout %@", key, sourceID(layoutSource)];
    return NO;
}

static int selfTest(void) {
    NSArray *sources = installedSources();
    NSDictionary *cases = @{@"com.apple.keylayout.US":@[@"a", @(kVK_ANSI_A)],
        @"com.apple.keylayout.German":@[@"y", @(kVK_ANSI_Z)],
        @"com.apple.keylayout.French":@[@"a", @(kVK_ANSI_Q)],
        @"com.apple.keylayout.Dvorak":@[@"j", @(kVK_ANSI_C)]};
    NSMutableArray *checked = [NSMutableArray array];
    for (NSString *identifier in cases) {
        TISInputSourceRef source = findSource(identifier, sources);
        if (!source) continue;
        CGKeyCode code;
        CGEventFlags flags = 0;
        NSString *error = nil;
        if (!resolveKey(cases[identifier][0], source, &code, &flags, &error) || code != [cases[identifier][1] unsignedShortValue]) {
            emit(@{@"ok":@NO, @"error":error ?: @"layout keycode regression", @"source":identifier}); return 1;
        }
        [checked addObject:identifier];
    }
    CGEventFlags flags;
    NSString *error = nil;
    pid_t pid;
    if (!checked.count || parseModifiers(@[@"bogus"], &flags, &error) || parsePID(@"0", &pid) || parsePID(@"123x", &pid)) {
        emit(@{@"ok":@NO, @"error":@"validation self-test failed"}); return 1;
    }
    emit(@{@"ok":@YES, @"event":@"self_test", @"layoutsChecked":checked, @"passive":@YES});
    return 0;
}
