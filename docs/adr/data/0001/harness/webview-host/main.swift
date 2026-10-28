import AppKit
import WebKit

func emit(_ payload: [String: Any]) {
    guard let bytes = try? JSONSerialization.data(withJSONObject: payload, options: [.sortedKeys]) else {
        FileHandle.standardError.write(Data("host JSON serialization failed\n".utf8)); exit(2)
    }
    FileHandle.standardOutput.write(bytes + Data("\n".utf8))
}

// This is an evidence host, not a proposed application shell. Private PID SPIs attribute launchd XPC memory.
@MainActor
final class Host: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKScriptMessageHandler {
    var window: NSWindow?
    var webView: WKWebView?
    let url: URL
    let settings: String
    var hasStartedNavigation = false

    init(url: URL, settings: String) { self.url = url; self.settings = settings }

    func applicationDidFinishLaunching(_ notification: Notification) {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        configuration.userContentController.add(self, name: "benchConsole")
        let source = """
        localStorage.setItem('bistro-settings-v1', JSON.stringify(\(settings)));
        (() => {
          const send = (level, message) => window.webkit.messageHandlers.benchConsole.postMessage({level, message});
          const format = value => typeof value === 'string' ? value : String(value);
          for (const level of ['log', 'info', 'warn', 'error', 'debug']) {
            const original = console[level].bind(console);
            console[level] = (...args) => { send(level, args.map(format).join(' ')); original(...args); };
          }
          addEventListener('error', event => send('pageerror', `${event.message} ${event.filename}:${event.lineno}:${event.colno}`));
          addEventListener('unhandledrejection', event => send('pageerror', format(event.reason)));
          addEventListener('visibilitychange', () => send('visibility', document.visibilityState));
          addEventListener('blur', () => send('focus', 'blur'));
          addEventListener('focus', () => send('focus', 'focus'));
        })();
        """
        configuration.userContentController.addUserScript(WKUserScript(source: source, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        let view = WKWebView(frame: NSRect(x: 0, y: 0, width: 1600, height: 900), configuration: configuration)
        if #available(macOS 13.3, *) { view.isInspectable = true }
        view.navigationDelegate = self
        let window = NSWindow(contentRect: view.frame, styleMask: [.titled, .closable, .resizable], backing: .buffered, defer: false)
        window.title = "Bistro ADR 0001 — WKWebView"
        window.isReleasedWhenClosed = false
        window.contentView = view
        window.center()
        self.window = window
        webView = view
        window.delegate = self
        emit(["event": "host_ready", "pid": ProcessInfo.processInfo.processIdentifier,
              "webKitVersion": Bundle(for: WKWebView.self).object(forInfoDictionaryKey: "CFBundleVersion") ?? "unknown"])
        NSApp.activate(ignoringOtherApps: true)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        reportWindowState()
    }

    func applicationDidBecomeActive(_ notification: Notification) {
        window?.makeKeyAndOrderFront(nil)
        reportWindowState()
    }

    func windowDidBecomeKey(_ notification: Notification) { reportWindowState() }
    func windowDidResignKey(_ notification: Notification) { reportWindowState() }
    func windowDidChangeOcclusionState(_ notification: Notification) { reportWindowState() }
    func windowWillClose(_ notification: Notification) {
        emit(["event": "host_error", "message": "Window closed during measurement"])
    }

    func reportWindowState() {
        guard let window = window else { return }
        let isKey = window.isKeyWindow
        let isVisible = window.isVisible && window.occlusionState.contains(.visible)
        emit(["event": "window_state", "isKey": isKey, "isVisible": isVisible])
        // App activation/occlusion notifications are asynchronous: never load while backgrounded.
        guard !hasStartedNavigation, isKey, isVisible else { return }
        hasStartedNavigation = true
        webView?.load(URLRequest(url: url))
    }

    func webView(_ view: WKWebView, didFinish navigation: WKNavigation!) {
        let targets: [(NSObject, String, String)] = [
            (view, "_webProcessIdentifier", "webcontent_pid"),
            (view, "_gpuProcessIdentifier", "gpu_pid"),
            (view.configuration.websiteDataStore, "_networkProcessIdentifier", "networking_pid")
        ]
        for (object, selector, event) in targets {
            guard object.responds(to: NSSelectorFromString(selector)), let pid = object.value(forKey: selector) as? NSNumber, pid.intValue > 0 else {
                emit(["event": "pid_unavailable", "selector": selector]); continue
            }
            emit(["event": event, "pid": pid.intValue])
        }
        reportWindowState()
    }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        emit(["event": "console", "payload": message.body])
    }
    func webView(_ view: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { fail(error) }
    func webView(_ view: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { fail(error) }
    func webViewWebContentProcessDidTerminate(_ view: WKWebView) {
        emit(["event": "host_error", "message": "WebContent terminated"]); exit(3)
    }
    func fail(_ error: Error) { emit(["event": "host_error", "message": error.localizedDescription]); exit(3) }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}

guard CommandLine.arguments.count == 3, let url = URL(string: CommandLine.arguments[1]),
      url.scheme == "http", url.host == "localhost",
      let settingsBytes = FileManager.default.contents(atPath: CommandLine.arguments[2]),
      let settings = String(data: settingsBytes, encoding: .utf8),
      let parsed = try? JSONSerialization.jsonObject(with: settingsBytes), parsed is [String: Any] else {
    FileHandle.standardError.write(Data("Usage: webview-host <http://localhost:PORT/?bench_collector=...> <settings.json>\n".utf8)); exit(2)
}
MainActor.assumeIsolated {
    let host = Host(url: url, settings: settings)
    let app = NSApplication.shared
    app.setActivationPolicy(.regular)
    app.delegate = host
    app.run()
}
