package domain

import (
	"bytes"
	"encoding/gob"
)

type MessageType byte

const (
	MessageTypeTx       MessageType = 0x1
	MessageTypeBlock    MessageType = 0x2
	MessageTypeGetPeers MessageType = 0x3
	MessageTypePeers    MessageType = 0x4
)

type Message struct {
	Type    MessageType
	Payload []byte
}

type PeersPayload struct {
	Peers []string
}

func (m *Message) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(m)
	return buf.Bytes(), err
}
