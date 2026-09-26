package storage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These white-box tests cover the record codec directly: its layout is part
// of the on-disk format.

// TestRecordLayout_KV_STO_002 pins the exact bytes of one record, so any
// change to the on-disk layout fails here first.
// Verifies: KV-STO-002.
func TestRecordLayout_KV_STO_002(t *testing.T) {
	t.Parallel()

	got := encodeRecord(1, []byte("put"))

	// The checksum was computed independently of this package, with a
	// bitwise CRC32C (reflected polynomial 0x82F63B78) validated against the
	// standard check value CRC32C("123456789") = 0xE3069283.
	want := []byte{
		0x0b, 0x00, 0x00, 0x00, // length: 8-byte index + 3-byte command
		0x40, 0xa9, 0x50, 0x3f, // CRC32C of the body, 0x3F50A940

		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // index 1
		'p', 'u', 't', // command
	}
	assert.Equal(t, want, got)
}

// TestRecordRoundTrip checks that decoding returns what was encoded, for
// empty and non-empty commands.
func TestRecordRoundTrip(t *testing.T) {
	t.Parallel()

	for _, cmd := range [][]byte{nil, []byte("x"), bytes.Repeat([]byte{0xFF}, 300)} {
		rec := encodeRecord(42, cmd)

		index, got, size, err := decodeRecord(append(rec, "trailing"...))

		require.NoError(t, err)
		assert.Equal(t, uint64(42), index)
		assert.Equal(t, len(cmd), len(got))
		assert.True(t, bytes.Equal(cmd, got))
		assert.Equal(t, len(rec), size)
	}
}

// TestDecodeRecordErrors checks how each kind of damage is classified.
func TestDecodeRecordErrors(t *testing.T) {
	t.Parallel()

	rec := encodeRecord(7, []byte("command"))
	flipped := bytes.Clone(rec)
	flipped[len(flipped)-1] ^= 0x01

	tests := []struct {
		name string
		buf  []byte
		want error
	}{
		{"empty", nil, errTornRecord},
		{"partial header", rec[:5], errTornRecord},
		{"partial body", rec[:len(rec)-1], errTornRecord},
		{"length below index size", []byte{4, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4}, errBadLength},
		{"length above limit", []byte{0xff, 0xff, 0xff, 0x7f, 0, 0, 0, 0}, errBadLength},
		{"flipped bit", flipped, errBadChecksum},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, err := decodeRecord(tt.buf)

			assert.ErrorIs(t, err, tt.want)
		})
	}
}

// TestHasLaterRecord checks the search that tells a torn tail from damage in
// the middle of the log.
func TestHasLaterRecord(t *testing.T) {
	t.Parallel()

	later := encodeRecord(6, []byte("intact"))
	stale := encodeRecord(4, []byte("older"))

	assert.True(t, hasLaterRecord(append([]byte("garbage"), later...), 5), "an intact later record")
	assert.False(t, hasLaterRecord([]byte("garbage that is not a record"), 5), "only garbage")
	assert.False(t, hasLaterRecord(append([]byte("x"), stale...), 5), "only an older index")
	assert.False(t, hasLaterRecord(nil, 5), "nothing")
}

// FuzzDecodeRecord feeds arbitrary bytes to the decoder: it must never panic,
// and whatever it accepts must re-encode to exactly the bytes it read.
// Verifies: QA-005, KV-STO-004.
func FuzzDecodeRecord(f *testing.F) {
	f.Add(encodeRecord(1, []byte("put")))
	f.Add(encodeRecord(99, nil))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, buf []byte) {
		index, cmd, size, err := decodeRecord(buf)
		if err != nil {
			return
		}
		if size > len(buf) {
			t.Fatalf("size %d beyond buffer of %d", size, len(buf))
		}
		if !bytes.Equal(encodeRecord(index, cmd), buf[:size]) {
			t.Fatal("accepted bytes do not re-encode identically")
		}
	})
}
