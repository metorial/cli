import path from 'node:path';
import { parseArgs } from 'node:util';
import { $ } from 'bun';
import { emitCatalogGo } from './emit/catalog.go';
import { emitSlateContract } from './emit/slate';
import { getEndpoints, getEndpointVersions, type IntrospectionSnapshot } from './fetch';
import { joinManifest, type MappedResource } from './join';
import { loadManifest } from './manifest';

let repoRoot = path.resolve(import.meta.dir, '../..');
let defaultUrls = [
  process.env.API_URL,
  'http://metorial-root.localhost:4310',
  'http://localhost:4310'
].filter(Boolean) as string[];

let { positionals, values } = parseArgs({
  args: process.argv.slice(2),
  allowPositionals: true,
  options: {
    target: { type: 'string', default: 'go' },
    out: { type: 'string' },
    snapshot: { type: 'string' }
  }
});

let emitters: Record<string, (catalog: MappedResource[], apiVersion: string) => Promise<string>> = {
  go: async catalog => {
    let outputPath = path.join(repoRoot, 'internal/generated/admin/catalog.go');
    await Bun.write(outputPath, emitCatalogGo(catalog));
    await $`gofmt -w ${outputPath}`;
    return outputPath;
  },
  slate: async (catalog, apiVersion) => {
    let outputPath = path.resolve(values.out!);
    await Bun.write(outputPath, emitSlateContract({ apiVersion, resources: catalog }));
    return outputPath;
  }
};

let emit = emitters[values.target!];
if (!emit) {
  throw new Error(`Unknown --target ${values.target}, expected ${Object.keys(emitters).join(' or ')}`);
}
if (values.target === 'slate' && !values.out) {
  throw new Error('--out <path> is required for --target slate');
}

let urlArg = positionals[0];
let urls = urlArg ? urlArg.split(',') : defaultUrls;

// An explicit URL (argument or API_URL) wins over --snapshot so a running instance can be used.
let snapshotPath = values.snapshot;
let snapshot =
  snapshotPath && !urlArg && !process.env.API_URL
    ? ((await Bun.file(path.resolve(snapshotPath)).json()) as IntrospectionSnapshot)
    : undefined;

let workingUrl = snapshot ? 'snapshot' : await resolveUrl(urls);
if (!workingUrl) {
  throw new Error(`None of the API URLs are reachable: ${urls.join(', ')}`);
}

let manifestPath = path.join(repoRoot, 'manifest/admin.yaml');
let manifest = await loadManifest(manifestPath);
let versions = await getEndpointVersions(workingUrl, snapshot);
let version =
  versions.versions.find(item => item.version === manifest.apiVersion) ||
  versions.versions.find(item => item.version.includes('magnetar'));

if (!version) {
  throw new Error(`API at ${workingUrl} does not expose ${manifest.apiVersion}`);
}

let payload = await getEndpoints(workingUrl, version.version, snapshot);
let catalog = joinManifest({
  manifest,
  endpoints: payload.endpoints,
  controllers: payload.controllers,
  types: payload.types
});

let outputPath = await emit(catalog, version.version);

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
