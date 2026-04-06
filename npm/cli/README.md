# `@metorial/cli`

The official npm package for the Metorial CLI.

Metorial helps you deploy and manage providers for AI applications. This package gives you the `metorial` command through npm so you can inspect providers, deployments, sessions, integrations, and related resources directly from your terminal.

## Install

Install globally:

```bash
npm install -g @metorial/cli
```

Or run without installing:

```bash
npx @metorial/cli@latest
```

## What You Can Do

Use the CLI to:

- inspect providers and deployments
- review sessions and integrations
- send authenticated API requests
- start from official examples

## Common Commands

```bash
metorial login
metorial providers list
metorial deployments list
metorial sessions list
metorial integrations list
metorial example list
metorial open
```

## Help

```bash
metorial --help
metorial <command> --help
```

If you want to start from an example project, use `npm create metorial@latest` to browse examples or `npm create metorial@latest <identifier>` to create one directly.
