import { MakerDMG } from '@electron-forge/maker-dmg';
import { MakerZIP } from '@electron-forge/maker-zip';
import { AutoUnpackNativesPlugin } from '@electron-forge/plugin-auto-unpack-natives';
import { VitePlugin } from '@electron-forge/plugin-vite';
import { PublisherGithub } from '@electron-forge/publisher-github';
import type { ForgeConfig } from '@electron-forge/shared-types';

const config: ForgeConfig = {
  packagerConfig: {
    appBundleId: 'app.bistro.review',
    executableName: 'Bistro',
    name: 'Bistro',
    asar: true,
  },
  rebuildConfig: {},
  makers: [new MakerZIP({}, ['darwin', 'linux', 'win32']), new MakerDMG({})],
  publishers: [
    new PublisherGithub({
      repository: {
        owner: 'Itterno',
        name: 'Bistro',
      },
      draft: true,
      prerelease: false,
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
