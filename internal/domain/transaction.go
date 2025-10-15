package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type Transaction struct {
	ID        string `json:"id,omitempty"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

func NewTransaction(data string) *Transaction {
	tx := &Transaction{
		Data:      data,
		Timestamp: time.Now().UnixNano(),
	}
	tx.Hash()
	return tx
}

func (t *Transaction) Hash() {
	dataToHash := fmt.Sprintf("%s%d", t.Data, t.Timestamp)
	hash := sha256.Sum256([]byte(dataToHash))
	t.ID = hex.EncodeToString(hash[:])
}
