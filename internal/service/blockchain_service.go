package service

import (
	"context"
	"errors"
	"time"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	metric "github.com/joseferreira/Immutable-Ledger-Service/internal/infra"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/persistence"
	"github.com/sirupsen/logrus"
)

type BlockchainService struct {
	Blockchain  *domain.Blockchain
	Persistence *persistence.BlockchainRepository
	Mempool     *MempoolService
	difficulty  int
	config      *metric.Config
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewBlockchainService(ctx context.Context, persistence *persistence.BlockchainRepository, mempool *MempoolService, config *metric.Config) *BlockchainService {
	bc := domain.NewBlockchain()
	blocks, err := persistence.LoadBlocks()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to load blocks for BlockchainService")
	}
	bc.SetBlocks(blocks)

	childCtx, cancel := context.WithCancel(ctx)
	bs := &BlockchainService{
		Blockchain:  bc,
		Persistence: persistence,
		Mempool:     mempool,
		difficulty:  config.InitialDifficulty,
		config:      config,
		ctx:         childCtx,
		cancel:      cancel,
	}

	if len(bs.Blockchain.Blocks) == 0 {
		genesis := bs.createGenesisBlock()
		if err := bs.Persistence.PersistBlock(genesis); err != nil {
			logrus.WithError(err).Fatal("Failed to persist genesis block")
		}
		bs.Blockchain.AddBlock(genesis)
		logrus.WithFields(logrus.Fields{
			"index": genesis.Index,
			"hash":  genesis.Hash,
		}).Info("Created and stored genesis block")
	}

	return bs
}

func (bs *BlockchainService) createGenesisBlock() *domain.Block {
	tx := &domain.Transaction{Data: "Genesis Transaction"}
	tx.Timestamp = 1
	tx.Hash()
	genesis := domain.NewBlock(0, []*domain.Transaction{tx}, "0", bs.config.InitialDifficulty)
	genesis.Timestamp = 1
	genesis.Mine()
	return genesis
}

func (bs *BlockchainService) Start() {
	logrus.Info("BlockchainService started.")
}

func (bs *BlockchainService) Close() {
	bs.cancel()
	logrus.Info("BlockchainService stopped.")
}

func (bs *BlockchainService) AddBlock() *domain.Block {
	startTime := time.Now()

	txs := bs.Mempool.GetTransactions()

	prevBlock := bs.Blockchain.GetLatestBlock()
	difficulty := bs.getDifficulty()
	newBlock := domain.NewBlock(prevBlock.Index+1, txs, prevBlock.Hash, difficulty)
	newBlock.Mine()

	duration := time.Since(startTime)
	metric.MiningTime.Observe(duration.Seconds())
	metric.TransactionsPerBlock.Observe(float64(len(newBlock.Transactions)))

	if err := bs.Persistence.PersistBlock(newBlock); err != nil {
		logrus.WithError(err).Panic("Failed to save block to database")
	}

	bs.Blockchain.AddBlock(newBlock)
	bs.Mempool.RemoveTransactions(newBlock.Transactions)

	metric.BlockHeight.Inc()

	logrus.WithFields(logrus.Fields{
		"index":      newBlock.Index,
		"hash":       newBlock.Hash,
		"nonce":      newBlock.Nonce,
		"duration":   duration,
		"difficulty": difficulty,
	}).Info("Block mined")

	return newBlock
}

func (bs *BlockchainService) AddBlockFromNetwork(block *domain.Block) (bool, error) {
	latestBlock := bs.Blockchain.GetLatestBlock()

	if block.Index <= latestBlock.Index {
		return false, nil
	}

	if block.PrevBlockHash != latestBlock.Hash {
		return false, errors.New("invalid previous block hash")
	}
	if block.CalculateHash() != block.Hash {
		return false, errors.New("invalid block hash")
	}

	if err := bs.Persistence.PersistBlock(block); err != nil {
		return false, err
	}

	bs.Blockchain.AddBlock(block)
	bs.Mempool.RemoveTransactions(block.Transactions)

	metric.BlockHeight.Inc()
	metric.TransactionsPerBlock.Observe(float64(len(block.Transactions)))

	logrus.WithFields(logrus.Fields{
		"index": block.Index,
		"hash":  block.Hash,
	}).Info("Added block from network")

	return true, nil
}

func (bs *BlockchainService) getDifficulty() int {
	latestBlock := bs.Blockchain.GetLatestBlock()
	if latestBlock.Index%bs.config.DifficultyAdjustmentInterval == 0 && latestBlock.Index != 0 {
		return bs.calculateDifficulty()
	}
	return latestBlock.Difficulty
}

func (bs *BlockchainService) calculateDifficulty() int {
	firstBlock := bs.Blockchain.Blocks[len(bs.Blockchain.Blocks)-int(bs.config.DifficultyAdjustmentInterval)]
	actualTimeTaken := bs.Blockchain.GetLatestBlock().Timestamp - firstBlock.Timestamp
	expectedTimeTaken := int64(bs.config.BlockGenerationInterval) * bs.config.DifficultyAdjustmentInterval

	if actualTimeTaken < expectedTimeTaken/2 {
		return bs.difficulty + 1
	} else if actualTimeTaken > expectedTimeTaken*2 {
		newDifficulty := bs.difficulty - 1
		if newDifficulty < 1 {
			return 1
		}
		return newDifficulty
	}
	return bs.difficulty
}

func (bs *BlockchainService) VerifyChain() bool {
	return bs.Blockchain.VerifyChain()
}

func (bs *BlockchainService) GetBlockByIndex(index int64) *domain.Block {
	bs.Blockchain.Mu.Lock()
	defer bs.Blockchain.Mu.Unlock()
	if index < 0 || int(index) >= len(bs.Blockchain.Blocks) {
		return nil
	}
	return bs.Blockchain.Blocks[index]
}
