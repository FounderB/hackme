# Contributing

Thanks for your interest in HackMe.

By participating, you agree to follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Before you open a PR

1. Run from the repo root:
   ```bash
   bash scripts/ops/verify_project_health.sh
   ```
   The first run needs the wasm task packs used by `lang_static` and some tests: they are
   gitignored build artifacts — build them once per docs/RUST_CPP_TASKS_QUICKSTART.md
   (rustc with the `wasm32-unknown-unknown` target + clang), via
   `bash scripts/build_task_wasm.sh` and `bash scripts/build_security_task_pack.sh`.
2. Keep changes focused; match existing Go and shell style.
3. Do not commit secrets, databases, or local env files (see `.gitignore` and [docs/SECURITY_REPO.md](docs/SECURITY_REPO.md)).
4. User-facing strings in **new** UI or public docs should be **English**.

## Reporting security issues

Do not open public issues for exploitable vulnerabilities. Contact the operator via [hackme.tech/contacts.html](https://hackme.tech/contacts.html) with reproduction steps and impact.

## Releases

Maintainers tag releases after `scripts/release/make_release_bundle.sh` and `public_release_readiness.sh` pass. Binaries are published on the website and GitHub Releases; source is this repository.
