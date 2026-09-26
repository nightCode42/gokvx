# Testing

How gokvx is tested. The strategy and its thresholds are specified in spec §18; this document defines the conventions that make tests consistent, readable, and traceable to requirements.

---

## 1. Principles

- **Tests are the evidence.** A requirement is `DONE` only when an automated test verifies it (`QA-070`). A claim without a test is not a claim.
- **Deterministic and hermetic.** No real network beyond loopback, no wall-clock dependence, no dependence on execution order (`QA-016`). The same commit gives the same result every time.
- **Readable first.** A test is documentation of behavior; a reader should understand the rule it checks without reading the implementation.
- **Never weaken a test to make a change pass.** If a test is wrong, fix it in a separate, explained commit.

## 2. Test levels

| Level | Location | Build tag | Runs |
|---|---|---|---|
| Unit, property, fuzz | next to the code (`*_test.go`) | none | every commit (`make test`), with `-race` in CI |
| Integration: real gRPC, real engine, real mTLS, stub issuer | `test/integration/` | `integration` | every pull request |
| End-to-end: full stack on Compose and kind, one binary | `test/e2e/` | `e2e` | every pull request (`QA-012`) |
| Linearizability and chaos (P2+) | `test/chaos/`, `cmd/kvcheck` | `chaos` | short on every PR, long nightly (`QA-024`) |
| Benchmarks | `*_test.go` benchmarks, `cmd/kvbench` | none | on demand; results committed to `docs/benchmarks/` |

Integration tests never take in-process shortcuts for gRPC, the storage engine, or TLS (`QA-010`).

## 3. Naming and traceability

Every test that verifies a requirement names it, so the traceability report can be generated from code (`QA-071`).

```go
// TestJWKSRefreshFailureKeepsCache_KV_SEC_012 checks that a failed background
// refresh leaves the last known good key set in service.
// Verifies: KV-SEC-012.
func TestJWKSRefreshFailureKeepsCache_KV_SEC_012(t *testing.T) { ... }
```

- Name: `Test<Subject><Behavior>_<REQUIREMENT_ID>`, with the ID's hyphens replaced by underscores. The primary requirement goes in the name.
- A `// Verifies:` line lists every requirement the test covers, comma-separated, and ends with a period like every other comment (`godot` enforces it). This line is what the traceability tooling reads.
- Tests with no requirement (helpers, regressions) omit the suffix but still describe the behavior: `TestLogRotateKeepsRecordsWhole`.
- Subtest names are short lower-case phrases describing the case: `t.Run("expired token", ...)`.

## 4. Assertions

- `github.com/stretchr/testify` is the assertion library. Only its `require` and `assert` packages are used; `testify/mock` and `testify/suite` are not.
- `require` for preconditions and anything after which the test cannot meaningfully continue; `assert` for independent checks that should all be reported.
- Compare whole values where practical (`assert.Equal(t, want, got)`) rather than field by field, so a failure shows the full difference.
- Error assertions check the **reason**, never the message text:

```go
require.Error(t, err)
assert.Equal(t, kverr.ReasonKeyTooLarge, kverr.ReasonOf(err))
```

## 5. Structure

- **Table-driven tests** for any behavior with more than two cases. Each case has a `name`; the table is declared inside the test function.
- **Arrange, act, assert**, separated by blank lines — no comments needed when the blocks are obvious.
- `t.Parallel()` for tests that share no mutable state.
- `t.Cleanup` rather than `defer` for teardown registered by helpers; helpers call `t.Helper()`.
- `t.TempDir()` for any file the test writes. Never write into the repository or a fixed path.
- Black-box tests (`package foo_test`) for exported behavior; white-box tests (`package foo`) only when an internal invariant cannot be observed from outside.

## 6. Time, concurrency, and leaks

- Time-dependent behavior — token refresh, lease expiry, election timing, backoff, snapshot intervals — is driven by a **fake clock** (`QA-003`). `time.Sleep` is never used to make a test pass.
- Integration and end-to-end tests that must wait for an asynchronous outcome poll with `require.Eventually` and a generous, documented bound.
- Packages that start goroutines verify that none leak, with `go.uber.org/goleak` in `TestMain`:

```go
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
```

- CI runs all unit tests with `-race`; a data race fails the build (`QA-001`).

## 7. Test doubles

- **Hand-written fakes, no generated mocks.** A fake is a small working implementation of an interface — an in-memory `Engine`, a fake `Clock`, a scripted `TokenSource`. Fakes make tests read like the production code and do not break when call order changes.
- Fakes live in a `<package>test` package next to the package they support, following the standard library's `httptest` convention: `internal/storage/storagetest`, `internal/auth/authtest`.
- Every fake is itself tested against the same contract tests as the real implementation, so a fake cannot drift from the behavior it imitates.
- Only interfaces gokvx owns are faked. Third-party code is exercised for real (Pebble on a temp directory, gRPC on a loopback listener), not mocked.

## 8. Property tests

- `pgregory.net/rapid` is used for properties over generated inputs — in particular the MVCC invariants (`QA-004`) and encoders.
- A property states an invariant in one sentence in its doc comment; generators produce the whole valid input space, including edge cases (empty keys at boundaries, maximum sizes, `0x00` bytes).
- A failing case that rapid shrinks is added as a regular table test, so it stays covered even if generation changes.

## 9. Fuzz tests

- Native Go fuzzing (`func FuzzX(f *testing.F)`) covers key encoding, page-token encoding and decoding, and JWT parsing (`QA-005`).
- Every crash found becomes a seed in `testdata/fuzz/<FuzzName>/` and a regular test case.

## 10. Golden tests

- Output that must never change silently — above all the slot function (`QA-006`) — is checked against committed golden files in `testdata/`.
- Golden files are regenerated only with an explicit `-update` flag, and a diff in a golden file is reviewed as a behavior change.

## 11. Coverage

- Thresholds (`QA-002`): ≥ 75% statement coverage of non-generated code; ≥ 85% for `consensus`, `storage`, `mvcc`, and `auth`.
- Coverage is a floor, not a goal. A test that executes code without asserting its behavior is not coverage.
- `make coverage` writes an HTML report locally; CI reports the total on every run.

## 12. Flaky tests

A flaky test is a bug. It is quarantined immediately with `t.Skip("flaky: #<issue>")` and an issue describing the failure, then fixed at its root cause. It is never made to pass by retries or longer sleeps (`QA-016`).
