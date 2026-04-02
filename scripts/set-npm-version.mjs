#!/usr/bin/env node

import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

let currentFile = fileURLToPath(import.meta.url);
let scriptsDir = path.dirname(currentFile);
let cliDir = path.resolve(scriptsDir, '..');
let packageJsonPath = path.join(cliDir, 'npm', 'package.json');
let requestedVersion = (process.argv[2] || process.env.RELEASE_VERSION || process.env.VERSION || '').replace(/^v/, '');

if (!requestedVersion) {
	throw new Error('Provide a version argument or set RELEASE_VERSION/VERSION');
}

let packageJson = JSON.parse(await readFile(packageJsonPath, 'utf8'));
packageJson.version = requestedVersion;

await writeFile(packageJsonPath, JSON.stringify(packageJson, null, 2) + '\n');
