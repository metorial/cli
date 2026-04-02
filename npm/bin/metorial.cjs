#!/usr/bin/env node

let { run } = require('../lib/runner.cjs');

run(process.argv.slice(2)).catch(error => {
	process.stderr.write(`${error.message}\n`);
	process.exit(1);
});
