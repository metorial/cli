# Metorial CLI

Metorial CLI is the command line interface for Metorial. This CLI is built agent-first, meaning it is designed to be used both by developers directly and by agents that need to interact with Metorial resources and integrations.

## Install

Choose the install method that matches how you normally manage tools.

### npm

Best if you already use Node.js and want `metorial` available globally.

```bash
npm install -g @metorial/cli
```

### npx

Best for one-off usage or quick checks without a global install.

```bash
npx @metorial/cli@latest
```

### Bash Installer

Best on macOS and Linux if you want the standalone CLI without depending on Node.js.

```bash
curl -fsSL https://cli.metorial.com/install.sh | bash
```

### Homebrew

Best if you already manage command line tools with Homebrew.

```bash
brew install metorial/tap/metorial
```

### Chocolatey

Best for Windows setups that already use Chocolatey.

```powershell
choco install metorial
```

### Scoop

Best for Windows setups that prefer Scoop.

```powershell
scoop bucket add metorial https://github.com/metorial/scoop-bucket
scoop install metorial
```

### Start From An Example

If you want to browse or create an official example project first:

```bash
npm create metorial@latest
npm create metorial@latest <identifier> [path]
```

## Which Install Option Should I Use?

- Use `npm` if you want a standard global install in a Node.js environment.
- Use `npx` if you only need the CLI occasionally.
- Use the bash installer if you want a simple standalone install on macOS or Linux.
- Use `brew`, `choco`, or `scoop` if you already manage tools through that package manager.
- Use `npm create metorial@latest` if your main goal is to begin with an official sample project.

## Quick Start

Install the CLI, then start here:

```bash
metorial login
metorial providers list
metorial deployments list
metorial sessions list
metorial integrations list
metorial example list
```

If you want the dashboard as well:

```bash
metorial open
```

## Core Concepts In The CLI

### Organizations, Projects, And Instances

Metorial organizes work by organization, then project, then instance. In practice, an instance is usually the environment you are working against, such as development or production.

When you browse resources in the CLI, you are usually looking at resources available in the current authenticated context and selected instance.

### Providers

Providers are the integrations you connect to Metorial. They define the tools, authentication methods, and configuration options available for a given integration.

The CLI lets you inspect providers and related resources directly:

```bash
metorial providers list
metorial providers get <provider-id>
```

### Deployments

Deployments are the configured, ready-to-use instances of your providers. They are the core building blocks your applications use.

Common commands:

```bash
metorial deployments list
metorial deployments get <deployment-id>
```

### Sessions

Sessions are the runtime layer that connects your application to one or more providers. A session exposes the tools from the attached providers and is useful for testing, debugging, and understanding how your runtime setup behaves.

Common commands:

```bash
metorial sessions list
metorial sessions get <session-id>
```

### Integrations

Integrations help you discover, set up, and manage the providers available through Metorial.

Common commands:

```bash
metorial integrations list
metorial integrations catalog list
metorial integrations get <integration-id>
```

## Main Commands

### Sign In

```bash
metorial login
```

Starts the sign-in flow and saves your profile locally so the CLI can act on your behalf.

### Explore Your Resources

```bash
metorial providers list
metorial deployments list
metorial sessions list
metorial instance list
```

This is the fastest way to understand what is available in your account and environment.

### Work With Integrations

```bash
metorial integrations list
metorial integrations catalog list
metorial integrations tools <integration-id>
```

Use these commands to review connected integrations, browse the catalog, and inspect the tools exposed by an integration.

### Start From Official Examples

```bash
metorial example list
metorial example create <identifier>
```

Use examples when you want a working starting point instead of beginning from scratch.

Examples are also available through `npm create`.

```bash
npm create metorial@latest
npm create metorial@latest <identifier> [path]
```

### Send Authenticated API Requests

```bash
metorial fetch <path-or-url>
```

Useful when you want to inspect an API response directly from the terminal while keeping the same authenticated context.

## Help

Use built-in help to discover commands as you go:

```bash
metorial --help
metorial <command> --help
```

## Learn More

- [Metorial docs](https://metorial.com/docs)
- [Providers](https://metorial.com/docs/concepts-providers)
- [Server deployments](https://metorial.com/docs/concepts-server-deployments)
- [Sessions](https://metorial.com/docs/concepts-sessions)
- [Projects and organizations](https://metorial.com/docs/concepts-projects-organizations)

For a shorter install-first guide, see `cli.metorial.com/cli.md` in this repository or visit [cli.metorial.com](https://cli.metorial.com).
