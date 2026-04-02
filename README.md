# Metorial CLI Release Notes

This directory contains the Go CLI, its release configuration, the hosted installer site, and the npm wrapper package.

## Release flow

1. Push a release tag like `v1.2.3` to `metorial-enterprise`.
2. `.github/workflows/release.yml` runs inside the standalone CLI repository.
3. `goreleaser` builds GitHub Release artifacts and publishes package metadata for Homebrew, Scoop, and Chocolatey.
4. The npm wrapper package version is set to the same version and published to npm.
5. `scripts/build-public.mjs` creates `public/` with `install.sh`, release metadata, and mirrored artifacts for `cli.metorial.com`.
6. `wrangler pages deploy` publishes `public/` to Cloudflare Pages.

## Required secrets

- `CLOUDFLARE_ACCOUNT_ID`
- `CLOUDFLARE_API_TOKEN`
- `CLOUDFLARE_CLI_PAGES_PROJECT`
- `NPM_TOKEN`

Optional package-manager publishing secrets:

- `HOMEBREW_TAP_GITHUB_TOKEN`
- `SCOOP_BUCKET_GITHUB_TOKEN`
- `CHOCOLATEY_API_KEY`

If the optional package-manager secrets are not set, GoReleaser still builds the formulas and manifests locally in `dist/`, but it skips publishing those package feeds.
