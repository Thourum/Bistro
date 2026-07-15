export interface GoProcessConfig {
  binaryPath: string;
  dbPath: string;
  wsPort: number;
  logLevel: 'debug' | 'info' | 'warn' | 'error';
  logFile: string;
  replayQueue: boolean;
  /** Seconds between automatic sync ticks. Passed as -sync-interval on startup. */
  syncInterval: number;
  /** Whether the automatic ticker is enabled. Passed as -auto-sync on startup. */
  autoSync: boolean;
}

export interface AppConfig {
  windowWidth: number;
  windowHeight: number;
  goProcess: GoProcessConfig;
}
