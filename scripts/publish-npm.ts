#!/usr/bin/env bun

import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

let currentFile = fileURLToPath(import.meta.url);
let scriptsDir = path.dirname(currentFile);
let rootDir = path.resolve(scriptsDir, '..');

let publishOrder = ['npm/cli-core', 'npm/cli', 'npm/create', 'npm/call-tool'];

for (let packagePath of publishOrder) {
  execFileSync(
    'npm',
    ['publish', '--access', 'public'],
    {
      cwd: path.join(rootDir, packagePath),
      stdio: 'inherit',
      env: process.env
    }
  );
}
