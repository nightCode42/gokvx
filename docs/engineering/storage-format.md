# Storage Format

The exact on-disk format of a gokvx node's data directory. **Everything on this page is permanent once released**: a node must always be able to read what an earlier release wrote, and a format change needs the maintainer's approval, a new `storage_version`, and an ADR (`internal/storage/AGENTS.md`).

Spec references: §9, `KV-STO-001`–`005`, `KV-STO-009`, `KV-STO-020`–`022`. Code: `internal/storage`.

---

## 1. Data directory

```text
<node.data_dir>/
├── storage_version        # format marker, checked before anything else is read
├── log/                   # the command log: intent
│   ├── 00000000000000000001.log
│   └── 00000000000000000735.log
└── engine/                # the Pebble database: state
```

The command log and the engine are two different structures with two different jobs (spec §9.1). The **log** records every command in order and is the source of truth for recovery; the **engine** holds the state machine's data and is rebuilt from the log after a crash. They are never merged.

## 2. `storage_version`

A text file holding the format version in decimal, followed by a newline:

```text
1
```

`storage.PrepareDataDir` runs before anything else in the directory is read (`KV-STO-009`):

| Directory state | Result |
|---|---|
| Empty or new | The marker is written atomically: temporary file, fsync, rename, directory fsync. |
| Marker names `FormatVersion` | Accepted. |
| Marker names another version | Refused: *"uses storage format N, but this binary supports only format M"*. |
| Marker unreadable | Refused. |
| Data present, no marker | Refused: the contents are not known to be gokvx data. |
| Only a leftover `storage_version.tmp` | Treated as empty: an interrupted earlier initialization. |

Current version: **1**.

## 3. Command log

### Segments

The log is split into segment files named after the index of their first record, zero-padded to 20 digits so that names sort in index order: `00000000000000000001.log`. The log directory holds nothing else; any other entry refuses to open, which catches a misconfigured directory.

A new segment starts when appending a record would take the active one past `storage.wal_segment_bytes`. A record larger than a segment gets a segment of its own. When a segment starts, the previous one is flushed and the new file's directory entry is fsynced, so the segment itself survives a crash.

### Records

Records are numbered from 1 without gaps. Each is laid out in little-endian byte order:

```text
offset  size  field
0       4     length    byte length of the body (index + command)
4       4     checksum  CRC32C (Castagnoli) of the body
8       8     index     the record's log index            ┐ body
16      n     command   the serialized command, n bytes   ┘
```

- The checksum covers the index as well as the command, so a record can never be read under the wrong index.
- `length` is at most 64 MiB. Commands are bounded far lower by `limits.max_request_bytes`; the cap keeps a corrupted length from causing an absurd allocation.
- `TestRecordLayout_KV_STO_002` pins the exact bytes of one record, with a checksum computed independently of the code.

### Durability

A record is acknowledged only once it is durable under `storage.fsync` (`KV-STO-005`):

| Policy | When records reach the disk | What a crash can lose |
|---|---|---|
| `always` (default) | Before `Append` returns | Nothing acknowledged |
| `interval` | Every `storage.fsync_interval`, by a background syncer | Records acknowledged since the last flush |
| `os` | When the operating system decides | Any record the OS has not flushed |

Every policy flushes when a segment is rotated and when the log is closed cleanly. After a failed write or fsync the log refuses every later append, because the kernel may already have dropped the unflushed data.

### Recovery

Opening the log verifies every record of every segment (`KV-STO-003`, `KV-STO-004`):

| Finding | Classification | Result |
|---|---|---|
| All records intact and in sequence | Healthy | Opens. |
| Damage in the **last** segment with no intact later record after it | **Torn tail** — a crash cut the last write short | The segment is truncated before the damage, flushed, and a `WARN` naming the segment and offset is logged. Opens. |
| Damage followed by any intact later record | **Corruption** | Refuses to open. |
| Damage in any segment but the last | **Corruption** | Refuses to open. |
| An intact record with the wrong index | **Corruption** — no crash produces it | Refuses to open. |
| A gap between segments | **Corruption** | Refuses to open. |
| An empty last segment | Healthy — a crash right after rotation | Opens. |

To decide between a torn tail and corruption, recovery searches the bytes after the damage **at every offset** for an intact record with a higher index. The search works even when the damage hit a length field and hides where the next record starts, so corruption in the middle of the log is never mistaken for a torn tail.

A bit flip in the final record is detected by its checksum and truncated like a torn tail (`KV-STO-022`). It can never be applied: the checksum would have to match.

### Verification

| Test | What it proves |
|---|---|
| `TestCrashRecovery_KV_STO_020` | A child process appending with `fsync: always` is killed at random moments — mid-write, mid-fsync, mid-rotation — five times over one directory; every acknowledged record survives intact, and no partial record is ever returned. |
| `TestTornWriteRecovery_KV_STO_021` | The final record cut at **every** byte offset recovers cleanly, with a warning naming the offset. |
| `TestCorruptTailIsDetected_KV_STO_022` | Flips in the final record's command, index, checksum, or length are detected and truncated. |
| `TestMidLogCorruptionRefusesToOpen_KV_STO_004` | Damage before intact records, including to a length field, refuses to open. |
| `FuzzDecodeRecord` | Arbitrary bytes never make the decoder panic, and whatever it accepts re-encodes identically. |

## 4. Engine

The engine is a Pebble database (`ADR-0005`) behind the `storage.Engine` interface (`KV-STO-001`). Pebble's own format version is **pinned** to `FormatValueSeparation` rather than left to Pebble's default, so a format change only ever happens deliberately; raising it upgrades existing stores one way.

How the state machine lays out its keys inside the engine is defined with the MVCC store and will be documented in this file.
