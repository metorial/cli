import { camelToKebab, pathPlaceholders } from '../case';
import type { IntrospectedType } from '../fetch';
import { extractProperties, type MappedOperation, type MappedResource } from '../join';

type JsonSchema = Record<string, any>;

type SlateParam = { in: 'path' | 'query' | 'body'; name: string };

type SlateTool = {
  key: string;
  name: string;
  description: string;
  category: string;
  method: string;
  path: string;
  instanceScoped: boolean;
  readOnly: boolean;
  destructive: boolean;
  params: Record<string, SlateParam>;
  inputSchema: JsonSchema;
};

type SlateCategory = { key: string; name: string; description: string };

let maxKeyLength = 44;
let destructiveMethods = /^(delete|archive|revoke|remove|terminate|destroy|purge)\b/;
let destructiveMethodNames = new Set([
  'disable',
  'exec',
  'revoke',
  'rollback',
  'rotate-webhook-secret',
  'set'
]);

export let emitSlateContract = (input: { apiVersion: string; resources: MappedResource[] }) => {
  let categories: SlateCategory[] = [];
  let tools: SlateTool[] = [];
  let errors: string[] = [];

  for (let root of input.resources) {
    categories.push({
      key: root.path[0],
      name: root.short,
      description: root.long || `${root.short} resources.`
    });

    for (let resource of flatten(root)) {
      let operationNames = new Set(resource.operations.map(operation => operation.name));
      for (let name of Object.keys(resource.slateKeys)) {
        if (!operationNames.has(name)) {
          errors.push(`slateKeys of ${resource.path.join(' ')} names unknown operation ${name}`);
        }
      }

      for (let name of resource.slateExclude) {
        if (!operationNames.has(name)) {
          errors.push(`slateExclude of ${resource.path.join(' ')} names unknown operation ${name}`);
        }
      }

      for (let operation of resource.operations) {
        if (operation.source.confidential) continue;
        if (resource.slateExclude.includes(operation.name)) continue;

        try {
          tools.push(mapTool(root.path[0], resource, operation));
        } catch (error) {
          errors.push((error as Error).message);
        }
      }
    }
  }

  let keys = new Set<string>();
  for (let tool of tools) {
    if (keys.has(tool.key)) errors.push(`duplicate tool key ${tool.key}`);
    keys.add(tool.key);
  }

  if (errors.length) {
    throw new Error(`Cannot emit slate contract:\n  ${errors.join('\n  ')}`);
  }

  let usedCategories = new Set(tools.map(tool => tool.category));

  return `${JSON.stringify(
    {
      generatedFrom: { apiVersion: input.apiVersion },
      categories: categories.filter(category => usedCategories.has(category.key)),
      tools
    },
    null,
    2
  )}\n`;
};

let flatten = (resource: MappedResource): MappedResource[] => [
  resource,
  ...resource.children.flatMap(flatten)
];

let snake = (value: string) => camelToKebab(value).replace(/-/g, '_');

let mapTool = (
  category: string,
  resource: MappedResource,
  operation: MappedOperation
): SlateTool => {
  let { query, body, instanceScoped } = operation.source;
  let key =
    resource.slateKeys[operation.name] || [...resource.path, operation.name].map(snake).join('_');
  if (key.length > maxKeyLength) {
    throw new Error(`tool key ${key} is ${key.length} chars (max ${maxKeyLength})`);
  }

  let params: Record<string, SlateParam> = {};
  let properties: Record<string, JsonSchema> = {};
  let required: string[] = [];

  let add = (field: string, param: SlateParam, schema: JsonSchema, isRequired: boolean) => {
    if (params[field]) {
      throw new Error(
        `tool ${key} has colliding input ${field} (${params[field].in}.${params[field].name} and ${param.in}.${param.name})`
      );
    }
    params[field] = param;
    properties[field] = schema;
    if (isRequired) required.push(field);
  };

  let keptPathParams = new Set(
    operation.args.filter(arg => arg.target.startsWith('path.')).map(arg => arg.target.slice(5))
  );
  for (let placeholder of pathPlaceholders(operation.path)) {
    if (!keptPathParams.has(placeholder)) {
      throw new Error(`tool ${key} path ${operation.path} has unmapped placeholder :${placeholder}`);
    }
    add(
      snake(placeholder),
      { in: 'path', name: placeholder },
      { type: 'string', description: camelToKebab(placeholder).replace(/-/g, ' ') },
      true
    );
  }

  for (let [location, type] of [
    ['query', query],
    ['body', body]
  ] as const) {
    for (let [field, schema] of Object.entries(inputProperties(type))) {
      add(field, { in: location, name: field }, toJsonSchema(schema), !schema.optional);
    }
  }

  let description =
    operation.long !== operation.short
      ? operation.long
      : [operation.short, resource.long].filter(Boolean).join('. ');

  return {
    key,
    name: operation.short,
    description,
    category,
    method: operation.method,
    path: operation.path,
    instanceScoped,
    readOnly: operation.method === 'GET',
    destructive:
      operation.method === 'DELETE' ||
      destructiveMethods.test(operation.name) ||
      destructiveMethodNames.has(operation.name),
    params,
    inputSchema: {
      $schema: 'http://json-schema.org/draft-07/schema#',
      type: 'object',
      properties,
      ...(required.length ? { required } : {})
    }
  };
};

