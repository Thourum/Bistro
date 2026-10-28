# Gate 2.1 headed smoke

**Playwright WebKit is NOT system WKWebView.** This is a compatibility smoke test, not a shell verdict. The deciding renderer run is Gate 2.3’s native host on the supported macOS floor. n=1 and synthetic-only fixtures cannot establish S1. Keep the browser visible; headless rAF timing is meaningless. No memory measurement.
