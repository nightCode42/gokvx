// Package shard decides where keys live.
//
// The key space is divided into a fixed number of slots (KV-DAT-020). Every
// key maps to exactly one slot through the slot function (KV-DAT-021):
//
//	slot = crc32c(hash_input) mod slot_count
//
// where hash_input is the partition key — the bytes between the first '{' and
// the first '}' after it, when that span is non-empty — or the whole key
// otherwise. Keys that share a partition key share a slot, so a client can
// keep related keys together: every key starting with "{tenant-42}" lands in
// the same slot, and a prefix scan over them touches one shard group.
//
// In Phase 3 an authoritative, replicated slot map assigns slots to shard
// groups (KV-CON-030); this package will then resolve key → slot → group.
// Phases 1 and 2 map every slot to a single group, but the slot function is
// implemented from Phase 1 so keys written then remain correctly routable
// later (KV-DAT-022).
//
// The slot function is permanent. Changing the hash, the brace rule, or the
// arithmetic would silently misroute every key already stored, so its output
// is locked by golden tests (QA-006) and any change requires the maintainer's
// explicit approval (internal/shard/AGENTS.md).
package shard
