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
	input  io.Reader
	buffer []byte
}

func newByteReader(input io.Reader) *byteReader {
	return &byteReader{input: input, buffer: make([]byte, 8)}
}

func (r *byteReader) ReadBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	read, err := r.input.Read(buf)
	if err != nil {
		return nil, err
	}

	if n != read {
		return nil, io.EOF
	}

	return buf, nil
}

func (r *byteReader) ReadUInt8BE() (uint8, error) {
	b, err := r.readInternal(1)
	if err != nil {
		return 0, err
	}
	return b[0], err
}

func (r *byteReader) ReadInt8BE() (int8, error) {
	b, err := r.readInternal(1)
	if err != nil {
		return 0, err
	}
	return int8(b[0]), err
}

func (r *byteReader) ReadUInt16BE() (uint16, error) {
	b, err := r.readInternal(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), err
}

func (r *byteReader) ReadInt16BE() (int16, error) {
	b, err := r.readInternal(2)
	if err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(b)), err
}

func (r *byteReader) ReadUInt32BE() (uint32, error) {
	b, err := r.readInternal(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), err
}

func (r *byteReader) ReadInt32BE() (int32, error) {
	b, err := r.readInternal(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b)), err
}

func (r *byteReader) ReadUInt64BE() (uint64, error) {
	b, err := r.readInternal(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), err
}

func (r *byteReader) ReadInt64BE() (int64, error) {
	b, err := r.readInternal(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), err
}

func (r *byteReader) readInternal(n int) ([]byte, error) {
	read, err := r.input.Read(r.buffer[:n])
	if err != nil {
		return nil, err
	}

	if n != read {
		return nil, io.EOF
	}

	return r.buffer, nil
}
