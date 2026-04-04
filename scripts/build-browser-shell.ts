#!/usr/bin/env bun

import { copyFile, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

let currentFile = fileURLToPath(import.meta.url);
let scriptsDir = path.dirname(currentFile);
let rootDir = path.resolve(scriptsDir, '..');
let outputDir = path.join(rootDir, 'dist', 'browser-shell');
let browserShellDir = path.join(rootDir, 'browser-shell');
let version = process.env.RELEASE_VERSION || process.env.VERSION || process.argv[2] || 'dev';

await rm(outputDir, { recursive: true, force: true });
await mkdir(outputDir, { recursive: true });

execFileSync(
  'go',
  ['build', '-o', path.join(outputDir, 'browser-shell-metorial.wasm'), './cmd/metorial-wasm'],
  {
    cwd: rootDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      GOOS: 'js',
      GOARCH: 'wasm'
    }
  }
);

let goroot = execFileSync('go', ['env', 'GOROOT'], { cwd: rootDir, encoding: 'utf8' }).trim();
await copyFile(
  path.join(goroot, 'lib', 'wasm', 'wasm_exec.js'),
  path.join(outputDir, 'browser-shell-wasm_exec.js')
);

let htmlTemplate = await readFile(path.join(browserShellDir, 'index.html'), 'utf8');
await writeFile(
  path.join(outputDir, 'browser-shell-index.html'),
  htmlTemplate.replaceAll('__METORIAL_VERSION__', version)
);
await copyFile(
  path.join(browserShellDir, 'style.css'),
  path.join(outputDir, 'browser-shell-style.css')
);

let build = await Bun.build({
  entrypoints: [path.join(browserShellDir, 'src', 'main.ts')],
  outdir: outputDir,
  naming: {
    entry: 'browser-shell-main.js'
  },
  target: 'browser',
  minify: false,
  sourcemap: 'none'
});

if (!build.success) {
  throw new Error('Browser shell bundle failed');
}
