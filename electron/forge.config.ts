import { copyFile } from 'node:fs/promises';
import { join } from 'node:path';
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
// them into Contents/Resources before signing seals the bundle.
const ELECTRON_LICENSE_FILES = ['LICENSE', 'LICENSES.chromium.html'];
const copyElectronLicenses = async ({
  buildPath,
  platform,
}: {
  buildPath: string;
  platform: string;
}) => {
  if (platform !== 'darwin') return;
  const resourcesDir = join(buildPath, 'Electron.app', 'Contents', 'Resources');
  await Promise.all(
    ELECTRON_LICENSE_FILES.map((name) =>
      copyFile(join(buildPath, name), join(resourcesDir, name)),
    ),
  );
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
    afterExtract: [copyElectronLicenses],
  },
  rebuildConfig: {},
  makers: [
    new MakerZIP({}, ['darwin', 'linux', 'win32']),
    new MakerDMG({ icon: 'assets/icon.icns' }),
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
