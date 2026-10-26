# Private GHSA draft — wazero (tetratelabs/wazero) compile-time OOM

**Status:** Draft for private GitHub Security Advisory / coordinated disclosure  
**Do not publish bomb recipes, fixtures, or PoC binaries to the public site or release notes.**  
**HackMe package pin:** `github.com/tetratelabs/wazero v1.12.0` (`go.mod`)  
**Upstream repo:** https://github.com/wazero/wazero (org moved from tetratelabs)

---

## Title (suggested)

wazero: unbounded LEB vector counts in module decode can OOM the host during `CompileModule`

## Summary

Certain WebAssembly binary sections encode **vector lengths as LEB128 integers**. In affected wazero versions, decode paths allocate based on the **declared count** before validating that the section payload actually contains that many entries. A compact malicious module can therefore force multi-GiB allocations and crash the process (availability DoS / OOM kill) during compile — without needing to execute guest code.

This affects any service that compiles untrusted WASM with wazero (including Dig/Hunt-style check gates).

## Impact

- **Type:** Denial of service (host process OOM) during module compile  
- **Privilege:** Attacker who can submit an arbitrary WASM blob to a compile path  
- **Integrity/confidentiality:** Not claimed; availability only  
- **CVSS (draft, unauthenticated remote compile):** High availability impact when compile is exposed to untrusted input

## Affected component (upstream)

- Runtime: wazero module decode / compile (`CompileModule` and related decode helpers)  
- Shape class: **inflated LEB counts** in export, type/params, code locals, name/custom subsections, tables/elements, and similar vector prefixes  
- Related discussion points for maintainers: allocate-by-declared-count before bounds checks; custom/name subsection parsing

Exact upstream file/function names should be confirmed against the wazero tag under review when filing; do not paste HackMe bomb hex into the public advisory body.

## How we found it (high level)

Internal adversarial WASM review for Dig/Hunt check-gate validation. Repro fixtures and regression tests live **only** in the HackMe private test tree (`internal/sandbox/*bomb*_test.go`, section prechecks). **Do not attach those binaries to a public issue.**

## HackMe mitigations (already shipped)

Defense in depth — upstream fix remains desirable:

1. **Section precheck** (`rejectWasmHostileSections`) — caps top-level and nested LEB vector counts, rejects import/start/unknown sections, bounds tables/elements/data before compile.  
2. **Custom-section strip** (`stripWasmCustomSections`) — drops id=0 (incl. name) so inflated name counts never reach decodeFunctionNames-style paths.  
3. **RLIMIT_AS child compile** (`ValidateCheckWasm` / wasm-validate child) — even novel shapes that slip the filter cannot OOM the parent hub process.

These are HackMe-side controls; other wazero consumers remain exposed until upstream hardens decode.

## Suggested upstream remediation

- Cap or stream-decode vector lengths; refuse counts that cannot fit in remaining section bytes  
- Fail closed on custom/name subsections with absurd counts  
- Add regression tests that assert compile rejects tiny inflated-count modules without large allocations  
- Consider optional “untrusted compile” mode with strict resource budgets

## Disclosure plan

1. Open a **private** GitHub Security Advisory on `wazero/wazero` (requires maintainer collaboration; reporters without admin cannot self-publish).  
2. Attach **redacted** description only (this document’s Summary + Impact + Remediation).  
3. Share PoC **out of band** to maintainers under embargo (not in public commits, site, or release notes).  
4. After fix + assigned GHSA/CVE: public note that HackMe already filtered; credit upstream.

## Reporter note / filing checklist

```text
gh api -X POST repos/wazero/wazero/security-advisories \
  # typically requires repo admin — use GitHub UI “Report a vulnerability”
  # or email security contact if listed in SECURITY.md
```

As of drafting: authenticated `gh` user has **pull** on `wazero/wazero`, not admin — use **Report a vulnerability** on the repo Security tab, or contact maintainers privately. Keep this file under `docs/security/` (operator path); do not sync bomb recipes into `web/site/`.

## Neutral commit hygiene (going forward)

- Prefer messages like: “Harden WASM section precheck before compile”  
- Avoid: “bomb”, “hang_input”, “OOM PoC”, hex dumps of adversarial modules in commit subjects/public site copy
