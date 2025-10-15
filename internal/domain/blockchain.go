package domain

import (
	"sync"
)

// Blockchain manages the chain of blocks.
type Blockchain struct {
	Blocks []*Block
	Mu     sync.Mutex
}

// NewBlockchain creates a new blockchain instance.
func NewBlockchain() *Blockchain {
	return &Blockchain{
		Blocks: []*Block{},
	}
}

// VerifyChain verifies the integrity of the blockchain.
func (bc *Blockchain) VerifyChain() bool {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()
	for i := 1; i < len(bc.Blocks); i++ {
		currentBlock := bc.Blocks[i]
		prevBlock := bc.Blocks[i-1]

		if currentBlock.Hash != currentBlock.CalculateHash() {
			return false
		}

		if currentBlock.PrevBlockHash != prevBlock.Hash {
			return false
		}
	}
	return true
}

func (bc *Blockchain) AddBlock(block *Block) {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()
	bc.Blocks = append(bc.Blocks, block)
}

func (bc *Blockchain) GetLatestBlock() *Block {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()
	if len(bc.Blocks) == 0 {
		return nil
	}
	return bc.Blocks[len(bc.Blocks)-1]
}

func (bc *Blockchain) SetBlocks(blocks []*Block) {
	bc.Mu.Lock()
	defer bc.Mu.Unlock()
	bc.Blocks = blocks
}
