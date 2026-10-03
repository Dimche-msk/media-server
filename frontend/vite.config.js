import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  base: '/',
  build: {
    outDir: '../backend/web',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Чистый basename для стабильных путей
      },
    },
  },
});
