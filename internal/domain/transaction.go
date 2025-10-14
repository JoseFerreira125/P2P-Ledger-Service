package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

type Transaction struct {
	ID        string `json:"id,omitempty"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

func NewTransaction(data string) *Transaction {
	return &Transaction{
		Data:      data,
		Timestamp: time.Now().UnixNano(),
	}
}

func (t *Transaction) Hash() {
	// If the timestamp is not set, set it to the current time to ensure uniqueness.
	if t.Timestamp == 0 {
		t.Timestamp = time.Now().UnixNano()
	}
	dataToHash := fmt.Sprintf("%s%d", t.Data, t.Timestamp)
	hash := sha256.Sum256([]byte(dataToHash))
	t.ID = hex.EncodeToString(hash[:])

	logrus.WithFields(logrus.Fields{
		"data":      t.Data,
		"timestamp": t.Timestamp,
		"id":        t.ID,
	}).Debug("Transaction.Hash(): Calculated transaction ID")
}
