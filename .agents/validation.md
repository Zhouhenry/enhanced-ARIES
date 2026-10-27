# Validation

- Choose checks for the changed behavior; reuse coverage and avoid trivial
  forwarding tests. Assert observable contract behavior, not incidental wrappers
  or structure. Existing tests alone do not justify an extra policy. The full
  suite is for release validation, not every edit.
- Release checks: `make build`, `make test`, `make test-race`, `make lint`,
  `make integration`. Unit/race tests require neither Docker nor paid APIs;
  integration uses real containers and a deterministic fake model endpoint.
- Make configures local Go caches; direct Go commands may need
  `GOCACHE=$PWD/.cache/go-build GOMODCACHE=$PWD/.cache/go-mod`.
- Documentation-only edits need content/link/anchor checks and `git diff --check`.
- Performance changes must reconcile raw events and offline summaries. Preserve
  failed, canceled and unstarted work; missing terminal records or dropped events
  cannot count as complete observations. Missing measurements remain absent.
  Keep occurrence construction failures in the attempt denominator. Verified
  throughput requires independent verification and positive isolation; Goodput
  also requires an explicit deadline and stable candidate capture evidence.
  Retain evaluation results under an authorized Huawei verifier egress assumption,
  but distinguish their scores and throughput from results with confirmed egress
  isolation. The assumption is not a passed isolation check; retain failed or
  missing network evidence and do not count it as fully isolation-verified Goodput.
- Full-chain observability claims require independent coverage denominators and
  exact identities; a readable trace or paired subset is insufficient. Preserve
  partial artifacts and blocked native boundaries. Identity-only mappings never
  establish timing or Trace flows. Keep archived-call and invocation counts
  separate; missing counts or producer health are unknown, never zero loss.
  Track the approved acceptance
  matrix in [observability validation](../docs/observability/VALIDATION_AND_REPRODUCE.md).
- Paid runs require authorization. Use the existing
  `profiles/{hermes,openclaw}-tb2-fix-git-deepseek.json` smoke profiles when relevant.
  Inspect `runs/<run-id>/run-result.json`: harness outcome, evaluation score,
  confirmed isolation, task cleanup/errors, and shared infrastructure observation
  and cleanup/errors. Exit status alone is insufficient.
- After runtime work, check for leaked containers, networks, processes, listeners,
  and credential files. Scan private artifacts without printing secrets.
- Keep credentials, datasets, and run artifacts ignored. Report actual evidence
  and validation gaps.

Details: [Makefile](../Makefile), [run/artifact guide](../docs/run-results.md).
