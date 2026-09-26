# Configuration Reference

Every setting of a gokvx node, with its type, default, and effect (`KV-CFG-004`). The normative outline is spec [Appendix D](requirements.md#appendix-d--configuration-reference); automated tests keep this page, Appendix D, and the code identical.

---

## Sources and precedence

A node assembles its configuration from four layers. Each layer is optional and overrides the ones before it (`KV-CFG-001`):

| Layer | Example | Typical use |
|---|---|---|
| 1. Built-in defaults | — | Everything not set elsewhere |
| 2. YAML file, given with `--config` | `storage:` `fsync: always` | The bulk of a deployment's settings; in Kubernetes, a mounted ConfigMap |
| 3. Environment variables | `GOKVX_STORAGE_FSYNC=always` | Per-environment overrides; in Kubernetes, the pod spec |
| 4. Command-line flags | `--storage.fsync=always` | One-off overrides |

The same sources are read, in the same order, in every environment. `node.environment` does not choose where settings come from; it only decides which unsafe settings are permitted.

**Unknown keys are errors.** A key in the file, a `GOKVX_*` variable, or a flag that does not exist stops the node from starting and names the key, so a typo in a security setting can never silently fall back to its default (`KV-CFG-020`).

## Environment variables

- The name is `GOKVX_` followed by the key in upper case with dots replaced by underscores: `auth.jwt.clock_skew` is `GOKVX_AUTH_JWT_CLOCK_SKEW`. Every key maps to exactly one name.
- Lists are comma-separated: `GOKVX_AUTH_JWT_AUDIENCE=gokvx,gokvx-admin`.
- Durations use Go syntax: `100ms`, `20s`, `15m`, `1h30m`.
- Kubernetes injects variables such as `GOKVX_CLIENT_SERVICE_HOST` and `GOKVX_CLIENT_PORT_2379_TCP` for every Service in a namespace. They share the prefix but are not configuration, so they are ignored rather than rejected. Setting `enableServiceLinks: false` on gokvx pods avoids them altogether.

## Command-line flags

Every key is also a flag of the same name — `--node.id`, `--auth.jwt.audience=gokvx,gokvx-admin` — and only flags given explicitly override the other layers. Secrets are never accepted as flags (`KV-CFG-005`); see [Secrets](#secrets).

## Validating a configuration

```bash
gokvx config validate --config gokvx.yaml
```

The command applies all four layers and checks every rule without starting the node (`KV-CFG-003`). If anything is wrong, it lists **every** violation by key and exits non-zero (`KV-CFG-002`). Checking happens in two stages: unknown keys and values of the wrong type are reported first, because no rule can be checked until every value is understood; once those are fixed, all the rules are checked together. `--print` also shows the effective configuration, with credentials redacted. A node runs the same checks at startup and refuses to start on any violation.

Beyond individual values, validation enforces the rules between them:

- `auth.mode: disabled` requires `node.environment: development` (`KV-SEC-041`), and every listener must be on loopback unless `auth.insecure_allow_remote` is `true` (`KV-SEC-042`).
- `tls.enabled: false` requires `auth.mode: disabled` (`KV-SEC-001`).
- With authentication enabled, `auth.jwt.issuer_url` and `auth.jwt.jwks_url` must be absolute HTTPS URLs; with TLS enabled, `tls.allowed_client_sans` must name at least one client; with peers configured, the peer CA and `tls.allowed_peer_sans` are required.
- The four scopes must all differ, so no scope can ever grant another (`KV-SEC-022`).
- `cluster.raft.election_ticks` must be at least ten times `cluster.raft.heartbeat_ticks` (`KV-CON-004`).
- Keys, values, and limits must fit within `limits.max_request_bytes`.

## Unsafe settings

Each of these settings logs its own warning at startup, naming the setting and what it costs (`KV-CFG-021`). None is a default.

| Setting | Consequence |
|---|---|
| `auth.mode: disabled` | Mutual TLS and token verification are off; any caller that can connect may read and write every key. |
| `auth.insecure_allow_remote: true` | A node with authentication disabled may accept connections from the network. |
| `tls.enabled: false` | Traffic is unencrypted and callers are not identified by certificate. |
| `storage.fsync: interval` | A crash can lose writes acknowledged within the last `storage.fsync_interval`. |
| `storage.fsync: os` | A crash can lose any acknowledged write the operating system has not flushed. |

## Secrets

The configuration holds no secret values. Private keys and credentials live in files — in Kubernetes, mounted from Secret Manager — and only their paths appear here (`KV-CFG-005`). When the effective configuration is logged or printed, credentials embedded in a URL, as in `https://user:password@host`, are replaced with `REDACTED` (`KV-CFG-006`).

## Keys

"—" means the key has no default: it is empty until configured.

<!-- keys:start -->
| Key | Type | Default | Description |
|---|---|---|---|
| **Node** | | | |
| `node.id` | string | `gokvx-0` | Stable identity of the node; in Kubernetes, derived from the pod ordinal. |
| `node.environment` | string | `production` | `production`, `staging`, or `development`. Only `development` permits disabling authentication. |
| `node.data_dir` | string | `/var/lib/gokvx` | Directory holding the command log and the storage engine's data. |
| `node.shutdown_grace` | duration | `20s` | Deadline for graceful shutdown; must be shorter than the pod's `terminationGracePeriodSeconds` (`KV-CFG-011`). |
| **Listeners** | | | |
| `listen.client` | string | `0.0.0.0:2379` | gRPC API listener, `host:port`. |
| `listen.peer` | string | `0.0.0.0:2380` | Raft transport listener, `host:port`. |
| `listen.metrics` | string | `127.0.0.1:9100` | Prometheus listener; never exposed publicly (`KV-OBS-010`). |
| `listen.diagnostics` | string | `127.0.0.1:6060` | pprof listener; empty disables it (`KV-CFG-014`). |
| `advertise.client` | string | — | Address clients use to reach this node; empty means `listen.client`. |
| `advertise.peer` | string | — | Address peers use to reach this node; empty means `listen.peer`. |
| **Authentication** | | | |
| `auth.mode` | string | `enabled` | `enabled` or `disabled`. Disabled turns off mutual TLS and token verification; development only (`KV-SEC-040`). |
| `auth.insecure_allow_remote` | bool | `false` | Lets a node with authentication disabled listen beyond loopback (`KV-SEC-042`). |
| `auth.jwt.issuer_url` | string | — | Expected `iss` claim; an HTTPS URL. Required while authentication is enabled. |
| `auth.jwt.jwks_url` | string | — | HTTPS URL of the issuer's signing keys (`KV-SEC-018`). Required while authentication is enabled. |
| `auth.jwt.audience` | list | `gokvx` | Accepted `aud` claim values. |
| `auth.jwt.algorithms` | list | `RS256` | Accepted signing algorithms: RSA, RSA-PSS, or ECDSA only; never `none` or HMAC (`KV-SEC-014`). |
| `auth.jwt.clock_skew` | duration | `1m` | Tolerance applied to `exp`, `nbf`, and `iat` (`KV-SEC-015`). |
| `auth.jwt.refresh_interval` | duration | `15m` | How often the key set is refreshed in the background (`KV-SEC-012`). |
| `auth.jwt.min_refresh_interval` | duration | `1m` | Minimum gap between refreshes triggered by an unknown key ID (`KV-SEC-013`). |
| `auth.jwt.http_timeout` | duration | `5s` | Timeout of each key-set request. |
| `auth.jwt.max_response_bytes` | integer | `1048576` | Largest accepted key-set response. |
| `auth.jwt.min_rsa_bits` | integer | `2048` | Smallest accepted RSA modulus; at least 2048. |
| `auth.jwt.claims_cache.max_entries` | integer | `10000` | Size of the verified-token cache; `0` disables it (`KV-SEC-017`). |
| `auth.scopes.read` | string | `kv:read` | Scope required by `Get` and `List`. |
| `auth.scopes.write` | string | `kv:write` | Scope required by mutations and lease operations. |
| `auth.scopes.watch` | string | `kv:watch` | Scope required by `Watch`. |
| `auth.scopes.admin` | string | `kv:admin` | Scope required by compaction, membership, and slot moves. |
| **TLS** | | | |
| `tls.enabled` | bool | `true` | Mutual TLS on every listener; may be `false` only while authentication is disabled (`KV-SEC-001`). |
| `tls.min_version` | string | `1.3` | Lowest accepted protocol version: `1.3`, or `1.2` by explicit choice (`KV-SEC-002`). |
| `tls.cert_file` | string | `/etc/gokvx/tls/tls.crt` | Path of the node's certificate. |
| `tls.key_file` | string | `/etc/gokvx/tls/tls.key` | Path of the node's private key. |
| `tls.client_ca_file` | string | `/etc/gokvx/tls/client-ca.crt` | Path of the CA bundle trusted for clients (`KV-SEC-007`). |
| `tls.peer_ca_file` | string | `/etc/gokvx/tls/peer-ca.crt` | Path of the CA bundle trusted for peers. |
| `tls.reload` | bool | `true` | Reload certificates when their files change, without a restart (`KV-SEC-005`). |
| `tls.allowed_peer_sans` | list | — | Certificate SANs, exact or patterns, accepted from peers; required when peers exist (`KV-SEC-004`). |
| `tls.allowed_client_sans` | list | — | Certificate SANs, exact or patterns, accepted from clients; required while TLS is enabled. |
| **Limits** | | | |
| `limits.max_key_bytes` | integer | `1024` | Longest accepted key (`KV-DAT-011`). |
| `limits.max_value_bytes` | integer | `1048576` | Largest accepted value (`KV-DAT-011`). |
| `limits.max_request_bytes` | integer | `4194304` | Largest accepted request message (`KV-DAT-013`). |
| `limits.max_concurrent_streams` | integer | `1000` | Concurrent streams per connection (`KV-API-006`). |
| `limits.max_watchers_per_principal` | integer | `100` | Open watches per principal (`KV-SEC-033`). |
| `limits.rate_limit.read.rate` | number | `5000` | Sustained read requests per second, per principal (`KV-SEC-032`). |
| `limits.rate_limit.read.burst` | integer | `10000` | Read requests admitted at once. |
| `limits.rate_limit.write.rate` | number | `1000` | Sustained write requests per second, per principal. |
| `limits.rate_limit.write.burst` | integer | `2000` | Write requests admitted at once. |
| `limits.rate_limit.admin.rate` | number | `10` | Sustained administrative requests per second, per principal. |
| `limits.rate_limit.admin.burst` | integer | `20` | Administrative requests admitted at once. |
| **Storage** | | | |
| `storage.engine` | string | `pebble` | Storage engine; `pebble` is the only one (`KV-STO-001`). |
| `storage.fsync` | string | `always` | `always` flushes before acknowledging each write; `interval` and `os` trade durability for speed (`KV-STO-005`). |
| `storage.fsync_interval` | duration | `100ms` | Flush period when `storage.fsync` is `interval`. |
| `storage.wal_segment_bytes` | integer | `67108864` | Size of each command log segment (`KV-STO-002`). |
| `storage.wal_retain_segments` | integer | `4` | Segments kept beyond the latest snapshot (`KV-STO-008`). |
| `storage.snapshot_entries` | integer | `10000` | Applied entries between snapshots (`KV-STO-006`). |
| `storage.snapshot_interval` | duration | `30m` | Longest time between snapshots. |
| `storage.quota_bytes` | integer | `8589934592` | Database size above which writes are refused while reads continue (`KV-STO-010`). |
| `storage.auto_compaction_retention` | integer | `1000` | Revisions kept by automatic compaction; `0` disables it (`KV-DAT-007`). |
| **Cluster** | | | |
| `cluster.bootstrap` | bool | `false` | Create a new cluster instead of joining an existing one. |
| `cluster.initial_peers` | list | — | Founding members, each `name=host:port`. |
| `cluster.slot_count` | integer | `16384` | Size of the slot space; fixed when the cluster is created (`KV-DAT-020`). |
| `cluster.groups` | integer | `1` | Shard groups created at bootstrap. |
| `cluster.replication_factor` | integer | `3` | Replicas per shard group; odd sizes tolerate the most failures (`KV-CON-014`). |
| `cluster.raft.tick_interval` | duration | `100ms` | Length of one Raft tick. |
| `cluster.raft.heartbeat_ticks` | integer | `1` | Heartbeat interval, in ticks. |
| `cluster.raft.election_ticks` | integer | `10` | Election timeout, in ticks; at least ten heartbeats (`KV-CON-004`). |
| `cluster.raft.max_inflight_msgs` | integer | `256` | Unacknowledged append messages per peer. |
| `cluster.raft.max_size_per_msg` | integer | `1048576` | Largest append message. |
| `cluster.raft.pre_vote` | bool | `true` | PreVote; only `true` is accepted (`KV-CON-005`). |
| `cluster.raft.check_quorum` | bool | `true` | CheckQuorum; only `true` is accepted (`KV-CON-006`). |
| `cluster.raft.leader_transfer_on_shutdown` | bool | `true` | Hand leadership over before a leader shuts down (`KV-CON-013`). |
| `cluster.read.default_consistency` | string | `linearizable` | Mode for reads that specify none; only `linearizable` is accepted (`KV-API-080`). |
| `cluster.read.forward_writes_to_leader` | bool | `true` | Forward writes received by a follower rather than rejecting them (`KV-CON-008`). |
| `cluster.read.leader_wait_timeout` | duration | `3s` | How long a request waits for a leader before failing (`KV-CON-009`). |
| **Observability** | | | |
| `observability.log.level` | string | `info` | `debug`, `info`, `warn`, or `error`. |
| `observability.log.format` | string | `json` | `json`; `text` is permitted in development only (`KV-OBS-020`). |
| `observability.tracing.enabled` | bool | `true` | Export OpenTelemetry traces. |
| `observability.tracing.otlp_endpoint` | string | `otel-collector.observability.svc:4317` | OTLP collector address; required while tracing is enabled. |
| `observability.tracing.sampler` | string | `parentbased_traceidratio` | OpenTelemetry sampler name (`KV-OBS-005`). |
| `observability.tracing.sample_ratio` | number | `0.05` | Fraction of traces sampled, between 0 and 1. |
| `observability.metrics.enabled` | bool | `true` | Serve Prometheus metrics on `listen.metrics`. |
| `observability.health.max_apply_lag_entries` | integer | `1000` | Largest gap between committed and applied entries at which the node reports ready (`KV-OBS-032`). |
<!-- keys:end -->
