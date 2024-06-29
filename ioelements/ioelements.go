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
	IOElementSigned ElementType = iota
	IOElementUnsigned
	IOElementHEX
	IOElementASCII
)

type IOElementDefinition struct {
	Id              uint16      `json:"id"`
	Name            string      `json:"name"`
	NumBytes        int         `json:"numBytes"`
	Type            ElementType `json:"type"`
	Min             float64     `json:"min"`
	Max             float64     `json:"max"`
	Multiplier      float64     `json:"multiplier"`
	Units           string      `json:"units"`
	Description     string      `json:"description"`
	SupportedModels []string    `json:"supportedModels"`
	Groups          []string    `json:"groups"`
}

type IOElement struct {
	Id         uint16               `json:"id,omitempty"`
	Value      interface{}          `json:"value,omitempty"`
	Definition *IOElementDefinition `json:"definition,omitempty"`
}

type Parser struct {
	definitions []IOElementDefinition
}

var defaultParser = &Parser{ioElementDefinitions}

func (r *IOElement) String() string {
	switch r.Value.(type) {
	case float64:
		return fmt.Sprintf("%s: %.3f%s", r.Definition.Name, r.Value, r.Definition.Units)
	default:
		return fmt.Sprintf("%s: %v%s", r.Definition.Name, r.Value, r.Definition.Units)
	}
}

// NewParser create new Parser
func NewParser(definitions []IOElementDefinition) *Parser {
	return &Parser{definitions}
}

// DefaultParser returns default parser with IO Element definitions represented in `ioelements_dump.go` file
func DefaultParser() *Parser {
	return defaultParser
}

// GetElementInfo returns full description of IO Element by its id and model name
// If you don't know the model name, you can skip the model name check by passing '*' as the model name
func (r *Parser) GetElementInfo(modelName string, id uint16) (*IOElementDefinition, error) {
	for _, e := range r.definitions {
		if e.Id != id {
			continue
		}
		if modelName == "*" {
			return &e, nil
		}

		for _, v := range e.SupportedModels {
			if v != modelName {
				continue
			}
			return &e, nil
		}
	}

	return nil, fmt.Errorf("element with id %v not found", id)
}

// Parse parses IO Element (result can be represented in numan-readable format)
// If you don't know the model name, you can skip the model name check by passing '*' as the model name
func (r *Parser) Parse(modelName string, id uint16, buffer []byte) (*IOElement, error) {
	info, err := r.GetElementInfo(modelName, id)
	if err != nil {
		return nil, err
	}

	var res interface{}

	size := len(buffer)
	if size == 1 && info.Min == 0 && info.Max == 1 && info.Type == IOElementUnsigned {
		res = buffer[0] == 1
	} else if (size == 1 || size == 2 || size == 4 || size == 8) && (info.Type == IOElementUnsigned || info.Type == IOElementSigned) {
		buf := make([]byte, 8)
		copy(buf[8-size:], buffer)

		if info.Type == IOElementUnsigned {
			v := binary.BigEndian.Uint64(buf)
			if info.Multiplier != 1.0 {
				res = float64(v) * info.Multiplier
			} else {
				res = v
			}
		} else if info.Type == IOElementSigned {
			if (buffer[0] >> 7) == 1 {
				buf[8-size] &= 0x7F
				buf[0] |= 0x80
			}
			v := int64(binary.BigEndian.Uint64(buf))
			if info.Multiplier != 1.0 {
				res = float64(v) * info.Multiplier
			} else {
				res = v
			}
		}
	} else if info.Type == IOElementHEX {
		res = hex.EncodeToString(buffer)
	} else if info.Type == IOElementASCII {
		res = string(buffer)
	}

	if res == nil {
		return nil, fmt.Errorf("unable to proceed io element with id %v for buffer '%s'", info.Id, hex.EncodeToString(buffer))
	}

	return &IOElement{
		id, res, info,
	}, nil
}
