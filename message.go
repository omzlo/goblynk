package blynk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
)

type Header struct {
	Command byte
	Id      uint16
	Length  uint16
}

type Body struct {
	Content []string
}

type Message struct {
	Header Header
	Body   Body
}

/* HEADER */

func (h *Header) Set(command byte, id uint16, len uint16) {
	h.Command = command
	h.Id = id
	h.Length = len
}

func (h *Header) MarshalBinary() ([]byte, error) {
	data := make([]byte, 5, 5)

	data[0] = h.Command
	binary.BigEndian.PutUint16(data[1:3], h.Id)
	binary.BigEndian.PutUint16(data[3:5], h.Length)

	return data, nil
}

func (h *Header) UnmarshalBinary(data []byte) error {
	if len(data) < 5 {
		return errors.New("message header must be at least 5 bytes")
	}

	h.Command = data[0]
	h.Id = binary.BigEndian.Uint16(data[1:3])
	h.Length = binary.BigEndian.Uint16(data[3:5])
	return nil
}

func (h Header) String() string {
	return fmt.Sprintf("{cmd=%d id=%d len=%d}", h.Command, h.Id, h.Length)
}

/* BODY */

func (b *Body) Clear() {
	b.Content = nil
}

func (b *Body) MarshalBinary() ([]byte, error) {
	var data []byte

	if len(b.Content) > 0 {
		data = append(data, []byte(b.Content[0])...)
		for i := 1; i < len(b.Content); i++ {
			data = append(data, 0)
			data = append(data, []byte(b.Content[i])...)
		}
	}
	/* check that length matches reality */
	return data, nil
}

func (b *Body) UnmarshalBinary(data []byte) error {
	start := 0
	for start < len(data) {
		end := start + 1
		for end < len(data) && data[end] != 0 {
			end++
		}
		b.PushBytes(data[start:end])
		start = end + 1
	}
	return nil
}

func (b *Body) PushByte(bt byte) *Body {
	return b.PushString(string(bt))
}

func (b *Body) PushBytes(bt []byte) *Body {
	return b.PushString(string(bt))
}

func (b *Body) PushString(s string) *Body {
	b.Content = append(b.Content, s)
	return b
}

func (b *Body) PushInt(i int) *Body {
	return b.PushString(strconv.Itoa(i))
}

func (b *Body) PushFloat(f float64, bitsize int) *Body {
	return b.PushString(strconv.FormatFloat(f, 'g', -1, bitsize))
}

func (b *Body) AsByte(index int) (byte, bool) {
	if pop, ok := b.AsBytes(index); ok {
		if len(pop) == 1 {
			return pop[0], true
		}
	}
	return 0, false
}

func (b *Body) Shift(count int) {
	if count > len(b.Content) {
		count = len(b.Content)
	}
	b.Content = b.Content[count:]
}

func (b *Body) AsBytes(index int) ([]byte, bool) {
	if pop, ok := b.AsString(index); ok {
		return []byte(pop), true
	}
	return nil, false
}

func (b *Body) AsString(index int) (string, bool) {
	if index >= len(b.Content) || index < 0 {
		return "", false
	}
	return b.Content[index], true
}

func (b *Body) AsInt(index int) (int, bool) {
	if pop, ok := b.AsString(index); ok {
		if i, err := strconv.ParseInt(pop, 10, 32); err == nil {
			return int(i), true
		}
	}
	return 0, false
}

func (b *Body) String() string {
	r := ""
	if len(b.Content) > 0 {
		r = strconv.QuoteToASCII(b.Content[0])
		for i := 1; i < len(b.Content); i++ {
			r += "," + strconv.QuoteToASCII(b.Content[i])
		}
	}
	return "{" + r + "}"
}

/* MESSAGE (HEADER + BODY) */

func (m *Message) Build(command byte) *Body {
	m.Header.Set(command, 0, 0)
	m.Body.Clear()
	return &m.Body
}

func (m *Message) MarshalBinary() ([]byte, error) {
	b, err := m.Body.MarshalBinary()
	if err != nil {
		return nil, err
	}
	m.Header.Length = uint16(len(b))
	h, err := m.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(h, b...), nil
}

func (m *Message) UnmarshalBinary(data []byte) error {
	err := m.Header.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	if len(data)-5 < int(m.Header.Length) {
		return errors.New("content length mismatches header")
	}
	m.Body.Clear()
	return m.Body.UnmarshalBinary(data[5 : m.Header.Length+5])
}

func (m Message) String() string {
	return m.Header.String() + ":" + m.Body.String()
}
