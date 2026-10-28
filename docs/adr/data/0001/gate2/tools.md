# Gate 2 tools and compatibility scope

## Exact toolchain

- Node v24.13.0; global Playwright **1.63.0**, resolved by `npm root -g`, then absolute `playwright/index.mjs` import. Its nested playwright-core is 1.63.0.
- Actual cached builds selected by that installation: **Chromium 153.0.8010.12, revision 1243**, and **WebKit 26.6, revision 2359**. The initially advertised chromium-1248/webkit-2370 caches are not the builds this package selects. WebKit smoke initially failed because 2359 was absent; the coordinator installed the expected cached browsers before the successful run. These drivers never download browsers.
- `@electron/asar` **4.3.1** from repository-root node_modules extracts the packaged slim app; record its installed version when reproducing (see harness README). No development server or renderer rebuild is substituted.
- Compatibility tools are isolated in `compat/`: exact eslint 9.39.1, eslint-plugin-compat 6.0.2, typescript-eslint 8.46.2, with their resolved transitive versions locked by pnpm. pnpm 11.5.2; install with `--ignore-workspace`. These pins predate the seven-day release-age window. eslint 9.39.1 is now deprecated; retained as a reproducible analysis tool, not added to the app.

## macOS floor and limits

macOS **Ventura 13.0 shipped Safari 16.1**. Primary source: [WebKit Features in Safari 16.1, 24 October 2022](https://webkit.org/blog/13399/webkit-features-in-safari-16-1/), which lists Ventura and instructs users to update macOS to Ventura for Safari 16.1. `compat/package.json` pins Browserslist to `Safari 16.1`, not “latest Safari”.

**Playwright WebKit is NOT system WKWebView.** It is a patched test engine, not the operating system framework. Neither its successful smoke nor static API lint establishes compatibility with Ventura. Gate 2.3's native WKWebView host is the deciding renderer measurement; a Ventura native-host run is still needed for minimum-OS evidence. The current host is macOS 27.2.

## CSS feature detection

`compat/package.json` script `css` runs `css-probe.mjs [extracted-renderer-root]`. It inventories the exact built CSS and runs `CSS.supports` / `CSS.registerProperty` availability probes in cached Chromium and WebKit; headless is appropriate for these functional probes, never for frame timing. `css-results.json` records engine versions and feature counts. Both current engines support all five probes:

| Built CSS feature | Token occurrences | Safari 16.1 caveat |
| --- | ---: | --- |
| color-mix(in oklab, …) | 103 | Safari 16.2 feature; floor mismatch |
| oklch | 16 | Probe on floor still required |
| backdrop-filter | 6 | Prefix/fallback behavior needs floor verification |
| individual translate | 2 | Probe on floor still required |
| @property / registerProperty | 63 | Safari 16.4 feature; floor mismatch |

The bundle also contains five `@layer` tokens. Token counts are not a CSS parser or proof of usage. Generated Tailwind utility fallbacks and actual computed styles need inspection on the floor; `CSS.supports` does not establish cascade correctness. No polyfill or application CSS changes were made. Safari feature sources: [Safari 16.4 release notes](https://webkit.org/blog/13966/webkit-features-in-safari-16-4/), which confirm color-mix shipped in 16.2 and @property in 16.4. These are compatibility risks, not a fabricated pass/fail screenshot result.

Static JS results: `compat/compat-summary.md`. The checker self-tests against unsupported `requestIdleCallback`, then checks all renderer TS/TSX sources. It does not analyze dependency internals or all instance method chains. The absent `document.startViewTransition` diagnostic in a trial sentinel illustrates this coverage limit; zero reported findings is not zero unsupported APIs.
