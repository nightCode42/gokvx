package shard_test

import (
	"bytes"
	"testing"

	"github.com/nightCode42/gokvx/internal/shard"
)

// FuzzSlot checks, for arbitrary keys and slot counts, that the slot function
// never panics, always stays in range, is deterministic, and depends only on
// the hash input chosen by the brace rule.
// Verifies: KV-DAT-021.
func FuzzSlot(f *testing.F) {
	for _, key := range []string{"", "{}", "{a}", "{{a}}", "{a}{b}", "a}{b}", "{", "\x00{\xff}\x00"} {
		f.Add([]byte(key), shard.DefaultSlotCount)
	}
	f.Add([]byte("123456789"), uint32(1))

	f.Fuzz(func(t *testing.T, key []byte, count uint32) {
		s, err := shard.NewSlotter(count)
		if err != nil {
			if count != 0 {
				t.Fatalf("NewSlotter(%d) failed: %v", count, err)
			}
			return
		}

		original := bytes.Clone(key)
		slot := s.Slot(key)

		if slot >= count {
			t.Fatalf("slot %d out of range for count %d", slot, count)
		}
		if again := s.Slot(key); again != slot {
			t.Fatalf("slot not deterministic: %d then %d", slot, again)
		}
		if !bytes.Equal(key, original) {
			t.Fatal("Slot modified its input")
		}

		input := shard.HashInput(key)
		if !bytes.Contains(key, input) {
			t.Fatalf("hash input %q is not part of key %q", input, key)
		}
		// The brace rule is idempotent: the hash input of a hash input is
		// itself, so a key and its partition key always share a slot.
		if !bytes.Equal(shard.HashInput(input), input) {
			t.Fatalf("hash input of %q is not stable", key)
		}
		if s.Slot(input) != slot {
			t.Fatalf("key %q and its hash input disagree", key)
		}
	})
}
