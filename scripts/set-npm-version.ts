#!/usr/bin/env bun

import { readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

let currentFile = fileURLToPath(import.meta.url);
let scriptsDir = path.dirname(currentFile);
let rootDir = path.resolve(scriptsDir, '..');
let packagesDir = path.join(rootDir, 'npm');
let requestedVersion = (process.argv[2] || process.env.RELEASE_VERSION || process.env.VERSION || '').replace(/^v/, '');

if (!requestedVersion) {
  throw new Error('Provide a version argument or set RELEASE_VERSION/VERSION');
}

let packageDirs = (await readdir(packagesDir, { withFileTypes: true }))
  .filter(entry => entry.isDirectory())
  .map(entry => path.join(packagesDir, entry.name));

let packageNames = new Map<string, string>();

for (let packageDir of packageDirs) {
  let packageJsonPath = path.join(packageDir, 'package.json');
  let packageJson = JSON.parse(await readFile(packageJsonPath, 'utf8'));
  packageNames.set(packageJson.name, packageJsonPath);
}

for (let packageDir of packageDirs) {
  let packageJsonPath = path.join(packageDir, 'package.json');
  let packageJson = JSON.parse(await readFile(packageJsonPath, 'utf8'));

  packageJson.version = requestedVersion;

  for (let field of ['dependencies', 'devDependencies', 'peerDependencies', 'optionalDependencies'] as const) {
    let values = packageJson[field];
    if (!values) {
      continue;
    }

    for (let dependencyName of Object.keys(values)) {
      if (!packageNames.has(dependencyName)) {
        continue;
      }

      values[dependencyName] = requestedVersion;
    }
  }

  await writeFile(packageJsonPath, JSON.stringify(packageJson, null, 2) + '\n');
}
