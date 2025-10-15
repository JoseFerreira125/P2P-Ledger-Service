package persistence

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

const (
	blocksBucket = "blocks"
)

type BlockchainRepository struct {
	db *bolt.DB
}

func NewPersistenceService() *BlockchainRepository {
	dbPath := getDBPath()
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		logrus.WithError(err).WithField("db_path", dbPath).Fatal("Failed to open database")
	}

	logrus.WithField("db_path", dbPath).Info("Database opened")

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(blocksBucket))
		if err != nil {
			return fmt.Errorf("failed to create blocks bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		logrus.WithError(err).Fatal("Failed to ensure blocks bucket exists")
	}

	return &BlockchainRepository{
		db: db,
	}
}

func getDBPath() string {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "blockchain.db"
	}
	return dbPath
}

func (ps *BlockchainRepository) LoadBlocks() ([]*domain.Block, error) {
	var blocks []*domain.Block

	err := ps.db.View(func(boltTx *bolt.Tx) error {
		bucket := boltTx.Bucket([]byte(blocksBucket))
		if bucket == nil {
			return fmt.Errorf("blocks bucket not found")
		}

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var block domain.Block
			if err := json.Unmarshal(v, &block); err == nil {
				blocks = append(blocks, &block)
			} else {
				logrus.WithError(err).Warn("Failed to unmarshal block from database")
			}
		}
		logrus.Infof("Loaded %d blocks from the database", len(blocks))

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to load blocks from database: %w", err)
	}

	return blocks, nil
}

func (ps *BlockchainRepository) PersistBlock(block *domain.Block) error {
	return ps.db.Update(func(boltTx *bolt.Tx) error {
		bucket := boltTx.Bucket([]byte(blocksBucket))
		if bucket == nil {
			return fmt.Errorf("blocks bucket not found")
		}
		encoded, err := json.Marshal(block)
		if err != nil {
			return fmt.Errorf("failed to marshal block: %w", err)
		}
		return bucket.Put([]byte(block.Hash), encoded)
	})
}

func (ps *BlockchainRepository) Close() {
	logrus.Info("Closing database")
	if err := ps.db.Close(); err != nil {
		logrus.WithError(err).Error("Failed to close database")
	}
}
