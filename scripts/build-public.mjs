#!/usr/bin/env node

import { copyFile, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

let currentFile = fileURLToPath(import.meta.url);
let scriptsDir = path.dirname(currentFile);
let cliDir = path.resolve(scriptsDir, '..');
let distDir = path.join(cliDir, 'dist');
let publicDir = path.join(cliDir, 'public');
let installTemplatePath = path.join(cliDir, 'templates', 'install.sh');

let rawVersion = process.env.RELEASE_VERSION || process.env.VERSION || '';
let normalizedVersion = rawVersion.replace(/^v/, '');

if (!normalizedVersion) {
	throw new Error('RELEASE_VERSION or VERSION must be set');
}

let tag = `v${normalizedVersion}`;
let releaseDir = path.join(publicDir, 'releases', 'download', tag);
let latestManifestPath = path.join(publicDir, 'releases', 'latest.json');
let versionManifestPath = path.join(publicDir, 'releases', tag, 'manifest.json');
let checksumsPath = path.join(distDir, 'checksums.txt');

let checksumText = await readFile(checksumsPath, 'utf8');
let checksums = parseChecksums(checksumText);

await rm(publicDir, { recursive: true, force: true });
await mkdir(releaseDir, { recursive: true });
await mkdir(path.dirname(latestManifestPath), { recursive: true });
await mkdir(path.dirname(versionManifestPath), { recursive: true });

let distEntries = await readdir(distDir, { withFileTypes: true });
let assets = [];

for (let entry of distEntries) {
	if (!entry.isFile()) {
		continue;
	}

	let sourcePath = path.join(distDir, entry.name);
	let destinationPath = path.join(releaseDir, entry.name);

	await copyFile(sourcePath, destinationPath);

	let archiveMetadata = parseArchiveName(entry.name, normalizedVersion);
	if (!archiveMetadata) {
		continue;
	}

	assets.push({
		...archiveMetadata,
		download_url: `/releases/download/${tag}/${entry.name}`,
		checksum: checksums.get(entry.name) || null
	});
}

assets.sort((left, right) => left.name.localeCompare(right.name));

let manifest = {
	version: normalizedVersion,
	tag,
	install_script: '/install.sh',
	checksums_url: `/releases/download/${tag}/checksums.txt`,
	github_release_url: `https://github.com/metorial/metorial-enterprise/releases/tag/${tag}`,
	assets
};

await copyFile(installTemplatePath, path.join(publicDir, 'install.sh'));
await writeFile(latestManifestPath, JSON.stringify(manifest, null, 2) + '\n');
await writeFile(versionManifestPath, JSON.stringify(manifest, null, 2) + '\n');
await writeFile(path.join(publicDir, 'index.html'), renderIndex(manifest));

function parseChecksums(contents) {
	let values = new Map();

	for (let line of contents.split('\n')) {
		let trimmed = line.trim();
		if (!trimmed) {
			continue;
		}

		let parts = trimmed.split(/\s+/);
		if (parts.length < 2) {
			continue;
		}

		values.set(parts[1], parts[0]);
	}

	return values;
}

function parseArchiveName(fileName, version) {
	let match = fileName.match(new RegExp(`^metorial_${escapeRegExp(version)}_(darwin|linux|windows)_(amd64|arm64)\\.(tar\\.gz|zip)$`));
	if (!match) {
		return null;
	}

	return {
		name: fileName,
		os: match[1],
		arch: match[2],
		format: match[3]
	};
}

function escapeRegExp(value) {
	return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function renderIndex(manifest) {
	let assetList = manifest.assets
		.map(asset => `<li><a href="${asset.download_url}">${asset.name}</a> <span>${asset.os}/${asset.arch}</span></li>`)
		.join('\n');

	return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Metorial CLI</title>
    <style>
      :root {
        color-scheme: light;
        --background: #f6f4ef;
        --card: #fffdf8;
        --foreground: #17222f;
        --muted: #5d6b7a;
        --accent: #1d5eff;
        --border: #d8d0c4;
      }
      * {
        box-sizing: border-box;
      }
      body {
        margin: 0;
        font-family: "SF Mono", "IBM Plex Mono", "Consolas", monospace;
        background:
          radial-gradient(circle at top left, rgba(29, 94, 255, 0.12), transparent 35%),
          linear-gradient(180deg, #fbfaf6 0%, var(--background) 100%);
        color: var(--foreground);
      }
      main {
        max-width: 860px;
        margin: 0 auto;
        padding: 48px 20px 80px;
      }
      h1 {
        font-size: 40px;
        line-height: 1;
        margin: 0 0 12px;
      }
      p {
        color: var(--muted);
        line-height: 1.6;
      }
      section {
        background: var(--card);
        border: 1px solid var(--border);
        border-radius: 18px;
        padding: 24px;
        margin-top: 24px;
        box-shadow: 0 18px 50px rgba(30, 42, 56, 0.08);
      }
      code {
        background: rgba(23, 34, 47, 0.06);
        border-radius: 10px;
        padding: 2px 8px;
      }
      pre {
        margin: 0;
        overflow: auto;
        background: #132033;
        color: #f7f3ea;
        padding: 18px;
        border-radius: 14px;
      }
      a {
        color: var(--accent);
      }
      ul {
        margin: 0;
        padding-left: 20px;
      }
      li + li {
        margin-top: 10px;
      }
      span {
        color: var(--muted);
        margin-left: 8px;
      }
    </style>
  </head>
  <body>
    <main>
      <h1>Metorial CLI</h1>
      <p>Current release: <code>${manifest.tag}</code></p>
      <section>
        <p>Install on macOS or Linux:</p>
        <pre><code>curl -fsSL https://cli.metorial.com/install.sh | bash</code></pre>
      </section>
      <section>
        <p>Install from npm:</p>
        <pre><code>npm install -g @metorial/cli</code></pre>
      </section>
      <section>
        <p>Release assets:</p>
        <ul>
          ${assetList}
        </ul>
      </section>
    </main>
  </body>
</html>
`;
}
