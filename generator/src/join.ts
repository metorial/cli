import type { ArgOverride, CommandEntry, Manifest } from './manifest';
import type { Controller, Endpoint, IntrospectedType } from './fetch';
import {
  camelToKebab,
  normalizeSdkPath,
  pathPlaceholders,
  splitResourceMethod,
  stripSdkPath
} from './case';

export type FlagKind = 'string' | 'bool' | 'int' | 'float' | 'string-slice' | 'json' | 'json-file';

export type MappedArg = {
  name: string;
  target: string;
  required: boolean;
  description: string;
};

export type MappedFlag = {
  name: string;
  type: FlagKind;
  target: string;
  usage: string;
  required: boolean;
  repeated: boolean;
};

export type MappedOperation = {
  name: string;
  method: string;
  path: string;
  short: string;
  long: string;
  args: MappedArg[];
  flags: MappedFlag[];
  sdkMapping: string;
};

export type MappedResource = {
  path: string[];
  plural: string;
  singular: string;
  short: string;
  long: string;
  aliases: string[];
  operations: MappedOperation[];
  shortcuts: { path: string[]; method: string }[];
  children: MappedResource[];
};

type PublicEndpoint = Endpoint & {
  sdkPath: string;
  httpPath: string;
};

let paginationFields = new Set(['limit', 'after', 'before', 'cursor', 'order']);

export let joinManifest = (input: {
  manifest: Manifest;
  endpoints: Endpoint[];
  controllers: Controller[];
  types: { id: string; type: IntrospectedType }[];
}): MappedResource[] => {
  let publicEndpoints = collectPublicEndpoints(input.endpoints, input.manifest);
  let types = new Map(input.types.map(type => [type.id, type.type]));
  let controllers = new Map(input.controllers.map(controller => [controller.id, controller]));

  let mapped: { entry: CommandEntry; resource: MappedResource }[] = [];

  for (let entry of input.manifest.commands) {
    if (entry.group) {
      mapped.push({
        entry,
        resource: {
          path: entry.path,
          plural: entry.path[entry.path.length - 1],
          singular: entry.path[entry.path.length - 1],
          short: entry.short || entry.path.join(' '),
          long: entry.long || '',
          aliases: entry.aliases || [],
          operations: [],
          shortcuts: [],
          children: []
        }
      });
      continue;
    }

    if (!entry.resource) {
      throw new Error(`manifest command ${entry.path.join(' ')} is missing resource`);
    }

    let wanted = normalizeSdkPath(entry.resource);
    let matches = publicEndpoints.filter(endpoint => {
      let { resource, method } = splitResourceMethod(normalizeSdkPath(endpoint.sdkPath));
      if (resource !== wanted) return false;
      if (!entry.include?.length) return true;
      return entry.include.some(name => camelToKebab(name) === camelToKebab(method));
    });

    if (matches.length === 0) {
      throw new Error(
        `manifest command ${entry.path.join(' ')} did not match any Magnetar endpoints for resource ${entry.resource}`
      );
    }

    if (entry.include?.length) {
      for (let name of entry.include) {
        let found = matches.some(
          endpoint =>
            camelToKebab(splitResourceMethod(endpoint.sdkPath).method) === camelToKebab(name)
        );
        if (!found) {
          throw new Error(
            `manifest command ${entry.path.join(' ')} is missing Magnetar method ${name} on ${entry.resource}`
          );
        }
      }
    }

    let controller = controllers.get(matches[0].controllerId);
    let operations = matches
      .map(endpoint =>
        mapOperation({
          endpoint,
          entry,
          type: types,
          skipPathParams: input.manifest.globals.skipPathParams || [],
          pagination: input.manifest.globals.pagination !== false
        })
      )
      .sort((a, b) => a.name.localeCompare(b.name));

    mapped.push({
      entry,
      resource: {
        path: entry.path,
        plural: entry.path[entry.path.length - 1],
        singular: entry.path[entry.path.length - 1],
        short: entry.short || controller?.name || entry.path.join(' '),
        long: entry.long || controller?.description || '',
        aliases: entry.aliases || [],
        operations,
        shortcuts: (entry.shortcuts || []).map(shortcut => ({
          path: shortcut.path,
          method: camelToKebab(shortcut.method)
        })),
        children: []
      }
    });
  }

  return buildTree(mapped);
};

