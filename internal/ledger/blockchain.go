package ledger

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"github.com/sirupsen/logrus"
)

const (
	blocksBucket = "blocks"
)

// Blockchain represents the blockchain.
type Blockchain struct {
	Blocks     []*Block
	Mempool    []*Transaction
	Mu         sync.Mutex
	db         *bolt.DB
	difficulty int
}

// getDBPath returns the database path from the environment or a default value.
func getDBPath() string {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "blockchain.db"
	}
	return dbPath
}

// NewBlockchain creates a new blockchain with a genesis block or loads from the DB.
func NewBlockchain(difficulty int) *Blockchain {
	dbPath := getDBPath()
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		logrus.WithError(err).WithField("db_path", dbPath).Fatal("Failed to open database")
	}

	logrus.WithField("db_path", dbPath).Info("Database opened")

	var blocks []*Block

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))

		if b == nil {
			genesis := NewBlock(0, []*Transaction{}, "0", difficulty)
			genesis.Mine()

			b, err := tx.CreateBucket([]byte(blocksBucket))
			if err != nil {
				return err
			}

			encoded, err := json.Marshal(genesis)
			if err != nil {
				return err
			}

			err = b.Put([]byte(genesis.Hash), encoded)
			if err != nil {
				return err
			}

			blocks = append(blocks, genesis)
			logrus.WithFields(logrus.Fields{
				"index": genesis.Index,
				"hash":  genesis.Hash,
			}).Info("Created genesis block")
		} else {
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var block Block
				if err := json.Unmarshal(v, &block); err == nil {
					blocks = append(blocks, &block)
				}
			}
			logrus.Infof("Loaded %d blocks from the database", len(blocks))
		}

		return nil
	})

	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize blockchain")
	}

	bc := &Blockchain{
		Blocks:     blocks,
		Mempool:    []*Transaction{},
		db:         db,
		difficulty: difficulty,
	}

	BlockHeight.Set(float64(len(bc.Blocks)))
	MempoolSize.Set(0)

	return bc
}

// AddBlock adds a new block to the blockchain and persists it.
func (bc *Blockchain) AddBlock() *Block {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()

	startTime := time.Now()

	prevBlock := bc.Blocks[len(bc.Blocks)-1]
	newBlock := NewBlock(prevBlock.Index+1, bc.Mempool, prevBlock.Hash, bc.difficulty)
	newBlock.Mine()

	duration := time.Since(startTime)
	MiningTime.Observe(duration.Seconds())

	err := bc.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		encoded, err := json.Marshal(newBlock)
		if err != nil {
			return err
		}
		return b.Put([]byte(newBlock.Hash), encoded)
	})

	if err != nil {
		logrus.WithError(err).Panic("Failed to save block to database")
	}

	bc.Blocks = append(bc.Blocks, newBlock)
	bc.Mempool = []*Transaction{}

	BlockHeight.Inc()
	MempoolSize.Set(0)

	logrus.WithFields(logrus.Fields{
		"index":    newBlock.Index,
		"hash":     newBlock.Hash,
		"nonce":    newBlock.Nonce,
		"duration": duration,
	}).Info("Block mined")

	return newBlock
}

// AddTransactionToMempool adds a transaction to the mempool.
func (bc *Blockchain) AddTransactionToMempool(tx *Transaction) {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()
	bc.Mempool = append(bc.Mempool, tx)
	MempoolSize.Inc()
	logrus.WithField("transaction", tx.Data).Info("Transaction added to mempool")
}

// VerifyChain verifies the integrity of the blockchain.
func (bc *Blockchain) VerifyChain() bool {
	for i := 1; i < len(bc.Blocks); i++ {
		currentBlock := bc.Blocks[i]
		prevBlock := bc.Blocks[i-1]

		if currentBlock.Hash != currentBlock.CalculateHash() {
			logrus.WithFields(logrus.Fields{
				"block_index":   currentBlock.Index,
				"expected_hash": currentBlock.CalculateHash(),
				"actual_hash":   currentBlock.Hash,
			}).Warn("Chain verification failed: block hash mismatch")
			return false
		}

		if currentBlock.PrevBlockHash != prevBlock.Hash {
			logrus.WithFields(logrus.Fields{
				"block_index":          currentBlock.Index,
				"expected_prev_hash": prevBlock.Hash,
				"actual_prev_hash":   currentBlock.PrevBlockHash,
			}).Warn("Chain verification failed: previous block hash mismatch")
			return false
		}
	}
	logrus.Info("Chain verification successful")
	return true
}

// Close closes the database connection.
func (bc *Blockchain) Close() {
	logrus.Info("Closing database")
	bc.db.Close()
}
