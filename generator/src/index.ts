import path from 'node:path';
import { $ } from 'bun';
import { emitCatalogGo } from './emit/catalog.go';
import { getEndpoints, getEndpointVersions } from './fetch';
import { joinManifest } from './join';
import { loadManifest } from './manifest';

let repoRoot = path.resolve(import.meta.dir, '../..');
let defaultUrls = [
  process.env.API_URL,
  'http://metorial-root.localhost:4310',
  'http://localhost:4310'
].filter(Boolean) as string[];

let args = process.argv.slice(2);
let urlArg = args.find(arg => !arg.startsWith('-'));
let urls = urlArg ? urlArg.split(',') : defaultUrls;

let workingUrl = await resolveUrl(urls);
if (!workingUrl) {
  throw new Error(`None of the API URLs are reachable: ${urls.join(', ')}`);
}

let manifestPath = path.join(repoRoot, 'manifest/admin.yaml');
let manifest = await loadManifest(manifestPath);
let versions = await getEndpointVersions(workingUrl);
let version =
  versions.versions.find(item => item.version === manifest.apiVersion) ||
  versions.versions.find(item => item.version.includes('magnetar'));

if (!version) {
  throw new Error(`API at ${workingUrl} does not expose ${manifest.apiVersion}`);
}

let payload = await getEndpoints(workingUrl, version.version);
let catalog = joinManifest({
  manifest,
  endpoints: payload.endpoints,
  controllers: payload.controllers,
  types: payload.types
});

let outputPath = path.join(repoRoot, 'internal/generated/admin/catalog.go');
await Bun.write(outputPath, emitCatalogGo(catalog));
await $`gofmt -w ${outputPath}`;

console.log(`Wrote ${outputPath} from ${workingUrl} (${version.version})`);

async function resolveUrl(candidates: string[]) {
  for (let url of candidates) {
    try {
      let response = await fetch(new URL('/metorial/introspect/versions', url));
      if (response.ok) return url;
    } catch {}
  }
  return null;
}
