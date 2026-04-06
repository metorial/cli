# `create-metorial`

The official `npm create` entrypoint for Metorial examples.

Use `create-metorial` when you want a fast starting point for a Metorial project instead of setting everything up from scratch.

## What It Is For

Metorial examples are a simple way to see how providers, deployments, sessions, and SDK usage come together in a working project.

This package helps you:

- browse the official example list
- pick an example by identifier
- create a local project from that example

## Create A Project

List available examples:

```bash
npm create metorial@latest
```

Create a project from an example:

```bash
npm create metorial@latest <identifier>
```

Create it in a specific folder:

```bash
npm create metorial@latest <identifier> my-app
```

Or run the package directly:

```bash
npx create-metorial@latest
```

## When To Use It

Use `create-metorial` when:

- you want to explore the official examples first
- you want to start from a working sample app
- you do not need the full CLI installed yet

If you want the full command line experience for login, resource inspection, integrations, and API access, install `@metorial/cli` instead.
