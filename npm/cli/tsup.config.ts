import { defineConfig } from 'tsup';

export default defineConfig([
  {
    entry: ['src/index.ts'],
    format: ['cjs'],
    dts: true,
    bundle: true,
    clean: true,
    target: 'node20',
    outDir: 'dist'
  },
  {
    entry: ['src/bin.ts'],
    format: ['cjs'],
    bundle: true,
    clean: false,
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
  }
]);
