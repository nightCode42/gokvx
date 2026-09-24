package shard

import (
	"bytes"
	"hash/crc32"

	"github.com/nightCode42/gokvx/internal/kverr"
)

// DefaultSlotCount is the number of slots a cluster uses unless configured
// otherwise at bootstrap (KV-DAT-020).
const DefaultSlotCount uint32 = 16384

// castagnoli is the CRC32C (Castagnoli polynomial) table used by the slot
// function. It is built once and only read afterwards; the standard library
// uses hardware acceleration for it where the CPU supports it.
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// Slotter maps keys to slots for a fixed slot count.
//
// Obtain one from [NewSlotter], which validates the count once, so
// [Slotter.Slot] needs no checks on the request path. The zero Slotter has a
// slot count of zero and must not be used. A Slotter is a small immutable
// value, safe for concurrent use and cheap to copy.
type Slotter struct {
	// count is the number of slots; always greater than zero.
	count uint32
}

// NewSlotter returns a Slotter for slotCount slots. It fails with
// INVALID_ARGUMENT when slotCount is zero, since every key must map to some
// slot.
func NewSlotter(slotCount uint32) (Slotter, error) {
	if slotCount == 0 {
		return Slotter{}, kverr.New(kverr.ReasonInvalidArgument, "slot count must be greater than zero")
	}
	return Slotter{count: slotCount}, nil
}

// Count returns the number of slots.
func (s Slotter) Count() uint32 {
	return s.count
}

// Slot returns the slot for key: crc32c(HashInput(key)) mod the slot count
// (KV-DAT-021). The result is always in [0, Count()). Every key maps to a
// slot, including the empty key, which is rejected earlier by request
// validation (KV-DAT-012) but is still routable here.
func (s Slotter) Slot(key []byte) uint32 {
	return crc32.Checksum(HashInput(key), castagnoli) % s.count
}

// HashInput returns the bytes the slot function hashes for key: the partition
// key when there is one, otherwise the whole key (KV-DAT-021).
//
// The partition key is the span between the first '{' in key and the first
// '}' after it, when that span is non-empty. Consequences of this rule, all
// locked by golden tests:
//
//   - "{}x" has empty braces, so the whole key is hashed.
//   - "{a" and "a}{" have no closing brace after the first '{', so the whole
//     key is hashed.
//   - "{{a}}" hashes "{a": the span ends at the first '}'.
//   - "{a}{b}" hashes "a": only the first pair counts.
//
// The returned slice aliases key and must not be modified.
func HashInput(key []byte) []byte {
	start := bytes.IndexByte(key, '{')
	if start < 0 {
		return key
	}
	length := bytes.IndexByte(key[start+1:], '}')
	if length <= 0 { // no closing brace, or empty braces
		return key
	}
	return key[start+1 : start+1+length]
}