let isEnumLike = (type: IntrospectedType) =>
  (type.type === 'literal' || type.type === 'enum') && type.examples.length > 0;

// Unlike extractProperties, merges literal values of keys shared across variants (e.g. a discriminator).
let inputProperties = (type?: IntrospectedType): Record<string, IntrospectedType> => {
  if (type?.type !== 'union' || !type.items) return extractProperties(type);

  let properties: Record<string, IntrospectedType> = {};
  for (let item of type.items) {
    for (let [key, property] of Object.entries(extractProperties(item))) {
      let existing = properties[key];
      if (!existing) {
        properties[key] = { ...property, optional: true };
      } else if (isEnumLike(existing) && isEnumLike(property)) {
        properties[key] = {
          ...existing,
          type: 'enum',
          examples: [...new Set([...existing.examples, ...property.examples])],
          nullable: existing.nullable || property.nullable
        };
      }
    }
  }

  return properties;
};

let toJsonSchema = (type: IntrospectedType): JsonSchema => {
  let schema = baseSchema(type);
  if (type.type === 'date') {
    schema.description = type.description
      ? `${type.description} (ISO 8601 date-time)`
      : 'ISO 8601 date-time.';
  } else if (type.description) {
    schema.description = type.description;
  }
  return type.nullable ? withNull(schema) : schema;
};

let enumSchema = (values: any[]): JsonSchema => {
  let types = new Set(values.map(value => typeof value));
  return types.size === 1 ? { type: typeof values[0], enum: values } : { enum: values };
};

let baseSchema = (type: IntrospectedType): JsonSchema => {
  let items = type.items || [];

  switch (type.type) {
    case 'string':
      return { type: 'string' };
    case 'date':
      return { type: 'string' };
    case 'null':
      return { type: 'null' };
    case 'bigint':
      return { type: 'integer' };
    case 'number':
      return { type: 'number' };
    case 'boolean':
      return { type: 'boolean' };
    case 'enum':
      return type.examples.length ? enumSchema(type.examples) : { type: 'string' };
    case 'literal':
      return { type: typeof type.examples[0], const: type.examples[0] };
    case 'array':
      return { type: 'array', items: items[0] ? toJsonSchema(items[0]) : {} };
    case 'record':
      return {
        type: 'object',
        additionalProperties: items[0] && items[0].type !== 'any' ? toJsonSchema(items[0]) : {}
      };
    case 'object':
      return objectSchema(type.properties || {});
    case 'intersection':
      if (items.every(item => ['object', 'intersection'].includes(item.type))) {
        return objectSchema(extractProperties(type));
      }
      return { allOf: items.map(toJsonSchema) };
    case 'union':
      if (items.every(item => item.type === 'literal' || item.type === 'enum')) {
        let schema = enumSchema(items.flatMap(item => item.examples));
        return items.some(item => item.nullable) ? withNull(schema) : schema;
      }
      return { anyOf: items.map(toJsonSchema) };
    default:
      return {};
  }
};

let objectSchema = (properties: Record<string, IntrospectedType>): JsonSchema => {
  let required = Object.entries(properties)
    .filter(([, value]) => !value.optional)
    .map(([key]) => key);

  return {
    type: 'object',
    properties: Object.fromEntries(
      Object.entries(properties).map(([key, value]) => [key, toJsonSchema(value)])
    ),
    ...(required.length ? { required } : {})
  };
};

let withNull = (schema: JsonSchema): JsonSchema => {
  if (schema.type === 'null' || schema.enum?.includes(null)) return schema;
  if (schema.enum && schema.type === undefined) return { ...schema, enum: [...schema.enum, null] };
  if (schema.anyOf) return { ...schema, anyOf: [...schema.anyOf, { type: 'null' }] };
  if (schema.const !== undefined) {
    let { const: value, type, ...rest } = schema;
    return { ...rest, type: [type, 'null'], enum: [value, null] };
  }
  if (typeof schema.type !== 'string') return schema;
  return {
    ...schema,
    type: [schema.type, 'null'],
    ...(schema.enum ? { enum: [...schema.enum, null] } : {})
  };
};
