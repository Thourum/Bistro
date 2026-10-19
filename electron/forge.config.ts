import { createReadStream, createWriteStream } from 'node:fs';
import { copyFile, readdir, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { pipeline } from 'node:stream/promises';
import { createGzip } from 'node:zlib';
import { MakerDMG } from '@electron-forge/maker-dmg';
import { MakerZIP } from '@electron-forge/maker-zip';
import { AutoUnpackNativesPlugin } from '@electron-forge/plugin-auto-unpack-natives';
import { VitePlugin } from '@electron-forge/plugin-vite';
import { PublisherGitHub } from '@electron-forge/publisher-github';
import type { ForgeConfig } from '@electron-forge/shared-types';

// Signing is opt-in: unsigned local builds stay possible when no identity is provided.
const APPLE_SIGNING_IDENTITY = process.env.APPLE_SIGNING_IDENTITY;
const MAC_ENTITLEMENTS_PATH = 'build/entitlements.mac.plist';
// Electron's own sub-helpers keep @electron/osx-sign's defaults, which match what Chromium ships.
const ELECTRON_SUB_HELPER_PATTERN = /\((Plugin|GPU|Renderer)\)\.app/;

const macSignOptions = APPLE_SIGNING_IDENTITY
  ? {
      identity: APPLE_SIGNING_IDENTITY,
      hardenedRuntime: true,
      optionsForFile: (filePath: string) =>
        ELECTRON_SUB_HELPER_PATTERN.test(filePath)
          ? {}
          : { entitlements: MAC_ENTITLEMENTS_PATH },
    }
  : undefined;

const { APPLE_ID, APPLE_APP_SPECIFIC_PASSWORD, APPLE_TEAM_ID } = process.env;
const macNotarizeOptions =
  APPLE_SIGNING_IDENTITY &&
  APPLE_ID &&
  APPLE_APP_SPECIFIC_PASSWORD &&
  APPLE_TEAM_ID
    ? {
        appleId: APPLE_ID,
        appleIdPassword: APPLE_APP_SPECIFIC_PASSWORD,
        teamId: APPLE_TEAM_ID,
      }
    : undefined;

// Electron's MIT LICENSE and the Chromium/Node notices sit beside Electron.app
// in the extracted prebuilt, and the DMG/ZIP makers ship only the .app. Copy
// them into Contents/Resources before signing seals the bundle. The 20 MB
// Chromium notices ship gzipped (2 MB): still included, as their BSD-style
// licenses require, without the installed-size cost.
const ELECTRON_LICENSE = 'LICENSE';
const CHROMIUM_NOTICES = 'LICENSES.chromium.html';
const copyElectronLicenses = async (
  resourcesDir: string,
  buildPath: string,
) => {
  await copyFile(
    join(buildPath, ELECTRON_LICENSE),
    join(resourcesDir, ELECTRON_LICENSE),
  );
  await pipeline(
    createReadStream(join(buildPath, CHROMIUM_NOTICES)),
    createGzip(),
    createWriteStream(join(resourcesDir, `${CHROMIUM_NOTICES}.gz`)),
  );
};

// Bistro's UI is English-only, so Chromium's 54 other locale packs (49 MB
// installed, 12 MB in the DMG) are dead weight. Removing the app-level .lproj
// stubs too keeps macOS from advertising those languages for the app.
const KEPT_LOCALES = new Set(['en.lproj', 'en_GB.lproj']);
const removeUnusedLocales = async (resourcesDir: string) => {
  const entries = await readdir(resourcesDir);
  await Promise.all(
    entries
      .filter((name) => name.endsWith('.lproj') && !KEPT_LOCALES.has(name))
      .map((name) => rm(join(resourcesDir, name), { recursive: true })),
  );
};

const prepareMacBundle = async ({
  buildPath,
  platform,
}: {
  buildPath: string;
  platform: string;
}) => {
  if (platform !== 'darwin') return;
  const contentsDir = join(buildPath, 'Electron.app', 'Contents');
  const appResourcesDir = join(contentsDir, 'Resources');
  const frameworkResourcesDir = join(
    contentsDir,
    'Frameworks',
    'Electron Framework.framework',
    'Versions',
    'A',
    'Resources',
  );
  await Promise.all([
    copyElectronLicenses(appResourcesDir, buildPath),
    removeUnusedLocales(appResourcesDir),
    removeUnusedLocales(frameworkResourcesDir),
  ]);
};

const config: ForgeConfig = {
  packagerConfig: {
    appBundleId: 'app.bistro.review',
    executableName: 'Bistro',
    name: 'Bistro',
    icon: ['assets/icon.icns', 'assets/icon.icon'],
    asar: true,
    // Forge copies this directory into Contents/Resources/go-bin (see main/index.ts resolveGoBinaryPath).
    extraResource: ['../go/dist/go-bin', 'assets/icon.png'],
    osxSign: macSignOptions,
    osxNotarize: macNotarizeOptions,
    afterExtract: [prepareMacBundle],
  },
  rebuildConfig: {},
  makers: [
    new MakerZIP({}, ['darwin', 'linux', 'win32']),
    // LZMA (ULMO, macOS 10.15+) instead of the zlib default: about 33 MB smaller.
    new MakerDMG({ icon: 'assets/icon.icns', format: 'ULMO' }),
  ],
  publishers: [
    new PublisherGitHub({
      repository: {
        owner: 'Itterno',
        name: 'Bistro',
      },
      draft: true,
      prerelease: true,
      generateReleaseNotes: true,
    }),
  ],
  plugins: [
    new AutoUnpackNativesPlugin({}),
    new VitePlugin({
      build: [
        {
          entry: 'src/main/index.ts',
          config: 'vite.main.config.ts',
          target: 'main',
        },
        {
          entry: 'src/main/preload.ts',
          config: 'vite.preload.config.ts',
          target: 'preload',
        },
      ],
      renderer: [
        {
          name: 'main_window',
          config: 'vite.renderer.config.ts',
        },
      ],
    }),
  ],
};

export default config;
