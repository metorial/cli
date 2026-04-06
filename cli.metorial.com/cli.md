# Metorial CLI

Metorial CLI is the simplest way to work with Metorial from your terminal.

It helps you:

- sign in to your Metorial account
- inspect providers, deployments, sessions, and other resources
- browse and manage integrations
- send authenticated API requests
- start from official example projects

If you are new to Metorial, the core idea is simple: Metorial helps you deploy and manage providers for AI applications. The CLI gives you a direct way to explore those providers, inspect your deployments, and work with runtime sessions from the command line.

## Install

Pick the option that matches your setup.

### macOS or Linux

Use the standalone installer:

```bash
curl -fsSL https://cli.metorial.com/install.sh | bash
```

### npm

Use npm if you already work in Node.js:

```bash
npm install -g @metorial/cli
```

### npx

Use npx if you want to run the CLI without installing it globally:

```bash
npx @metorial/cli@latest
```

### Homebrew

```bash
brew install metorial/tap/metorial
```

### Chocolatey

```powershell
choco install metorial
```

### Scoop

```powershell
scoop bucket add metorial https://github.com/metorial/scoop-bucket
scoop install metorial
```

## Which Install Option Should I Pick?

- Use the installer for a simple standalone install on macOS or Linux.
- Use `npm` if you want a normal global install in a Node.js environment.
- Use `npx` if you only need the CLI occasionally.
- Use `brew`, `choco`, or `scoop` if that is already how you manage tools.

## Understand The Basics

You do not need to memorize the full Metorial model to get started, but these terms help:

- A provider is an integration such as Slack, GitHub, Exa, or another tool source.
- A deployment is the configured instance of that provider that your app can use.
- A session is the runtime layer that gives your AI access to tools from one or more attached providers.
- An organization, project, and instance help you separate work and environments such as development and production.

## First Steps

Sign in first:

```bash
metorial login
```

Then inspect what is available in your account:

```bash
metorial providers list
metorial deployments list
metorial sessions list
metorial integrations list
```

If you want the dashboard too:

```bash
metorial open
```

## Core Commands

### Sign In

```bash
metorial login
```

Starts the browser-based sign-in flow and makes that profile active on your machine.

### Browse Resources

```bash
metorial providers list
metorial deployments list
metorial sessions list
metorial instance list
```

These commands help you understand your current setup, especially when you are checking what providers and deployments are available in a given environment.

### Work With Integrations

```bash
metorial integrations list
metorial integrations catalog list
metorial integrations tools <integration-id>
```

Use these commands to inspect connected integrations, browse the catalog, and see the tools an integration exposes.

### Start From An Example

```bash
metorial example list
metorial example create <identifier>
```

You can also do this through npm:

```bash
npm create metorial@latest
npm create metorial@latest <identifier> [path]
```

### Open Metorial In Your Browser

```bash
metorial open
```

### Send An Authenticated API Request

```bash
metorial fetch <path-or-url>
```

Useful when you want a direct API response from the same authenticated context you are already using in the CLI.

### Check Your Version

```bash
metorial version
```

## Need More?

```bash
metorial --help
metorial <command> --help
```

Docs:

- [Metorial docs](https://metorial.com/docs)
- [Providers](https://metorial.com/docs/concepts-providers.md)
- [Server deployments](https://metorial.com/docs/concepts-server-deployments.md)
- [Sessions](https://metorial.com/docs/concepts-sessions.md)
