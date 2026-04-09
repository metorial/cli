import { npmInstallEnvironment, runCLI } from '@metorial/cli-core';

let PACKAGE_NAME = 'call-tool';
let BINARY_NAME = PACKAGE_NAME;

let ROOT_VALUE_FLAGS = new Set([
  '--api-key',
  '--api-host',
  '--instance',
  '--profile',
  '--format'
]);

let ROOT_BOOLEAN_FLAGS = new Set(['--help', '-h', '--version', '-v']);

let OUTPUT_REPLACEMENTS: Array<[string, string]> = [
  ['metorial integrations catalog search', `${BINARY_NAME} search`],
  ['metorial integrations catalog list', `${BINARY_NAME} search`],
  ['metorial integrations search', `${BINARY_NAME} search`],
  ['metorial integrations setup', `${BINARY_NAME} setup`],
  ['metorial integrations schema', `${BINARY_NAME} schema`],
  ['metorial integrations tools', `${BINARY_NAME} info`],
  ['metorial integrations get', `${BINARY_NAME} info`],
  ['metorial integrations client', `${BINARY_NAME} client`],
  ['metorial integrations install', `${BINARY_NAME} install`],
  ['metorial integrations call', BINARY_NAME]
];

let ROOT_HELP = `Call Tool by Metorial

Call MCP tools exposed through your Metorial integrations.

Usage:
  ${BINARY_NAME} <integration-id> <tool-key> [flags]
  ${BINARY_NAME} <command> [args]

Commands:
  setup [listing]               Create and finish an integration setup session
  search [search]               Search installable integrations
  info <integration-id>         Show provider details and tools for an integration
  schema <integration-id> <tool-key>
                                Show the MCP input schema for a tool
  client list                   List supported local MCP clients
  install <client> <id>         Install an integration into a local client

Flags:
  -d, --data string             JSON tool input, or @- to read from stdin
  -h, --help                    Show help
  --version                     Show the underlying Metorial CLI version

Global Flags:
  --api-key string              API key to use for authenticated requests
  --api-host string             API host or base URL (default: api.metorial.com)
  --instance string             Instance ID to use for organization-scoped tokens
  --profile string              Profile ID to use for authenticated requests
  --format string               Output format: yaml, toml, json, or structured

Examples:
  ${BINARY_NAME} my-integration search_docs --data '{"query":"oauth"}'
  echo '{"query":"oauth"}' | ${BINARY_NAME} my-integration search_docs --data @-
  ${BINARY_NAME} search github
  ${BINARY_NAME} setup github
  ${BINARY_NAME} info my-integration
  ${BINARY_NAME} schema my-integration search_docs
  ${BINARY_NAME} install codex my-integration

The default command is \`call\`, so \`${BINARY_NAME} my-integration my-tool\`
maps to \`metorial integrations call my-integration my-tool\`.

Use "${BINARY_NAME} <command> --help" for more details.
`;

void main();

async function main() {
  let args = process.argv.slice(2);
  let parsed = splitRootArgs(args);
  let env = npmInstallEnvironment(PACKAGE_NAME);

  if (parsed.incompleteRootFlag) {
    process.exitCode = await runCLI(args, { env });
    return;
  }

  if (shouldShowRootHelp(parsed)) {
    process.stdout.write(ROOT_HELP);
    return;
  }

  if (shouldShowRootVersion(parsed)) {
    process.exitCode = await runCLI(parsed.rootArgs, { env });
    return;
  }

  process.exitCode = await runCLI(buildCLIArgs(parsed), {
    env,
    stdoutTransform: patchToolCallOutput,
    stderrTransform: patchToolCallOutput
  });
}

function splitRootArgs(args: string[]) {
  let rootArgs: string[] = [];
  let index = 0;
  let incompleteRootFlag = false;

  while (index < args.length) {
    let current = args[index];

    if (current === '--') {
      break;
    }

    if (ROOT_VALUE_FLAGS.has(current)) {
      rootArgs.push(current);
      if (index + 1 >= args.length) {
        incompleteRootFlag = true;
        break;
      }
      rootArgs.push(args[index + 1]);
      index += 2;
      continue;
    }

    if (matchesInlineRootValueFlag(current)) {
      rootArgs.push(current);
      index += 1;
      continue;
    }

    if (ROOT_BOOLEAN_FLAGS.has(current)) {
      rootArgs.push(current);
      index += 1;
      continue;
    }

    break;
  }

  return {
    rootArgs,
    commandArgs: args.slice(index),
    incompleteRootFlag
  };
}

function matchesInlineRootValueFlag(value: string) {
  for (let flag of ROOT_VALUE_FLAGS) {
    if (value.startsWith(`${flag}=`)) {
      return true;
    }
  }

  return false;
}

function shouldShowRootHelp(parsed: {
  rootArgs: string[];
  commandArgs: string[];
  incompleteRootFlag: boolean;
}) {
  if (parsed.incompleteRootFlag) {
    return false;
  }

  if (parsed.commandArgs.length === 0) {
    return !hasVersionFlag(parsed.rootArgs);
  }

  return parsed.commandArgs.length === 1 && parsed.commandArgs[0] === 'help';
}

function shouldShowRootVersion(parsed: {
  rootArgs: string[];
  commandArgs: string[];
  incompleteRootFlag: boolean;
}) {
  return (
    !parsed.incompleteRootFlag &&
    parsed.commandArgs.length === 0 &&
    hasVersionFlag(parsed.rootArgs)
  );
}

function hasVersionFlag(args: string[]) {
  return args.includes('--version') || args.includes('-v');
}

function buildCLIArgs(parsed: {
  rootArgs: string[];
  commandArgs: string[];
  incompleteRootFlag: boolean;
}) {
  if (parsed.commandArgs[0] === 'help') {
    return [...parsed.rootArgs, ...routeHelpCommand(parsed.commandArgs.slice(1)), '--help'];
  }

  return [...parsed.rootArgs, ...routeCommand(parsed.commandArgs)];
}

function routeHelpCommand(args: string[]) {
  let command = args[0] || 'call';

  switch (command) {
    case 'setup':
      return ['integrations', 'setup'];
    case 'search':
      return ['integrations', 'search'];
    case 'schema':
      return ['integrations', 'schema'];
    case 'info':
    case 'tools':
      return ['integrations', 'tools'];
    case 'client':
      return ['integrations', 'client'];
    case 'install':
      return ['integrations', 'install'];
    case 'call':
    default:
      return ['integrations', 'call'];
  }
}

function routeCommand(args: string[]) {
  let [command, ...rest] = args;

  switch (command) {
    case 'setup':
      return ['integrations', 'setup', ...rest];
    case 'search':
      return ['integrations', 'search', ...rest];
    case 'schema':
      return ['integrations', 'schema', ...rest];
    case 'info':
    case 'tools':
      return ['integrations', 'tools', ...rest];
    case 'client':
      return ['integrations', 'client', ...rest];
    case 'install':
      return ['integrations', 'install', ...rest];
    case 'call':
      return ['integrations', 'call', ...rest];
    default:
      return ['integrations', 'call', ...args];
  }
}

function patchToolCallOutput(value: string) {
  let next = value;

  for (let [searchValue, replaceValue] of OUTPUT_REPLACEMENTS) {
    next = next.replaceAll(searchValue, replaceValue);
  }

  return next;
}
