package service

import (
	"sync"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	metric "github.com/joseferreira/Immutable-Ledger-Service/internal/infra"
	"github.com/sirupsen/logrus"
)

// MempoolService manages the transaction mempool.
type MempoolService struct {
	Mempool map[string]*domain.Transaction
	Mu      sync.Mutex
}

// NewMempoolService creates a new MempoolService instance.
func NewMempoolService() *MempoolService {
	return &MempoolService{
		Mempool: make(map[string]*domain.Transaction),
	}
}

// AddTransaction adds a transaction to the mempool if it doesn't already exist.
// It returns true if the transaction was added, false otherwise.
func (ms *MempoolService) AddTransaction(transaction *domain.Transaction) (bool, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if transaction.ID == "" {
		transaction.Hash()
	}

	if _, exists := ms.Mempool[transaction.ID]; exists {
		logrus.WithField("transaction_id", transaction.ID).Debug("Transaction already exists in mempool, skipping")
		return false, nil
	}

	ms.Mempool[transaction.ID] = transaction
	metric.MempoolSize.Inc()

	logrus.WithField("transaction_id", transaction.ID).Info("Transaction added to mempool")
	return true, nil
}

// GetTransactions returns all transactions currently in the mempool.
func (ms *MempoolService) GetTransactions() []*domain.Transaction {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	txs := make([]*domain.Transaction, 0, len(ms.Mempool))
	for _, tx := range ms.Mempool {
		txs = append(txs, tx)
	}
	return txs
}

// RemoveTransactions removes a list of transactions from the mempool.
func (ms *MempoolService) RemoveTransactions(blockTxs []*domain.Transaction) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	for _, tx := range blockTxs {
		if _, exists := ms.Mempool[tx.ID]; exists {
			delete(ms.Mempool, tx.ID)
			metric.MempoolSize.Dec()
		}
	}
}
