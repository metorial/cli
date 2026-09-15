export type ArgOverride = {
  flag?: string;
  positional?: boolean;
  name?: string;
  required?: boolean;
  type?: 'string' | 'bool' | 'int' | 'float' | 'string-slice' | 'json';
  target?: 'path' | 'query' | 'body';
  skip?: boolean;
};

export type ShortcutEntry = {
  path: string[];
  method: string;
};

export type CommandEntry = {
  path: string[];
  resource?: string;
  group?: boolean;
  include?: string[];
  aliases?: string[];
  short?: string;
  long?: string;
  args?: Record<string, Record<string, ArgOverride>>;
  shortcuts?: ShortcutEntry[];
};

export type Manifest = {
  apiVersion: string;
  binary: string;
  globals: {
    skipPathParams: string[];
    pagination: boolean;
    bodyEscapeHatches: boolean;
    pathStrategy: 'public';
  };
  exclude: {
    sdkPathPrefixes: string[];
    resources: string[];
  };
  commands: CommandEntry[];
};

export let loadManifest = async (filePath: string): Promise<Manifest> => {
  let text = await Bun.file(filePath).text();
  let parsed = Bun.YAML.parse(text) as Manifest;

  if (!parsed?.apiVersion) {
    throw new Error('manifest is missing apiVersion');
  }
  if (!parsed.binary) {
    throw new Error('manifest is missing binary');
  }
  if (!Array.isArray(parsed.commands) || parsed.commands.length === 0) {
    throw new Error('manifest must define at least one command');
  }

  for (let command of parsed.commands) {
    if (!command.path?.length) {
      throw new Error('manifest command is missing path');
    }
    if (!command.group && !command.resource) {
      throw new Error(`manifest command ${command.path.join(' ')} is missing resource`);
    }
  }

  return parsed;
};
