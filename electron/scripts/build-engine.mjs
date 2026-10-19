import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// Builds the production Go sync engine that Forge bundles into Resources/go-bin.
// Run via `pnpm run build:engine`, which loads electron/.env.local (if present) into the env.

const electronRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const goRoot = join(electronRoot, '..', 'go');
const outputDir = join(goRoot, 'dist', 'go-bin');
// Builds for the host OS/arch (the engine needs CGO), so run release:win on Windows.
const outputBinary = join(
  outputDir,
  process.platform === 'win32' ? 'bistro-sync.exe' : 'bistro-sync',
);

// Only the OAuth client id is embedded. The client secret must never be compiled into a binary.
const githubClientId = process.env.GITHUB_CLIENT_ID ?? '';
if (githubClientId === '') {
  console.warn(
    'build:engine: GITHUB_CLIENT_ID is empty; the packaged engine will have no OAuth client id.',
  );
}

mkdirSync(outputDir, { recursive: true });
execFileSync(
  'go',
  [
    'build',
    '-trimpath',
    '-ldflags',
    `-s -w -X main.embeddedGitHubClientID=${githubClientId}`,
    '-o',
    outputBinary,
    './cmd/bistro-sync',
  ],
  { cwd: goRoot, stdio: 'inherit' },
);
console.log(`build:engine: wrote ${outputBinary}`);