let collectPublicEndpoints = (endpoints: Endpoint[], manifest: Manifest): PublicEndpoint[] => {
  let prefixes = (manifest.exclude?.sdkPathPrefixes || []).map(normalizeSdkPath);
  let excludedResources = new Set(
    (manifest.exclude?.resources || []).map(resource => normalizeSdkPath(resource))
  );
  let collected: PublicEndpoint[] = [];

  for (let endpoint of endpoints) {
    for (let item of endpoint.allPaths) {
      if (item.sdkPath.startsWith('dashboard.') || item.sdkPath.startsWith('consumer.')) {
        continue;
      }
      if (prefixes.some(prefix => normalizeSdkPath(item.sdkPath).startsWith(prefix))) {
        continue;
      }

      let sdkPath = stripSdkPath(item.sdkPath);
      let { resource } = splitResourceMethod(normalizeSdkPath(sdkPath));
      if (excludedResources.has(resource)) continue;

      collected.push({
        ...endpoint,
        sdkPath,
        httpPath: item.path.startsWith('/') ? item.path : `/${item.path}`
      });
    }
  }

  let bySdk = new Map<string, PublicEndpoint>();
  for (let endpoint of collected) {
    let key = `${endpoint.method}:${normalizeSdkPath(endpoint.sdkPath)}`;
    let existing = bySdk.get(key);
    if (!existing) {
      bySdk.set(key, endpoint);
      continue;
    }
    if (scorePath(endpoint.httpPath) < scorePath(existing.httpPath)) {
      bySdk.set(key, endpoint);
    }
  }

  return [...bySdk.values()];
};

let scorePath = (path: string) => {
  let score = path.length;
  if (path.includes('/dashboard/')) score += 1000;
  if (path.includes(':instanceId')) score += 100;
  if (path.includes(':organizationId')) score += 50;
  return score;
};

let mapOperation = (input: {
  endpoint: PublicEndpoint;
  entry: CommandEntry;
  type: Map<string, IntrospectedType>;
  skipPathParams: string[];
  pagination: boolean;
}): MappedOperation => {
  let methodName = camelToKebab(splitResourceMethod(input.endpoint.sdkPath).method);
  let overrides = input.entry.args?.[methodName] || input.entry.args?.[splitResourceMethod(input.endpoint.sdkPath).method] || {};
  let skip = new Set(input.skipPathParams.map(camelToKebab));
  let args: MappedArg[] = [];
  let flags: MappedFlag[] = [];
  let pathParts = input.endpoint.httpPath.split('/').filter(part => part.length > 0);
  let lastSegment = pathParts[pathParts.length - 1] || '';

  for (let placeholder of pathPlaceholders(input.endpoint.httpPath)) {
    let listed =
      skip.has(camelToKebab(placeholder)) || input.skipPathParams.includes(placeholder);
    if (listed && lastSegment !== `:${placeholder}`) {
      continue;
    }

    let override = findOverride(overrides, placeholder);
    if (override?.skip) continue;

    args.push({
      name: override?.name || camelToKebab(placeholder).replace(/-id$/, '-id'),
      target: `path.${placeholder}`,
      required: override?.required ?? true,
      description: override?.name
        ? `${override.name}`
        : `${camelToKebab(placeholder).replace(/-/g, ' ')}`
    });
  }

  let queryType = input.endpoint.queryId ? input.type.get(input.endpoint.queryId) : undefined;
  for (let [key, schema] of Object.entries(extractProperties(queryType))) {
    let override = findOverride(overrides, key);
    if (override?.skip) continue;
    if (override?.positional) {
      args.push({
        name: override.name || camelToKebab(key),
        target: `query.${key}`,
        required: override.required ?? !schema.optional,
        description: schema.description || key
      });
      continue;
    }

    if (!input.pagination && paginationFields.has(key)) continue;

    let flagType = override?.type || inferFlagType(schema);
    if (!flagType) continue;

    flags.push({
      name: override?.flag || camelToKebab(key),
      type: flagType,
      target: `query.${key}`,
      usage: schema.description || key,
      required: override?.required ?? false,
      repeated: flagType === 'string-slice'
    });
  }

  let bodyType = input.endpoint.bodyId ? input.type.get(input.endpoint.bodyId) : undefined;
  for (let [key, schema] of Object.entries(extractProperties(bodyType))) {
    let override = findOverride(overrides, key);
    if (override?.skip) continue;
    if (override?.positional) {
      args.push({
        name: override.name || camelToKebab(key),
        target: `body.${key}`,
        required: override.required ?? !schema.optional,
        description: schema.description || key
      });
      continue;
    }

    let flagType = override?.type || inferFlagType(schema);
    if (!flagType) continue;

    flags.push({
      name: override?.flag || camelToKebab(key),
      type: flagType,
      target: `body.${key}`,
      usage: schema.description || key,
      required: override?.required ?? !schema.optional,
      repeated: flagType === 'string-slice'
    });

    if (flagType === 'json') {
      flags.push({
        name: `${override?.flag || camelToKebab(key)}-file`,
        type: 'json-file',
        target: `body.${key}`,
        usage: `Read ${key} JSON from a file`,
        required: false,
        repeated: false
      });
    }
  }

  return {
    name: methodName,
    method: input.endpoint.method.toUpperCase(),
    path: input.endpoint.httpPath,
    short: input.endpoint.name,
    long: input.endpoint.description || input.endpoint.name,
    args,
    flags,
    sdkMapping: input.endpoint.sdkPath
  };
};

