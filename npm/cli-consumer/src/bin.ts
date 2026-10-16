import { npmInstallEnvironment, runCLIAndExit } from '@metorial/cli-core';

void runCLIAndExit(process.argv.slice(2), {
  env: npmInstallEnvironment('@metorial/cli')
});
