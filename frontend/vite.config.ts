import { resolve } from 'node:path';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  root: 'src',
  base: './',
  build: { outDir: '../dist', emptyOutDir: true },
  plugins: [react(), tailwindcss()],
  // The diff highlighter's lazy WASM imports require code-splitting workers.
  worker: { format: 'es' },
  server: {
    host: '127.0.0.1',
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(
      process.env.BISTRO_VERSION ?? 'dev',
    ),
    'import.meta.env.VITE_SENTRY_ENABLED': JSON.stringify(
      String(Boolean(process.env.SENTRY_DSN ?? process.env.VITE_SENTRY_DSN)),
    ),
  },
  resolve: {
    alias: {
      '@renderer': resolve(__dirname, 'src'),
      '@shared': resolve(__dirname, 'src/shared'),
    },
    dedupe: ['react', 'react-dom'],
  },
});
