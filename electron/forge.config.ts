import { MakerDMG } from '@electron-forge/maker-dmg';
import { MakerZIP } from '@electron-forge/maker-zip';
import { AutoUnpackNativesPlugin } from '@electron-forge/plugin-auto-unpack-natives';
import { VitePlugin } from '@electron-forge/plugin-vite';
import { PublisherGithub } from '@electron-forge/publisher-github';
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

const macNotarizeOptions = APPLE_SIGNING_IDENTITY
  ? {
      appleId: process.env.APPLE_ID ?? '',
      appleIdPassword: process.env.APPLE_APP_SPECIFIC_PASSWORD ?? '',
      teamId: process.env.APPLE_TEAM_ID ?? '',
    }
  : undefined;

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
  },
  rebuildConfig: {},
  makers: [
    new MakerZIP({}, ['darwin', 'linux', 'win32']),
    new MakerDMG({ icon: 'assets/icon.icns' }),
  ],
  publishers: [
    new PublisherGithub({
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
