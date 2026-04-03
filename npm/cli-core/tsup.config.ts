import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['cjs'],
  dts: true,
  bundle: true,
  clean: true,
  target: 'node20',
  outDir: 'dist'
});
