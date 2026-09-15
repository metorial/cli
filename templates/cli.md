# Metorial CLI

Metorial ships two CLIs:

- `metorial` — integrations and MCP tool calling
- `metorial-admin` — Magnetar resource admin, `fetch`, and examples

## Install

### macOS or Linux

```bash
curl -fsSL https://cli.metorial.com/install.sh | bash
```

### npm

```bash
npm install -g @metorial/cli
npm install -g @metorial/admin
```

### npx

```bash
npx @metorial/cli@latest
npx @metorial/admin@latest
```

### Homebrew

```bash
brew install metorial/tap/metorial
brew install metorial/tap/metorial-admin
```

### Scoop

```powershell
scoop bucket add metorial https://github.com/metorial/scoop-bucket
scoop install metorial
scoop install metorial-admin
```

## First steps

```bash
metorial login
metorial integrations list
metorial integrations catalog list
```

Admin resources:

```bash
metorial-admin providers list
metorial-admin sessions list
metorial-admin fetch /providers
metorial-admin example list
```

## Help

```bash
metorial --help
metorial-admin --help
```

Docs: [metorial.com/docs](https://metorial.com/docs)
