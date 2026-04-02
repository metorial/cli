let { createHash } = require('node:crypto');
let fs = require('node:fs');
let fsp = require('node:fs/promises');
let http = require('node:http');
let https = require('node:https');
let os = require('node:os');
let path = require('node:path');
let { spawn } = require('node:child_process');

let packageJson = require('../package.json');

async function run(args) {
  let version = packageJson.version.replace(/^v/, '');
  let artifact = resolveArtifact(version);
  let installDir = path.join(resolveCacheDir(), version, `${artifact.os}-${artifact.arch}`);
  let executableName = artifact.os === 'windows' ? 'metorial.exe' : 'metorial';
  let executablePath = path.join(installDir, executableName);

  if (!fs.existsSync(executablePath)) {
    await downloadAndInstall({
      version,
      artifact,
      installDir,
      executablePath
    });
  }

  await runBinary(executablePath, args);
}

function resolveArtifact(version) {
  let osName = mapPlatform(process.platform);
  let archName = mapArch(process.arch);
  let format = osName === 'windows' ? 'zip' : 'tar.gz';

  return {
    version,
    os: osName,
    arch: archName,
    format,
    archiveName: `metorial_${version}_${osName}_${archName}.${format}`
  };
}

function mapPlatform(platform) {
  if (platform === 'darwin') {
    return 'darwin';
  }
  if (platform === 'linux') {
    return 'linux';
  }
  if (platform === 'win32') {
    return 'windows';
  }

  throw new Error(`Unsupported platform: ${platform}`);
}

function mapArch(arch) {
  if (arch === 'x64') {
    return 'amd64';
  }
  if (arch === 'arm64') {
    return 'arm64';
  }

  throw new Error(`Unsupported architecture: ${arch}`);
}

function resolveCacheDir() {
  let explicit = process.env.METORIAL_CLI_CACHE_DIR;
  if (explicit) {
    return path.join(explicit, 'metorial/cli');
  }

  if (process.platform === 'win32') {
    let localAppData = process.env.LOCALAPPDATA || path.join(os.homedir(), 'AppData', 'Local');
    return path.join(localAppData, 'metorial/cli');
  }

  let xdg = process.env.XDG_CACHE_HOME;
  if (xdg) {
    return path.join(xdg, 'metorial/cli');
  }

  return path.join(os.homedir(), '.cache', 'metorial/cli');
}

async function downloadAndInstall({ version, artifact, installDir, executablePath }) {
  let baseUrl =
    process.env.METORIAL_CLI_RELEASE_BASE_URL ||
    'https://github.com/metorial/cli/releases/download';
  let tag = `v${version}`;
  let archiveUrl = `${baseUrl}/${tag}/${artifact.archiveName}`;
  let checksumsUrl = `${baseUrl}/${tag}/checksums.txt`;
  let tempRoot = await fsp.mkdtemp(path.join(os.tmpdir(), 'metorial-cli-'));
  let archivePath = path.join(tempRoot, artifact.archiveName);
  let checksumsPath = path.join(tempRoot, 'checksums.txt');
  let extractDir = path.join(tempRoot, 'extract');

  try {
    await downloadFile(archiveUrl, archivePath);
    await downloadFile(checksumsUrl, checksumsPath);
    await verifyChecksum(archivePath, checksumsPath, artifact.archiveName);
    await extractArchive(archivePath, extractDir, artifact.format);
    await fsp.mkdir(installDir, { recursive: true });
    await copyExecutable(extractDir, executablePath);
  } finally {
    await fsp.rm(tempRoot, { recursive: true, force: true });
  }
}

async function downloadFile(url, destinationPath) {
  let client = url.startsWith('https://') ? https : http;

  await new Promise((resolve, reject) => {
    let request = client.get(
      url,
      {
        headers: {
          'user-agent': `@metorial/cli/${packageJson.version}`
        }
      },
      response => {
        if (
          response.statusCode &&
          response.statusCode >= 300 &&
          response.statusCode < 400 &&
          response.headers.location
        ) {
          response.resume();
          downloadFile(response.headers.location, destinationPath).then(resolve, reject);
          return;
        }

        if (response.statusCode !== 200) {
          response.resume();
          reject(new Error(`Failed to download ${url} (${response.statusCode})`));
          return;
        }

        let output = fs.createWriteStream(destinationPath);
        response.pipe(output);
        output.on('finish', () => {
          output.close();
          resolve();
        });
        output.on('error', reject);
      }
    );

    request.on('error', reject);
  });
}

async function verifyChecksum(archivePath, checksumsPath, archiveName) {
  let checksums = await fsp.readFile(checksumsPath, 'utf8');
  let line = checksums
    .split('\n')
    .map(entry => entry.trim())
    .find(entry => entry.endsWith(` ${archiveName}`) || entry.endsWith(`  ${archiveName}`));

  if (!line) {
    throw new Error(`Checksum entry for ${archiveName} not found`);
  }

  let expected = line.split(/\s+/)[0];
  let archiveContents = await fsp.readFile(archivePath);
  let actual = createHash('sha256').update(archiveContents).digest('hex');

  if (actual !== expected) {
    throw new Error(`Checksum verification failed for ${archiveName}`);
  }
}

async function extractArchive(archivePath, extractDir, format) {
  await fsp.mkdir(extractDir, { recursive: true });

  if (format === 'tar.gz') {
    await execFile('tar', ['-xzf', archivePath, '-C', extractDir]);
    return;
  }

  if (format === 'zip') {
    await execFile('powershell.exe', [
      '-NoLogo',
      '-NoProfile',
      '-NonInteractive',
      '-Command',
      `Expand-Archive -LiteralPath '${archivePath.replace(/'/g, "''")}' -DestinationPath '${extractDir.replace(/'/g, "''")}' -Force`
    ]);
    return;
  }

  throw new Error(`Unsupported archive format: ${format}`);
}

async function copyExecutable(extractDir, executablePath) {
  let sourceName = process.platform === 'win32' ? 'metorial.exe' : 'metorial';
  let sourcePath = path.join(extractDir, sourceName);

  if (!fs.existsSync(sourcePath)) {
    throw new Error(
      `Expected executable ${sourceName} was not found in the downloaded archive`
    );
  }

  await fsp.copyFile(sourcePath, executablePath);

  if (process.platform !== 'win32') {
    await fsp.chmod(executablePath, 0o755);
  }
}

async function runBinary(executablePath, args) {
  await new Promise((resolve, reject) => {
    let child = spawn(executablePath, args, {
      stdio: 'inherit',
      env: process.env
    });

    child.on('error', reject);
    child.on('close', code => {
      process.exitCode = code === null ? 1 : code;
      resolve();
    });
  });
}

async function execFile(command, args) {
  await new Promise((resolve, reject) => {
    let child = spawn(command, args, {
      stdio: 'inherit'
    });

    child.on('exit', code => {
      if (code === 0) {
        resolve();
        return;
      }

      reject(new Error(`${command} exited with code ${code}`));
    });

    child.on('error', reject);
  });
}

module.exports = {
  run
};