let findOverride = (overrides: Record<string, ArgOverride>, key: string) => {
  if (overrides[key]) return overrides[key];
  let kebab = camelToKebab(key);
  if (overrides[kebab]) return overrides[kebab];
  let camelKey = kebab.replace(/-([a-z])/g, (_, char: string) => char.toUpperCase());
  return overrides[camelKey];
};

let inferFlagType = (schema: IntrospectedType): FlagKind | undefined => {
  switch (schema.type) {
    case 'string':
    case 'enum':
    case 'literal':
    case 'date':
      return 'string';
    case 'boolean':
      return 'bool';
    case 'number':
      return 'float';
    case 'array': {
      let item = schema.items?.[0];
      if (!item || item.type === 'string' || item.type === 'enum' || item.type === 'literal') {
        return 'string-slice';
      }
      return 'json';
    }
    case 'object':
    case 'record':
    case 'intersection':
      return 'json';
    case 'union': {
      let items = schema.items || [];
      if (items.every(item => ['string', 'enum', 'literal'].includes(item.type))) {
        return 'string';
      }
      if (items.some(item => item.type === 'array')) return 'string-slice';
      return undefined;
    }
    default:
      return undefined;
  }
};

export let extractProperties = (
  typeDef?: IntrospectedType
): Record<string, IntrospectedType> => {
  let props: Record<string, IntrospectedType> = {};
  if (!typeDef) return props;

  if (typeDef.type === 'object' && typeDef.properties) {
    Object.assign(props, typeDef.properties);
  } else if (typeDef.type === 'intersection' && typeDef.items) {
    for (let item of typeDef.items) {
      Object.assign(props, extractProperties(item));
    }
  } else if (typeDef.type === 'union' && typeDef.items) {
    for (let item of typeDef.items) {
      for (let [key, prop] of Object.entries(extractProperties(item))) {
        if (!props[key]) {
          props[key] = { ...prop, optional: true };
        }
      }
    }
  }

  return props;
};

let buildTree = (mapped: { entry: CommandEntry; resource: MappedResource }[]) => {
  let byPath = new Map<string, MappedResource>();
  let sorted = [...mapped].sort((a, b) => a.entry.path.length - b.entry.path.length);

  for (let item of sorted) {
    byPath.set(item.entry.path.join('\0'), item.resource);
  }

  let roots: MappedResource[] = [];
  for (let item of sorted) {
    if (item.entry.path.length === 1) {
      roots.push(item.resource);
      continue;
    }

    let parent = byPath.get(item.entry.path.slice(0, -1).join('\0'));
    if (!parent) {
      throw new Error(
        `manifest command ${item.entry.path.join(' ')} is missing parent ${item.entry.path.slice(0, -1).join(' ')}`
      );
    }
    parent.children.push(item.resource);
  }

  return roots;
};
