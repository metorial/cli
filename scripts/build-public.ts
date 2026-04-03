#!/usr/bin/env bun

import { copyFile, mkdir, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

type GitHubAsset = {
  name: string;
  size: number;
  browser_download_url: string;
};

type GitHubRelease = {
  tag_name: string;
  name: string | null;
  draft: boolean;
  prerelease: boolean;
  html_url: string;
  published_at: string | null;
  created_at: string;
  assets: GitHubAsset[];
};

type MirroredAsset = {
  name: string;
  size: number;
  download_url: string;
};

type MirroredRelease = {
  tag: string;
  published_at: string | null;
  html_url: string;
  assets: MirroredAsset[];
};

let currentFile = fileURLToPath(import.meta.url);
let scriptsDir = path.dirname(currentFile);
let cliDir = path.resolve(scriptsDir, '..');
let publicDir = path.join(cliDir, 'public');
let installTemplatePath = path.join(cliDir, 'templates', 'install.sh');

let githubOwner = process.env.GITHUB_REPOSITORY_OWNER || 'metorial';
let githubRepo = process.env.GITHUB_REPOSITORY_NAME || 'cli';
let githubToken = process.env.GITHUB_TOKEN || '';

await rm(publicDir, { recursive: true, force: true });
await mkdir(publicDir, { recursive: true });

let releases = await getAllReleases();

if (releases.length === 0) {
  throw new Error(`No published releases found for ${githubOwner}/${githubRepo}`);
}

let latestRelease = releases[0];
let mirroredReleases: MirroredRelease[] = [];

for (let release of releases) {
  let versionDir = path.join(publicDir, 'metorial-cli', release.tag_name);
  await mkdir(versionDir, { recursive: true });

  let mirroredAssets: MirroredAsset[] = [];

  for (let asset of release.assets) {
    let destinationPath = path.join(versionDir, asset.name);
    await downloadFile(asset.browser_download_url, destinationPath);
    mirroredAssets.push({
      name: asset.name,
      size: asset.size,
      download_url: `/metorial-cli/${release.tag_name}/${asset.name}`
    });
  }

  await writeFile(
    path.join(versionDir, 'manifest.json'),
    JSON.stringify(
      {
        tag: release.tag_name,
        name: release.name || release.tag_name,
        published_at: release.published_at,
        html_url: release.html_url,
        assets: mirroredAssets
      },
      null,
      2
    ) + '\n'
  );

  mirroredReleases.push({
    tag: release.tag_name,
    published_at: release.published_at,
    html_url: release.html_url,
    assets: mirroredAssets
  });
}

await mkdir(path.join(publicDir, 'metorial-cli'), { recursive: true });
await writeFile(path.join(publicDir, 'metorial-cli', 'latest'), `${latestRelease.tag_name}\n`);
await writeFile(path.join(publicDir, 'metorial-cli', 'releases.json'), JSON.stringify(mirroredReleases, null, 2) + '\n');
await copyFile(installTemplatePath, path.join(publicDir, 'install.sh'));
await writeFile(path.join(publicDir, 'index.html'), renderIndex(latestRelease, mirroredReleases));

async function getAllReleases(): Promise<GitHubRelease[]> {
  let releases: GitHubRelease[] = [];
  let page = 1;

  while (true) {
    let url = `https://api.github.com/repos/${githubOwner}/${githubRepo}/releases?per_page=100&page=${page}`;
    let response = await fetchJson<GitHubRelease[]>(url);
    let published = response.filter(release => !release.draft && !release.prerelease);

    releases.push(...published);

    if (response.length < 100) {
      break;
    }

    page += 1;
  }

  releases.sort((left, right) => {
    let leftDate = new Date(left.published_at || left.created_at || 0).getTime();
    let rightDate = new Date(right.published_at || right.created_at || 0).getTime();
    return rightDate - leftDate;
  });

  return releases;
}

async function fetchJson<T>(url: string): Promise<T> {
  let response = await fetch(url, {
    headers: buildHeaders()
  });

  if (!response.ok) {
    throw new Error(`GitHub API request failed (${response.status}) for ${url}`);
  }

  return (await response.json()) as T;
}

async function downloadFile(url: string, destinationPath: string) {
  let response = await fetch(url, {
    headers: buildHeaders()
  });

  if (!response.ok) {
    throw new Error(`Download failed (${response.status}) for ${url}`);
  }

  let arrayBuffer = await response.arrayBuffer();
  await writeFile(destinationPath, Buffer.from(arrayBuffer));
}

function buildHeaders() {
  let headers: Record<string, string> = {
    Accept: 'application/vnd.github+json',
    'User-Agent': 'metorial-cli-release-builder'
  };

  if (githubToken) {
    headers.Authorization = `Bearer ${githubToken}`;
  }

  return headers;
}

function renderIndex(latestRelease: GitHubRelease, releases: MirroredRelease[]) {
  let releaseList = releases
    .map(release => {
      let assetList = release.assets
        .map(asset => `<li><a href="${asset.download_url}">${escapeHtml(asset.name)}</a></li>`)
        .join('');

      return `
        <section>
          <h2>${escapeHtml(release.tag)}</h2>
          <p>Published ${escapeHtml(formatDate(release.published_at))}</p>
          <p><a href="${escapeHtml(release.html_url)}">View GitHub release notes</a></p>
          <ul>${assetList}</ul>
        </section>
      `;
    })
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
        --background: #f3efe7;
        --panel: rgba(255, 251, 244, 0.88);
        --text: #18212b;
        --muted: #5d6672;
        --border: #d8cdbd;
        --accent: #0f5bd8;
      }
      * {
        box-sizing: border-box;
      }
      body {
        margin: 0;
        font-family: "IBM Plex Mono", "SFMono-Regular", "Menlo", monospace;
        color: var(--text);
        background:
          radial-gradient(circle at top left, rgba(15, 91, 216, 0.18), transparent 34%),
          linear-gradient(180deg, #fcfaf6 0%, var(--background) 100%);
      }
      main {
        max-width: 920px;
        margin: 0 auto;
        padding: 48px 20px 72px;
      }
      h1 {
        margin: 0 0 12px;
        font-size: 44px;
        line-height: 1;
      }
      p, li {
        line-height: 1.6;
      }
      .panel, section {
        margin-top: 24px;
        padding: 24px;
        border-radius: 20px;
        border: 1px solid var(--border);
        background: var(--panel);
        backdrop-filter: blur(10px);
        box-shadow: 0 20px 50px rgba(24, 33, 43, 0.08);
      }
      pre {
        margin: 0;
        overflow: auto;
        border-radius: 14px;
        padding: 18px;
        background: #102133;
        color: #f8f4eb;
      }
      a {
        color: var(--accent);
      }
      ul {
        padding-left: 20px;
      }
      .muted {
        color: var(--muted);
      }
    </style>
  </head>
  <body>
    <main>
      <h1>Metorial CLI</h1>
      <p class="muted">Hosted release artifacts and installer for the official Metorial CLI.</p>
      <div class="panel">
        <p>Latest release: <strong>${escapeHtml(latestRelease.tag_name)}</strong></p>
        <pre><code>curl -fsSL https://cli.metorial.com/install.sh | bash</code></pre>
      </div>
      ${releaseList}
    </main>
  </body>
</html>
`;
}

function formatDate(value: string | null) {
  if (!value) {
    return 'unknown date';
  }

  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric'
  }).format(new Date(value));
}

function escapeHtml(value: string) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}
