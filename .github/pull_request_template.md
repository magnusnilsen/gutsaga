## Summary

<!-- What changed, and why? -->

## Area

- [ ] addon
- [ ] companion (`cmd/gutsaga`, `internal/`)
- [ ] api
- [ ] smia (`smia/wow-mcp`)
- [ ] docs
- [ ] ci

## Test plan

<!-- How did you check it? Delete lines that do not apply. -->

- [ ] `go test ./...` against a database whose name contains `test`
- [ ] `luacheck addon`
- [ ] `lua5.1 addon/objectives_test.lua` and `lua5.1 addon/chains_test.lua`
- [ ] Companion or in-game smoke test (note the client)

## Checklist

- [ ] Commit messages use conventional commits (`type(scope): subject`)
- [ ] No secrets, `.env`, or a filled-in `config.json`
- [ ] Docs updated when behavior changes
