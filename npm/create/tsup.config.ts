import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/bin.ts'],
  format: ['cjs'],
  bundle: true,
  clean: true,
  target: 'node20',
  outDir: 'dist',
  banner: {
    js: '#!/usr/bin/env node'
  },
  outExtension() {
    return {
      js: '.cjs'
    };
  }
});
