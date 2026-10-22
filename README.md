# Metorial CLI

Metorial ships two command line tools:

- **`metorial`** — consumer / agent CLI for connecting your portal's integrations and calling MCP tools
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

`metorial` signs you in as a consumer of a Metorial portal. `metorial login` opens the browser, where you pick the portal to use. It then offers the same catalog and integrations as the portal itself.

```bash
metorial login
metorial whoami

# Browse the portal catalog and connect an integration
metorial catalog list
metorial catalog get <catalog-item-id>
metorial setup <catalog-item-id>
metorial request-access <catalog-item-id> --message "Need this for reporting"

# Use and manage your integrations
metorial list
metorial tools <integration-id>
metorial call <integration-id> <tool> -d '{"query":"..."}'
metorial update <integration-id> --name "Work GitHub"
metorial reconnect <catalog-item-id> <integration-id>
metorial remove <integration-id>

# Install into a local MCP client
metorial client list
metorial install codex <integration-id>
metorial tokens list
```

`setup` opens the portal's setup page in your browser, waits for you to finish, and then creates the integration. `reconnect` runs the same flow for an existing integration and keeps its endpoint URL. `install` creates a long-lived MCP token for the client, and `tokens revoke <token-id>` removes it.

Consumer logins are only available to users with a consumer profile on an active portal. Organization administration, `fetch`/`curl`, and `example` live in `metorial-admin`.

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

### Uploading files

`files upload` uploads a local file to Metorial. Directories are zipped automatically, with the directory's contents at the root of the archive.

```bash
metorial-admin files upload ./report.pdf
metorial-admin files upload ./logo.png --purpose organization_image
metorial-admin files upload ./build-context --purpose sandbox_image_build_context
```

## Skills

Both CLIs can edit Metorial skills. `metorial` works with the skills you can access in your portal; `metorial-admin skills` works with every skill in the instance.

```bash
# Work on a skill as a local folder
metorial skills list
metorial skills pull release-notes ./release-notes
metorial skills status ./release-notes
metorial skills push ./release-notes            # --dry-run to preview, --force to overwrite remote changes

# Edit single files
metorial skills files ls release-notes
metorial skills files cat release-notes /SKILL.md
metorial skills files put release-notes /references/api.md -f ./api.md
metorial skills files rm release-notes /references/old.md
```

`pull`, `push` and `status` work the same way in both CLIs. For single files, `metorial-admin` has the generated `skills files list/get/put/delete/move/batch` commands instead of `ls/cat/put -f/rm`.

`pull` records the version of every file in `.metorial/skill.json`. Before each batch of up to 100 changes, `push` checks that none of the files it touches changed remotely, and every change also carries the recorded version as a precondition, so edits someone made in the dashboard in the meantime are not overwritten. Batches are not atomic: if the server rejects a change, the changes before it stay applied and are recorded locally, so `status` shows what is left. `push` never deletes `/SKILL.md`. Markdown files become collaboratively editable documents; other files are uploaded as-is.

These paths are never pushed or pulled: `.git`, `.metorial`, `.DS_Store`, `node_modules`, `.idea`, `.vscode`, `.env`, `.env.*`, `*.swp`, `*~` and `.metorial-*` temporary files. Add up to 30 more patterns to a `.metorialignore` file in the skill folder, one per line; a pattern without `/` matches any file or folder name, one with `/` (including a leading `/`, as in `/drafts`) is matched from the skill root, and a matching folder ignores everything under it (`path.Match` syntax, no `**` or negation; a trailing `/` is ignored). A file with more patterns makes `pull`, `push` and `status` fail until you trim it. `.metorialignore` itself is synced like any other file, so everyone who pulls the skill gets the same rules; you can create it in an empty folder before the first `pull`.

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
