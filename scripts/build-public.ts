#!/usr/bin/env bun

import { cp, copyFile, mkdir, rm, writeFile } from 'node:fs/promises';
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
let latestBrowserShellDir = '';

for (let release of releases) {
  let versionDir = path.join(publicDir, 'metorial-cli', release.tag_name);
  await mkdir(versionDir, { recursive: true });
  let browserVersionDir = path.join(publicDir, 'metorial-cli-browser', release.tag_name);
  let hasBrowserShell = false;

  let mirroredAssets: MirroredAsset[] = [];

  for (let asset of release.assets) {
    let destinationPath = path.join(versionDir, asset.name);
    await downloadFile(asset.browser_download_url, destinationPath);
    mirroredAssets.push({
      name: asset.name,
      size: asset.size,
      download_url: `/metorial-cli/${release.tag_name}/${asset.name}`
    });

    if (asset.name.startsWith('browser-shell-')) {
      hasBrowserShell = true;
      await mkdir(browserVersionDir, { recursive: true });
      let browserFileName = asset.name.replace(/^browser-shell-/, '');
      await copyFile(destinationPath, path.join(browserVersionDir, browserFileName));
      if (browserFileName === 'index.html') {
        await copyFile(destinationPath, path.join(browserVersionDir, 'browser-shell-index.html'));
      }
    }
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

  if (hasBrowserShell && release.tag_name === latestRelease.tag_name) {
    latestBrowserShellDir = browserVersionDir;
  }
}

await mkdir(path.join(publicDir, 'metorial-cli'), { recursive: true });
await writeFile(path.join(publicDir, 'metorial-cli', 'latest'), `${latestRelease.tag_name}\n`);
await writeFile(
  path.join(publicDir, 'metorial-cli', 'releases.json'),
  JSON.stringify(mirroredReleases, null, 2) + '\n'
);
await copyFile(installTemplatePath, path.join(publicDir, 'install.sh'));

if (latestBrowserShellDir) {
  await rm(path.join(publicDir, 'metorial-cli-browser', 'latest'), { recursive: true, force: true });
  await cp(latestBrowserShellDir, path.join(publicDir, 'metorial-cli-browser', 'latest'), { recursive: true });
  await writeFile(path.join(publicDir, 'metorial-cli-browser', 'latest-tag'), `${latestRelease.tag_name}\n`);
}

await writeFile(
  path.join(publicDir, 'index.html'),
  renderIndex(latestRelease, mirroredReleases)
);

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
  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Metorial CLI</title>
    <meta http-equiv="refresh" content="0; url=https://metorial.com/cli" />
  </head>
</html>
`;
}
