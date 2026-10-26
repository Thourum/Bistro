// A static import (not createRequire) is required so Vite bundles wait-on —
// the packaged asar has no node_modules.
declare module 'wait-on' {
  interface WaitOnOptions {
    resources: string[];
    timeout?: number;
    interval?: number;
    tcpTimeout?: number;
    window?: number;
  }
  export default function waitOn(options: WaitOnOptions): Promise<void>;
}
