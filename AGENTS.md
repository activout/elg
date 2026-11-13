# Agent guide: activout/elg

elg is a Go CLI for GleSYS. It uses our fork of the SDK,
[activout/glesys-go](https://github.com/activout/glesys-go), via a `replace` in `go.mod`.
Work in a worktree on a new branch from fresh `origin/main` and open a PR.

## Keeping in sync with upstream glesys-go

The fork's `AGENTS.md` describes merging `glesys/glesys-go` into its `elg-support` branch.
After that PR merges:

```sh
go mod edit -replace github.com/glesys/glesys-go/v8=github.com/activout/glesys-go/v8@<merge-commit-sha>
go mod tidy && go vet ./... && go test ./... && go test -race ./...
```

Then check `docs/sdk-coverage.md` for new upstream endpoints worth exposing, and release.

## Releasing

1. In a PR: write `docs/release-notes-vX.Y.Z.md` (used as the GitHub release notes) and
   merge it to `main`. Use a minor bump for new commands or SDK features, a patch bump for fixes.
2. Tag the merge commit and push the tag: `git tag vX.Y.Z origin/main && git push origin vX.Y.Z`.
   The `Release` workflow vets, tests, builds, attests and publishes archives and `checksums.txt`.
   To republish an existing tag: `gh workflow run release.yml --ref vX.Y.Z -f version=vX.Y.Z`.
3. The Homebrew formula in [activout/homebrew-tap](https://github.com/activout/homebrew-tap)
   updates itself from the latest release (daily 03:17 UTC). To publish it now:
   `gh workflow run update.yml -R activout/homebrew-tap`. Never edit `Formula/elg.rb` by hand.
4. Verify: `gh release view vX.Y.Z -R activout/elg` and `brew upgrade activout/tap/elg && elg version`.
