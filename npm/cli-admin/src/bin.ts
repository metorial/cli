import { npmInstallEnvironment, runCLIAndExit } from '@metorial/cli-core';

void runCLIAndExit(process.argv.slice(2), {
  env: npmInstallEnvironment('@metorial/admin'),
  binary: 'metorial-admin'
});
