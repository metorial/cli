# Metorial CLI Release Notes

This directory contains the Go CLI, its GoReleaser configuration, and the hosted installer site for `cli.metorial.com`.

## Release flow

1. Push a release tag like `v1.2.3` to `metorial/cli`.
2. `.github/workflows/release.yml` runs inside the standalone CLI repository.
3. `goreleaser` builds GitHub Release artifacts and publishes package metadata for Homebrew, Scoop, and Chocolatey.
4. `scripts/build-public.ts` fetches every published GitHub release and mirrors its assets into `public/metorial-cli/<version-tag>/`.
5. The script writes `public/metorial-cli/latest` with the latest version tag and copies `public/install.sh`.
6. `wrangler pages deploy` publishes `public/` to Cloudflare Pages for `cli.metorial.com`.

## Required secrets

- `CLOUDFLARE_ACCOUNT_ID`
- `CLOUDFLARE_API_TOKEN`
- `CLOUDFLARE_CLI_PAGES_PROJECT`

Optional package-manager publishing secrets:

- `HOMEBREW_TAP_GITHUB_TOKEN`
- `SCOOP_BUCKET_GITHUB_TOKEN`
- `CHOCOLATEY_API_KEY`

If the optional package-manager secrets are not set, GoReleaser still builds the formulas and manifests locally in `dist/`, but it skips publishing those package feeds.
