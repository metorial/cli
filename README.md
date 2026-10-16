# Metorial CLI

Metorial ships two command line tools:

- **`metorial`** — consumer / agent CLI for discovering integrations and calling MCP tools
- **`metorial-admin`** — generated admin CLI for Magnetar API resources, raw requests, and examples

Both are agent-first: they are meant to be used by developers and by agents.

## Install

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

### Bash installer

```bash
curl -fsSL https://cli.metorial.com/install.sh | bash
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

### Start from an example

```bash
npm create metorial@latest
npm create metorial@latest <identifier> [path]
```

`create-metorial` delegates to `metorial-admin example`.

## Consumer CLI (`metorial`)

Sign in, then work with integrations and MCP tools:

```bash
metorial login
metorial integrations list
metorial integrations catalog list
metorial integrations tools <integration-id>
metorial integrations call <integration-id> <tool> -d '{"query":"..."}'
metorial open
```

Resource administration, `fetch`/`curl`, and `example` moved to `metorial-admin`.

## Admin CLI (`metorial-admin`)

```bash
metorial-admin login
metorial-admin providers list
metorial-admin providers deployments list
metorial-admin sessions list
metorial-admin magic-mcp servers list
metorial-admin integrations setup
metorial-admin outposts credentials create <outpost-id> --name laptop
metorial-admin fetch /providers
metorial-admin example list
```

Admin commands are generated from the Magnetar API. To regenerate them (requires a running API):

```bash
make generate
```

The generator reads `manifest/admin.yaml` and writes `internal/generated/admin/catalog.go`.

### Sandbox shells and files

`sandbox machines shell` and `sandbox machines files` are hand-written and connect directly to a running sandbox machine.

```bash
# Interactive login shell (like ssh); dropped connections resume automatically
metorial-admin sandbox machines shell <machine-id>

# Run one command; stdout/stderr stay separate and the exit code is passed through
metorial-admin sandbox machines shell <machine-id> -- ls -la /workspace
metorial-admin sandbox machines shell <machine-id> --cwd /workspace -e DEBUG=1 -- npm test

# Browse and transfer files (remote paths are absolute)
metorial-admin sandbox machines files ls <machine-id> /workspace
metorial-admin sandbox machines files cat <machine-id> /etc/os-release
metorial-admin sandbox machines files download <machine-id> /workspace/report.pdf
metorial-admin sandbox machines files download <machine-id> /workspace ./backup -r
metorial-admin sandbox machines files upload <machine-id> ./app /workspace -r
echo hello | metorial-admin sandbox machines files upload <machine-id> - /tmp/hello.txt
metorial-admin sandbox machines files rm <machine-id> /workspace/tmp -r
```

## Help

```bash
metorial --help
metorial-admin --help
metorial <command> --help
metorial-admin <command> --help
```

## Learn More

- [Metorial docs](https://metorial.com/docs)
- [Providers](https://metorial.com/docs/concepts-providers)
- [Server deployments](https://metorial.com/docs/concepts-server-deployments)
- [Sessions](https://metorial.com/docs/concepts-sessions)
- [Projects and organizations](https://metorial.com/docs/concepts-projects-organizations)

For a shorter install-first guide, see `cli.metorial.com/cli.md` in this repository or visit [cli.metorial.com](https://cli.metorial.com).
