// Copyright 2022 Alim Zanibekov
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioelements

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

type ElementType uint8

const (
	AVLTypeSigned ElementType = iota
	AVLTypeUnsigned
	AVLTypeHEX
	AVLTypeASCII
)

type Info struct {
	Id          uint16      `json:"id"`
	Name        string      `json:"name"`
	Bytes       int         `json:"bytes"`
	Type        ElementType `json:"type"`
	Min         float64     `json:"min"`
	Max         float64     `json:"max"`
	Multiplier  float64     `json:"multiplier"`
	Units       string      `json:"units"`
	Description string      `json:"description"`
	Support     string      `json:"support"`
	Group       string      `json:"group"`
}

type Parsed struct {
	Id    uint16      `json:"id,omitempty"`
	Value interface{} `json:"value,omitempty"`
	Units string      `json:"units,omitempty"`
	Name  string      `json:"name,omitempty"`
}

type Parser struct {
	elements []Info
}

var defaultParser = &Parser{elements}

// NewParser create new Parser
func NewParser(ioElements []Info) *Parser {
	return &Parser{elements: ioElements}
}

// DefaultParser returns default parser with IO Element info represented in `ioelements_dump.go` file
func DefaultParser() *Parser {
	return defaultParser
}

// GetElementInfo returns full description of IO Element by its id
func (r *Parser) GetElementInfo(id uint16) (Info, error) {
	for _, e := range r.elements {
		if e.Id == id {
			return e, nil
		}
	}

	return Info{}, fmt.Errorf("element with id %v not found", id)
}

// Parse parses IO Element (result can be represented in numan-readable format)
func (r *Parser) Parse(id uint16, buffer []byte) (*Parsed, error) {
	info, err := r.GetElementInfo(id)
	if err != nil {
		return nil, err
	}

	var res interface{}

	size := len(buffer)
	if size == 1 && info.Min == 0 && info.Max == 1 && info.Type == AVLTypeUnsigned {
		res = buffer[0] == 1
	} else if (size == 1 || size == 2 || size == 4 || size == 8) && (info.Type == AVLTypeUnsigned || info.Type == AVLTypeSigned) {
		buf := make([]byte, 8)
		copy(buf[8-size:], buffer)

		if info.Type == AVLTypeUnsigned {
			v := binary.BigEndian.Uint64(buf)
			if v >= uint64(info.Min) && v <= uint64(info.Max) {
				if info.Multiplier != 0 {
					res = float64(v) * info.Multiplier
				} else {
					res = v
				}
			} else {
				return nil, fmt.Errorf("buffer %v out of range [%v, %v]", v, uint64(info.Min), uint64(info.Max))
			}
		} else if info.Type == AVLTypeSigned {
			if (buffer[0] >> 7) == 1 {
				buf[8-size] &= 0x7F
				buf[0] |= 0x80
			}
			v := int64(binary.BigEndian.Uint64(buf))
			if v >= int64(info.Min) && v <= int64(info.Max) {
				if info.Multiplier != 0 {
					res = float64(v) * info.Multiplier
				} else {
					res = v
				}
			} else {
				return nil, fmt.Errorf("buffer %v out of range [%v, %v]", v, info.Min, info.Max)
			}
		}
	} else if info.Type == AVLTypeHEX {
		res = hex.EncodeToString(buffer)
	} else if info.Type == AVLTypeASCII {
		res = string(buffer)
	}

	if res == nil {
		return nil, fmt.Errorf("unable to proceed io element with id %v for buffer '%s'", info.Id, hex.EncodeToString(buffer))
	}

	return &Parsed{
		id, res, info.Units, info.Name,
	}, nil
}
