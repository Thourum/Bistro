import { resolve } from 'node:path';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';
import { version } from './package.json';

export default defineConfig({
  root: resolve(__dirname, 'src/renderer'),
  base: './',
  // Forge 8 passes a relative outDir, which Vite resolves against `root`
  // (src/renderer) and the packaged asar then lacks the renderer. Pin it to
  // the path main/window.ts loads.
  build: {
    outDir: resolve(__dirname, '.vite/renderer/main_window'),
    emptyOutDir: true,
  },
  plugins: [react(), tailwindcss()],
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(version),
    'import.meta.env.VITE_SENTRY_ENABLED': JSON.stringify(
      String(Boolean(process.env.SENTRY_DSN ?? process.env.VITE_SENTRY_DSN)),
    ),
  },
  resolve: {
    alias: {
      '@renderer': resolve(__dirname, 'src/renderer'),
      '@shared': resolve(__dirname, 'src/shared'),
    },
    dedupe: ['react', 'react-dom'],
  },
});
