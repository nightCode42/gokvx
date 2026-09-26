# internal/shard — Agent Notes

The slot function, the slot map, and request routing. The slot function decides where every key lives — **forever**. Read spec §7.3, §10.3 and [system-invariants §5](../../docs/engineering/system-invariants.md#5-slots) before editing.

## Invariants

- `slot = crc32c(hash_input) mod slot_count`, Castagnoli polynomial (`KV-DAT-021`).
- `hash_input` is the substring between the first `{` and the first following `}` when non-empty; otherwise the whole key. `{}` is not a partition key.
- The slot count is fixed at bootstrap and immutable; default 16384 (`KV-DAT-020`).
- The slot map is authoritative, replicated, and versioned with a monotonically increasing version (`KV-CON-030`, P3).
- Every node can resolve any key to its owning group (`KV-CON-031`, P3).

## Stop and ask before

- **Any** change to the slot function, its hash, the brace rule, or the default slot count. A change would silently misroute every existing key.
- Changing golden test data. A golden-file diff is a behavior change, not a test fix.

## Required tests

- Golden tests fixing the slot for a committed set of keys, including braces edge cases, empty braces, nested and unbalanced braces, and binary keys (`QA-006`).
- The worked examples from spec §7.3 as table tests.
