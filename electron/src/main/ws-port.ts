const WS_PORT_ARG_PREFIX = '--bistro-ws-port=';

export function wsPortToArg(port: number): string {
  return `${WS_PORT_ARG_PREFIX}${port}`;
}

/**
 * Reads the WebSocket port passed to the preload script via
 * additionalArguments. A dynamic-port engine starts with port 0, which must
 * remain pending rather than silently becoming the legacy port 8080.
 */
export function readWsPortFromArgv(argv: readonly string[]): number | null {
  const arg = argv.find((a) => a.startsWith(WS_PORT_ARG_PREFIX));
  if (arg === undefined) {
    return null;
  }
  const port = Number(arg.slice(WS_PORT_ARG_PREFIX.length));
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    return null;
  }
  return port;
}
