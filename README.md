# Gooo repository bootstrap

This repository defines a machine-readable bootstrap contract for new Gooo repositories.
The `.gooo` metacode owns the policy meaning; Go consumes it as an executor and verifier.

The root commit is the single permitted `BOOTSTRAP_EXCEPTION`. After it, changes must
arrive through pull requests. The CI workflow verifies the contract and uploads evidence
for every run.

The contract deliberately excludes this root `README.md` from inventory measurements.

## Operating boundary

The reusable contract separates planning from applying repository mutations. A plan is
caller-owned output and must be generated before an explicit apply operation. Before apply,
the target repository must have zero writes. Unknown GitHub API or ruleset observability is
preserved as `UNKNOWN` with all six required fields; it is never treated as closed.

The contract also makes improvement claims conservative: a same-input-digest integer
before/after pair is required, otherwise the claim is `UNKNOWN`. Global language
self-improvement and external utility likewise remain `UNKNOWN` without evidence.

The executor exposes four stages: `plan` writes a deterministic manifest/dossier to a
caller-owned path, `verify` evaluates observed policy evidence, `conformance` checks the
canonical cases and repeatability, and `evidence` combines the exact inventory, runtime,
test, and artifact measurements. None of these stages writes the target repository.

## Status precedence

`REFUTED` outranks `UNKNOWN`, which outranks `CLOSED`. A single observed post-bootstrap
direct-main commit therefore refutes the policy even when other evidence is unavailable.
