import extractZip from 'extract-zip';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import {
  access,
  chmod,
  copyFile,
  mkdir,
  readFile,
  readdir,
  rm,
  writeFile
} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import * as tar from 'tar';
import packageJson from '../package.json';

export type OutputTransform = (value: string) => string;

export type RunCLIOptions = {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  version?: string;
  stdoutTransform?: OutputTransform;
  stderrTransform?: OutputTransform;
};

type PlatformSpec = {
  archiveName: string;
  binaryName: string;
};

let DEFAULT_BASE_URL = 'https://cli.metorial.com';

export async function resolveCLIPath(version?: string) {
  let tag = normalizeVersion(
    version || process.env.METORIAL_CLI_VERSION || packageJson.version
  );
  let spec = getPlatformSpec(tag);
  let installDir = path.join(getCacheRoot(), 'versions', tag);
  let binaryPath = path.join(installDir, spec.binaryName);

  if (await exists(binaryPath)) {
    return binaryPath;
  }

  await downloadAndInstall({ tag, spec, installDir, binaryPath });
  return binaryPath;
}

export async function runCLI(args: string[], options: RunCLIOptions = {}) {
  let binaryPath = await resolveCLIPath(options.version);

  if (!options.stdoutTransform && !options.stderrTransform) {
    return await spawnWithInheritedOutput(binaryPath, args, options);
  }

  return await spawnWithBufferedOutput(binaryPath, args, options);
}

export async function runCLIAndExit(args: string[], options: RunCLIOptions = {}) {
  let exitCode = await runCLI(args, options);
  process.exit(exitCode);
}

export function replaceOutput(searchValue: string, replaceValue: string): OutputTransform {
  return value => value.replaceAll(searchValue, replaceValue);
}

export function npmInstallEnvironment(packageName: string): NodeJS.ProcessEnv {
  return {
    METORIAL_INSTALL_METHOD: 'npm',
    METORIAL_INSTALL_PM: detectNodePackageManager(__filename),
    METORIAL_INSTALL_PACKAGE: packageName
  };
}

async function downloadAndInstall(input: {
  tag: string;
  spec: PlatformSpec;
  installDir: string;
  binaryPath: string;
}) {
  let tempDir = await makeTempDir(path.join(getCacheRoot(), 'tmp'));
  let archivePath = path.join(tempDir, input.spec.archiveName);
  let checksumsPath = path.join(tempDir, 'checksums.txt');
  let extractDir = path.join(tempDir, 'extract');
  let baseUrl = `${(process.env.METORIAL_CLI_BASE_URL || DEFAULT_BASE_URL).replace(/\/$/, '')}/metorial-cli/${input.tag}`;

  try {
    await mkdir(extractDir, { recursive: true });

    await downloadFile(`${baseUrl}/${input.spec.archiveName}`, archivePath);
    await downloadFile(`${baseUrl}/checksums.txt`, checksumsPath);
    await verifyChecksum(checksumsPath, input.spec.archiveName, archivePath);
    await extractArchive(archivePath, extractDir);

    let extractedBinary = await findBinary(extractDir, input.spec.binaryName);
    await mkdir(input.installDir, { recursive: true });
    await copyFile(extractedBinary, input.binaryPath);

    if (process.platform !== 'win32') {
      await chmod(input.binaryPath, 0o755);
    }
  } finally {
    await rm(tempDir, { recursive: true, force: true });
  }
}

async function spawnWithInheritedOutput(
  binaryPath: string,
  args: string[],
  options: RunCLIOptions
) {
  return await new Promise<number>((resolve, reject) => {
    let child = spawn(binaryPath, args, {
      cwd: options.cwd,
      env: { ...process.env, ...options.env },
      stdio: 'inherit'
    });

    child.on('error', reject);
    child.on('close', code => resolve(code ?? 1));
  });
}

async function spawnWithBufferedOutput(
  binaryPath: string,
  args: string[],
  options: RunCLIOptions
) {
  return await new Promise<number>((resolve, reject) => {
    let stdout = '';
    let stderr = '';
    let child = spawn(binaryPath, args, {
      cwd: options.cwd,
      env: { ...process.env, ...options.env },
      stdio: ['inherit', 'pipe', 'pipe']
    });

    child.stdout?.setEncoding('utf8');
    child.stderr?.setEncoding('utf8');

    child.stdout?.on('data', chunk => {
      stdout += String(chunk);
    });
    child.stderr?.on('data', chunk => {
      stderr += String(chunk);
    });

    child.on('error', reject);
    child.on('close', code => {
      process.stdout.write(options.stdoutTransform ? options.stdoutTransform(stdout) : stdout);
      process.stderr.write(options.stderrTransform ? options.stderrTransform(stderr) : stderr);
      resolve(code ?? 1);
    });
  });
}

