// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package teltonika

import (
	"encoding/binary"
	"io"
)

func Crc16IBM(data []byte) uint16 {
	crc := uint16(0)
	size := len(data)
	for i := 0; i < size; i++ {
		crc ^= uint16(data[i])
		for j := 0; j < 8; j++ {
			if (crc & 0x0001) == 1 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

type byteReader struct {
	pos         int
	size        int
	input       []byte
	allocOnRead bool
}

func newByteReader(input []byte, allocOnRead bool) *byteReader {
	return &byteReader{input: input, size: len(input), allocOnRead: allocOnRead}
}

func (r *byteReader) ReadBytes(n int) ([]byte, error) {
	if r.pos+n > r.size {
		return nil, io.EOF
	}
	r.pos += n

	if r.allocOnRead {
		buf := make([]byte, n)
		copy(buf, r.input[r.pos-n:r.pos])

		return buf, nil
	}

	return r.input[r.pos-n : r.pos], nil
}

func (r *byteReader) ReadUInt8BE() (uint8, error) {
	if r.pos+1 > r.size {
		return 0, io.EOF
	}
	r.pos += 1
	return r.input[r.pos-1], nil
}

func (r *byteReader) ReadInt8BE() (int8, error) {
	if r.pos+1 > r.size {
		return 0, io.EOF
	}
	r.pos += 1
	return int8(r.input[r.pos-1]), nil
}

func (r *byteReader) ReadUInt16BE() (uint16, error) {
	if r.pos+2 > r.size {
		return 0, io.EOF
	}
	r.pos += 2
	return binary.BigEndian.Uint16(r.input[r.pos-2:]), nil
}

func (r *byteReader) ReadInt16BE() (int16, error) {
	if r.pos+2 > r.size {
		return 0, io.EOF
	}
	r.pos += 2
	return int16(binary.BigEndian.Uint16(r.input[r.pos-2:])), nil
}

func (r *byteReader) ReadUInt32BE() (uint32, error) {
	if r.pos+4 > r.size {
		return 0, io.EOF
	}
	r.pos += 4
	return binary.BigEndian.Uint32(r.input[r.pos-4:]), nil
}

func (r *byteReader) ReadInt32BE() (int32, error) {
	if r.pos+4 > r.size {
		return 0, io.EOF
	}
	r.pos += 4
	return int32(binary.BigEndian.Uint32(r.input[r.pos-4:])), nil
}

func (r *byteReader) ReadUInt64BE() (uint64, error) {
	if r.pos+8 > r.size {
		return 0, io.EOF
	}
	r.pos += 8
	return binary.BigEndian.Uint64(r.input[r.pos-8:]), nil
}

func (r *byteReader) ReadInt64BE() (int64, error) {
	if r.pos+8 > r.size {
		return 0, io.EOF
	}
	r.pos += 8
	return int64(binary.BigEndian.Uint64(r.input[r.pos-8:])), nil
}
