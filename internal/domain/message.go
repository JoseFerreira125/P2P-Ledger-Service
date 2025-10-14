package domain

import (
	"bytes"
	"encoding/gob"
)

type MessageType byte

const (
	MessageTypeTx        MessageType = 0x1
	MessageTypeBlock     MessageType = 0x2
	MessageTypeGetStatus MessageType = 0x3
	MessageTypeStatus    MessageType = 0x4
)

type Message struct {
	Type    MessageType
	Payload []byte
}

type StatusPayload struct {
	CurrentHeight     uint32
	PeerAddresses     []string
	SenderListenAddress string
}

func (m *Message) ToBytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(m)
	return buf.Bytes(), err
}
