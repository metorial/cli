import { npmInstallEnvironment, replaceOutput, runCLIAndExit } from '@metorial/cli-core';

let args = process.argv.slice(2);
let cliArgs = args.length === 0 ? ['example', 'list'] : ['example', 'create', ...args];
let transform = replaceOutput('metorial example create', 'npm create');

void runCLIAndExit(cliArgs, {
  env: npmInstallEnvironment('create-metorial'),
  stdoutTransform: transform,
  stderrTransform: transform
});
