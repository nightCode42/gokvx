package shard_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nightCode42/gokvx/internal/kverr"
	"github.com/nightCode42/gokvx/internal/shard"
)

// mustSlotter returns a Slotter for count, failing the test on error.
func mustSlotter(t *testing.T, count uint32) shard.Slotter {
	t.Helper()

	s, err := shard.NewSlotter(count)
	require.NoError(t, err)
	return s
}

// TestDefaultSlotCount_KV_DAT_020 checks the default size of the slot space.
// Verifies: KV-DAT-020.
func TestDefaultSlotCount_KV_DAT_020(t *testing.T) {
	t.Parallel()

	assert.Equal(t, uint32(16384), shard.DefaultSlotCount)
}

// TestSlotUsesCRC32C_KV_DAT_021 checks the hash against values published
// independently of this code: the standard CRC32C check value and the RFC 3720
// (iSCSI) test vectors. With a slot count of 2^32-1 the modulo leaves these
// checksums unchanged, so the slot equals the raw CRC.
// Verifies: KV-DAT-021.
func TestSlotUsesCRC32C_KV_DAT_021(t *testing.T) {
	t.Parallel()

	s := mustSlotter(t, math.MaxUint32)

	tests := []struct {
		name string
		key  []byte
		want uint32
	}{
		{"check value", []byte("123456789"), 0xE3069283},
		{"32 zero bytes", make([]byte, 32), 0x8A9136AA},
		{"32 0xFF bytes", bytes.Repeat([]byte{0xFF}, 32), 0x62A8AB43},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, s.Slot(tt.key))
		})
	}
}

// TestSlotWorkedExamples_KV_DAT_021 checks the worked examples in spec §7.3.
// Verifies: KV-DAT-021, KV-DAT-022.
func TestSlotWorkedExamples_KV_DAT_021(t *testing.T) {
	t.Parallel()

	s := mustSlotter(t, shard.DefaultSlotCount)

	// "/app/config": no partition key, so the whole key is hashed.
	assert.Equal(t, []byte("/app/config"), shard.HashInput([]byte("/app/config")))

	// "{tenant-42}/users/1" and "/2": both hash "tenant-42" and are colocated.
	assert.Equal(t, []byte("tenant-42"), shard.HashInput([]byte("{tenant-42}/users/1")))
	assert.Equal(t, s.Slot([]byte("{tenant-42}/users/1")), s.Slot([]byte("{tenant-42}/users/2")))
	assert.Equal(t, s.Slot([]byte("tenant-42")), s.Slot([]byte("{tenant-42}/users/1")))

	// "{}/x": empty braces are not a partition key, so the whole key is hashed.
	assert.Equal(t, []byte("{}/x"), shard.HashInput([]byte("{}/x")))
}

// TestHashInput checks the brace rule on its edge cases.
// Verifies: KV-DAT-021.
func TestHashInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"empty key", "", ""},
		{"no braces", "plain/key", "plain/key"},
		{"partition key at start", "{user-1}/profile", "user-1"},
		{"partition key in middle", "app/{user-1}/profile", "user-1"},
		{"partition key at end", "app/{user-1}", "user-1"},
		{"whole key is a partition key", "{user-1}", "user-1"},
		{"empty braces", "{}", "{}"},
		{"empty braces then a pair", "{}{a}", "{}{a}"},
		{"opening brace only", "{abc", "{abc"},
		{"closing brace only", "abc}", "abc}"},
		{"closing before opening", "a}b{c", "a}b{c"},
		{"closing before a valid pair", "a}{b}", "b"},
		{"nested braces", "{{a}}", "{a"},
		{"two pairs", "{a}{b}", "a"},
		{"lone braces", "{", "{"},
		{"single character partition key", "{x}", "x"},
		{"spaces inside braces", "{ }", " "},
		{"binary bytes inside braces", "\x00{\xff\x00}\x01", "\xff\x00"},
		{"invalid UTF-8", "\xc3\x28{k}", "k"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []byte(tt.want), shard.HashInput([]byte(tt.key)))
		})
	}
}

// TestSlotColocatesPartitionKeys_KV_DAT_021 checks that every key sharing a
// partition key lands in the same slot, whatever surrounds it.
// Verifies: KV-DAT-021.
func TestSlotColocatesPartitionKeys_KV_DAT_021(t *testing.T) {
	t.Parallel()

	s := mustSlotter(t, shard.DefaultSlotCount)
	want := s.Slot([]byte("order-7"))

	for _, key := range []string{
		"{order-7}",
		"{order-7}/lines/1",
		"{order-7}/lines/2",
		"shipments/{order-7}",
		"x{order-7}y{other}",
		"\x00{order-7}\xff",
	} {
		assert.Equal(t, want, s.Slot([]byte(key)), "key %q", key)
	}
}

// TestSlotIsInRange checks that every slot lies in [0, count) for several
// slot counts, including a single slot.
func TestSlotIsInRange(t *testing.T) {
	t.Parallel()

	keys := []string{"", "a", "{a}", "{}", "/app/config", "\xff\xfe\xfd", "{tenant-42}/users/1"}

	for _, count := range []uint32{1, 2, 7, 1024, shard.DefaultSlotCount, math.MaxUint32} {
		s := mustSlotter(t, count)
		assert.Equal(t, count, s.Count())

		for _, key := range keys {
			assert.Less(t, s.Slot([]byte(key)), count, "count %d, key %q", count, key)
		}
	}
}

// TestSlotSingleSlot checks that a one-slot space maps every key to slot 0,
// as a Phase 1 or Phase 2 cluster effectively does.
func TestSlotSingleSlot(t *testing.T) {
	t.Parallel()

	s := mustSlotter(t, 1)

	for _, key := range []string{"", "a", "{a}", "/app/config"} {
		assert.Zero(t, s.Slot([]byte(key)))
	}
}

// TestNewSlotterRejectsZero checks that a slot count of zero is refused, so
// Slot never divides by zero.
func TestNewSlotterRejectsZero(t *testing.T) {
	t.Parallel()

	_, err := shard.NewSlotter(0)

	require.Error(t, err)
	assert.Equal(t, kverr.ReasonInvalidArgument, kverr.ReasonOf(err))
}

// TestHashInputDoesNotCopy checks that HashInput returns a view into the key
// rather than an allocation, so the request path stays allocation-free.
func TestHashInputDoesNotCopy(t *testing.T) {
	t.Parallel()

	key := []byte("{abc}/rest")
	input := shard.HashInput(key)

	require.Equal(t, []byte("abc"), input)
	assert.Same(t, &key[1], &input[0])
}

// TestSlotDoesNotAllocate checks that computing a slot allocates nothing.
func TestSlotDoesNotAllocate(t *testing.T) {
	s := mustSlotter(t, shard.DefaultSlotCount)
	key := []byte("{tenant-42}/users/1")

	allocs := testing.AllocsPerRun(100, func() { _ = s.Slot(key) })

	assert.Zero(t, allocs)
}
