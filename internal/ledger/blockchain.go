package ledger

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

const (
	blocksBucket = "blocks"
)

type Blockchain struct {
	Blocks     []*Block
	Mempool    []*Transaction
	Mu         sync.Mutex
	db         *bolt.DB
	difficulty int
	config     *Config
}

func getDBPath() string {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "blockchain.db"
	}
	return dbPath
}

func NewBlockchain(config *Config) *Blockchain {
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
			genesis := newBlock(0, []*Transaction{}, "0", config.InitialDifficulty)
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
		difficulty: config.InitialDifficulty,
		config:     config,
	}

	BlockHeight.Set(float64(len(bc.Blocks)))
	MempoolSize.Set(0)

	return bc
}

func (bc *Blockchain) getDifficulty() int {
	latestBlock := bc.Blocks[len(bc.Blocks)-1]
	if latestBlock.Index%bc.config.DifficultyAdjustmentInterval == 0 && latestBlock.Index != 0 {
		return bc.calculateDifficulty()
	}
	return latestBlock.Difficulty
}

func (bc *Blockchain) calculateDifficulty() int {
	firstBlock := bc.Blocks[len(bc.Blocks)-int(bc.config.DifficultyAdjustmentInterval)]
	actualTimeTaken := bc.Blocks[len(bc.Blocks)-1].Timestamp - firstBlock.Timestamp
	expectedTimeTaken := int64(bc.config.BlockGenerationInterval) * bc.config.DifficultyAdjustmentInterval

	if actualTimeTaken < expectedTimeTaken/2 {
		return bc.difficulty + 1
	} else if actualTimeTaken > expectedTimeTaken*2 {
		newDifficulty := bc.difficulty - 1
		if newDifficulty < 1 {
			return 1
		}
		return newDifficulty
	}
	return bc.difficulty
}

func (bc *Blockchain) AddBlock() *Block {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()

	startTime := time.Now()

	prevBlock := bc.Blocks[len(bc.Blocks)-1]
	difficulty := bc.getDifficulty()
	newBlock := newBlock(prevBlock.Index+1, bc.Mempool, prevBlock.Hash, difficulty)
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
		"index":      newBlock.Index,
		"hash":       newBlock.Hash,
		"nonce":      newBlock.Nonce,
		"duration":   duration,
		"difficulty": difficulty,
	}).Info("Block mined")

	return newBlock
}

func (bc *Blockchain) AddTransactionToMempool(tx *Transaction) {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()
	bc.Mempool = append(bc.Mempool, tx)
	MempoolSize.Inc()
	logrus.WithField("transaction", tx.Data).Info("Transaction added to mempool")
}

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

func (bc *Blockchain) Close() {
	logrus.Info("Closing database")
	bc.db.Close()
}
