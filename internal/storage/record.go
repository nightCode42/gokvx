package storage

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

// A command log record, in little-endian byte order. This layout is part of
// the on-disk format and permanent once released (KV-STO-002):
//
//	offset  size  field
//	0       4     length:   byte length of the body that follows the header
//	4       4     checksum: CRC32C (Castagnoli) of the body
//	8       8     index:    the record's log index           ┐ body
//	16      n     command:  the serialized command, n bytes  ┘
const (
	// recordHeaderSize is the size of the length and checksum fields.
	recordHeaderSize = 8
	// recordIndexSize is the size of the index at the start of the body.
	recordIndexSize = 8
	// maxRecordBodySize bounds the length field. It is far above any valid
	// command, whose size is limited by limits.max_request_bytes, and keeps a
	// corrupted length from ever causing an absurd allocation.
	maxRecordBodySize = 64 << 20
	// maxCommandSize is the largest command a record can hold.
	maxCommandSize = maxRecordBodySize - recordIndexSize
)

// crcTable is the CRC32C (Castagnoli) table used for record checksums.
var crcTable = crc32.MakeTable(crc32.Castagnoli)

// The ways a record can fail to decode. A torn record is cut short, as a
// crash during a write leaves it; the others mean the bytes are wrong.
var (
	errTornRecord  = errors.New("record is cut short by the end of the segment")
	errBadLength   = errors.New("record length is out of range")
	errBadChecksum = errors.New("record checksum does not match")
)

// encodeRecord returns the bytes of the record holding command at index.
func encodeRecord(index uint64, command []byte) []byte {
	buf := make([]byte, recordHeaderSize+recordIndexSize+len(command))
	body := buf[recordHeaderSize:]
	binary.LittleEndian.PutUint64(body[:recordIndexSize], index)
	copy(body[recordIndexSize:], command)

	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(body))) //nolint:gosec // len(body) <= maxRecordBodySize, enforced by Append.
	binary.LittleEndian.PutUint32(buf[4:8], crc32.Checksum(body, crcTable))
	return buf
}

// decodeRecord decodes the record at the start of buf. It returns the
// record's index, its command (aliasing buf), and its total size in bytes,
// or one of the decode errors above.
func decodeRecord(buf []byte) (index uint64, command []byte, size int, err error) {
	if len(buf) < recordHeaderSize {
		return 0, nil, 0, errTornRecord
	}
	length := int(binary.LittleEndian.Uint32(buf[0:4]))
	if length < recordIndexSize || length > maxRecordBodySize {
		return 0, nil, 0, errBadLength
	}
	size = recordHeaderSize + length
	if len(buf) < size {
		return 0, nil, 0, errTornRecord
	}
	body := buf[recordHeaderSize:size]
	if crc32.Checksum(body, crcTable) != binary.LittleEndian.Uint32(buf[4:8]) {
		return 0, nil, 0, errBadChecksum
	}
	return binary.LittleEndian.Uint64(body[:recordIndexSize]), body[recordIndexSize:], size, nil
}

// hasLaterRecord reports whether tail — the bytes following a damaged record
// whose index would have been damaged — contains an intact record with a
// higher index at any offset. If it does, the damage is corruption in the
// middle of the log rather than a torn tail (KV-STO-004). Searching every
// offset keeps that true even when the damage hit a length field, which would
// otherwise hide where the next record starts.
func hasLaterRecord(tail []byte, damaged uint64) bool {
	for p := 1; p+recordHeaderSize+recordIndexSize <= len(tail); p++ {
		// The index field sits right after the header; checking it first
		// avoids computing a checksum at almost every offset.
		index := binary.LittleEndian.Uint64(tail[p+recordHeaderSize:])
		if index <= damaged || index-damaged > uint64(len(tail)) {
			continue
		}
		if got, _, _, err := decodeRecord(tail[p:]); err == nil && got == index {
			return true
		}
	}
	return false
}
