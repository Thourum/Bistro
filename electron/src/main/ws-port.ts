const WS_PORT_ARG_PREFIX = '--bistro-ws-port=';
export const DEFAULT_WS_PORT = 8080;

export function wsPortToArg(port: number): string {
  return `${WS_PORT_ARG_PREFIX}${port}`;
}

/**
 * Reads the WebSocket port passed to the preload script via
 * additionalArguments. Falls back to DEFAULT_WS_PORT when absent or invalid.
 */
export function readWsPortFromArgv(argv: readonly string[]): number {
  const arg = argv.find((a) => a.startsWith(WS_PORT_ARG_PREFIX));
  if (arg === undefined) {
    return DEFAULT_WS_PORT;
  }
  const port = Number(arg.slice(WS_PORT_ARG_PREFIX.length));
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    return DEFAULT_WS_PORT;
  }
  return port;
}
