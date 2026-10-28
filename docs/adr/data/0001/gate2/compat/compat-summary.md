# Gate 2.2 JavaScript API compatibility

Target: **Safari 16.1**, shipped with macOS Ventura 13.0. Source: [WebKit, 24 October 2022](https://webkit.org/blog/13399/webkit-features-in-safari-16-1/). Exact tool pins and resolved dependency versions are in package.json and pnpm-lock.yaml.

239 renderer TS/TSX files checked (including tests); 0 findings. ES API checks enabled (plugin labels these experimental). No polyfills claimed.

| Feature / diagnostic | Count |
| --- | ---: |
| None | 0 |

Raw locations: compat-results.txt and compat-results.json. This is syntax-based source analysis, not a verdict: instance methods, aliases, dependency code, runtime feature guards and Electron-only bridge calls need manual review. It does not check CSS. See ../tools.md for CSS probes and the minimum-OS limitation.
