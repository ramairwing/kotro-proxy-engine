# Contributing to Kotro

Thanks for considering a contribution. Kotro is early — this doc is short on purpose, and will grow as real contributors hit real friction.

## Before you start

- **New features go in the Rust engine (`rust/kotro-proxy/`), not Go.** `internal/` (Go, Phase 1) is the historical reference implementation and is moving toward frozen/reference-only status — see [`docs/roadmap/next-steps.md`](docs/roadmap/next-steps.md) for the current state of that transition. If you're fixing a bug that exists in both, a Rust-side fix is what we'll merge; a matching Go fix is welcome but not required.
- **Check [`docs/roadmap/next-steps.md`](docs/roadmap/next-steps.md) first.** It's a live, prioritized list of what's actually planned next, including known gaps between the Go and Rust implementations. If what you want to work on is already listed, that's a good signal it's wanted; if it's not, open an issue before sending a large PR so we can align on approach first.
- **Read [`docs/security/THREAT-MODEL.md`](docs/security/THREAT-MODEL.md) before touching scope/isolation, redaction, or gateway-trust code.** These are the security-load-bearing parts of the codebase; changes there get held to a higher bar.

## Development setup

```bash
# Go (Phase 1 reference)
go test ./...
make build          # builds bin/kotro-proxy (Rust) + bin/mock-upstream (Go)

# Rust (Phase 2, primary target)
cd rust
cargo build
cargo test
```

`cmd/mockupstream` / `bin/mock-upstream` is an offline mock OpenAI + Anthropic SSE server — you don't need real API keys to develop or test against Kotro locally.

## Before opening a PR

- **Run the relevant test suite.** `cargo test` for Rust changes, `go test ./...` for Go changes. If you touched `router/handlers.rs`, `guardrail/`, or `router/scope.rs`, run the full suite (`cargo test -p kotro-proxy`), not just the module you edited — these areas have cross-module invariants (see `docs/roadmap/next-steps.md` P1 for why).
- **If you're changing redaction patterns**, check both `internal/guardrail/pattern.go` and `rust/kotro-proxy/src/guardrail/redactor.rs` — these are meant to stay in sync, and a past audit found them drifting (patterns present in Go, missing in Rust). Add or update a pattern in both, or note explicitly in the PR why you're not.
- **If you're changing cache-key or scope logic**, add a test that exercises the actual request-handling wiring, not just the unit in isolation — see `router::scope::tests` for the pattern (`different_credentials_produce_different_cache_keys_for_identical_request` and friends). A change that's correct in isolation but wired in wrong is exactly the class of bug that slips through unit-only coverage here.
- **Keep claims in the README and code comments honest.** If a feature is partial, a fallback, or not yet enabled by default, say so — see the note in `docs/roadmap/next-steps.md` about the project's own history of overstating feature completeness (the original "semantic cache" was exact-match hashing; the original vector encoder was a stub). We'd rather ship something narrower and true than broader and wrong.

## What's especially welcome right now

- Anything on the P1/P2 lists in [`docs/roadmap/next-steps.md`](docs/roadmap/next-steps.md).
- Bug reports with a minimal repro — this project is young enough that a good repro is often worth more than a PR.
- Documentation fixes, especially anywhere the docs and the code have drifted apart.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).

## Questions

Open an issue. There's no separate chat/forum yet — GitHub issues are the source of truth for now.
