import { defineConfig } from 'vite';

export default defineConfig({
  resolve: {
    conditions: ['node'],
    mainFields: ['module', 'jsnext:main', 'jsnext'],
  },
  build: {
    rollupOptions: {
      // Only Electron itself is provided at runtime. Everything else must be bundled:
      // Forge ships only .vite/ in the asar, with no node_modules to resolve from.
      external: ['electron'],
    },
  },
});
