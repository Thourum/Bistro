import onigWasmUrl from 'shiki/onig.wasm?url';

/**
 * Stands in for `shiki/wasm` (aliased in vite.config.ts). `@pierre/diffs`
 * imports `shiki/wasm` from both the main bundle and the highlighter worker,
 * and Vite builds workers as a separate graph, so the base64-inlined module
 * was emitted twice (~622 KB each). Fetching the raw `onig.wasm` asset lets
 * both bundles share one hashed file. ArrayBuffer instantiation, rather than
 * `instantiateStreaming`, does not depend on the asset server's MIME type.
 */
export default function getWasmInstance(
  info: WebAssembly.Imports,
): Promise<WebAssembly.Exports> {
  return fetch(onigWasmUrl)
    .then((response) => response.arrayBuffer())
    .then((binary) => WebAssembly.instantiate(binary, info))
    .then((wasm) => wasm.instance.exports);
}