async function downloadFile(url: string, destinationPath: string) {
  let response = await fetch(url, {
    headers: {
      'User-Agent': 'metorial-cli-npm',
      Accept: '*/*'
    }
  });

  if (!response.ok || !response.body) {
    throw new Error(`Failed to download ${url}: ${response.status} ${response.statusText}`);
  }

  await writeFile(destinationPath, Buffer.from(await response.arrayBuffer()));
}

async function verifyChecksum(
  checksumsPath: string,
  archiveName: string,
  archivePath: string
) {
  let checksums = await readFile(checksumsPath, 'utf8');
  let expected = checksums
    .split('\n')
    .map(line => line.trim().split(/\s+/))
    .find(parts => parts.length >= 2 && parts[1] === archiveName)?.[0];

  if (!expected) {
    throw new Error(`Checksum for ${archiveName} not found`);
  }

  let archiveBuffer = await readFile(archivePath);
  let actual = createHash('sha256').update(archiveBuffer).digest('hex');

  if (actual !== expected) {
    throw new Error(`Checksum verification failed for ${archiveName}`);
  }
}

async function extractArchive(archivePath: string, extractDir: string) {
  if (archivePath.endsWith('.zip')) {
    await extractZip(archivePath, { dir: extractDir });
    return;
  }

  await tar.x({
    file: archivePath,
    cwd: extractDir
  });
}

async function findBinary(rootDir: string, binaryName: string): Promise<string> {
  let directPath = path.join(rootDir, binaryName);
  if (await exists(directPath)) {
    return directPath;
  }

  return await walkForBinary(rootDir, binaryName);
}

function getCacheRoot() {
  if (process.env.METORIAL_CLI_CACHE_DIR) {
    return process.env.METORIAL_CLI_CACHE_DIR;
  }

  return path.join(os.homedir(), '.metorial', 'cli-npm');
}

function getPlatformSpec(tag: string): PlatformSpec {
  let goos = resolveGoOS();
  let goarch = resolveGoArch();
  let extension = goos === 'windows' ? 'zip' : 'tar.gz';
  let binaryName = goos === 'windows' ? 'metorial.exe' : 'metorial';

  return {
    archiveName: `metorial_${tag.replace(/^v/, '')}_${goos}_${goarch}.${extension}`,
    binaryName
  };
}

function resolveGoOS() {
  switch (process.platform) {
    case 'darwin':
      return 'darwin';
    case 'linux':
      return 'linux';
    case 'win32':
      return 'windows';
    default:
      throw new Error(`Unsupported platform: ${process.platform}`);
  }
}

function resolveGoArch() {
  switch (process.arch) {
    case 'x64':
      return 'amd64';
    case 'arm64':
      return 'arm64';
    default:
      throw new Error(`Unsupported architecture: ${process.arch}`);
  }
}

function normalizeVersion(value: string) {
  if (!value.trim()) {
    throw new Error('Unable to resolve a CLI version');
  }

  return value.startsWith('v') ? value : `v${value}`;
}

async function makeTempDir(parentDir: string) {
  let suffix = `${process.pid}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  let tempDir = path.join(parentDir, suffix);
  await mkdir(tempDir, { recursive: true });
  return tempDir;
}

async function exists(filePath: string) {
  try {
    await access(filePath);
    return true;
  } catch {
    return false;
  }
}

async function walkForBinary(rootDir: string, binaryName: string): Promise<string> {
  let entries = await readdir(rootDir, { withFileTypes: true });

  for (let entry of entries) {
    let entryPath = path.join(rootDir, entry.name);

    if (entry.isFile() && entry.name === binaryName) {
      return entryPath;
    }

    if (entry.isDirectory()) {
      try {
        return await walkForBinary(entryPath, binaryName);
      } catch {
        continue;
      }
    }
  }

  throw new Error(`Downloaded archive did not contain ${binaryName}`);
}

function detectNodePackageManager(fileName: string) {
  let userAgent = process.env.npm_config_user_agent || '';

  if (userAgent.startsWith('pnpm/')) {
    return 'pnpm';
  }
  if (userAgent.startsWith('yarn/')) {
    return 'yarn';
  }
  if (userAgent.startsWith('bun/')) {
    return 'bun';
  }
  if (userAgent.startsWith('npm/')) {
    return 'npm';
  }

  let normalized = fileName.replaceAll('\\', '/').toLowerCase();
  if (normalized.includes('/.bun/install/global/')) {
    return 'bun';
  }
  if (normalized.includes('/pnpm/global/') || normalized.includes('/pnpm-global/')) {
    return 'pnpm';
  }
  if (normalized.includes('/.config/yarn/global/') || normalized.includes('/.yarn/global/')) {
    return 'yarn';
  }

  return 'npm';
}
