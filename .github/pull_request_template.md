## Summary
<!-- What and why (1–3 bullets). -->

-

## Test plan
- [ ] `go test` on touched packages (at least `./internal/...` relevant paths)
- [ ] CI green (fmt / vet / govulncheck / tests) when targeting `main`
- [ ] If Hunt/pool/escrow: `bash scripts/tests/hunt_pool_smoke_gate.sh` or equivalent
- [ ] If coordinator/nginx/auth: note how auth was verified (no cleartext admin on public ports)

## Notes
- AI review (Qodo / CodeRabbit) is advisory — **CI is the merge gate**.
- Label `do-not-review` or title `[skip review]` / `WIP` to skip CodeRabbit auto-review.
- Do **not** deploy hub / production from this PR unless cutover checklist says so.
