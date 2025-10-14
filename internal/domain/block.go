package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Block struct {
	Index         int64          `json:"index"`
	Timestamp     int64          `json:"timestamp"`
	Transactions  []*Transaction `json:"transactions"`
	Nonce         int64          `json:"nonce"`
	PrevBlockHash string         `json:"prevBlockHash"`
	Hash          string         `json:"hash"`
	Difficulty    int            `json:"difficulty"`
	MerkleRoot    string         `json:"merkleRoot"`
}

func newBlock(index int64, transactions []*Transaction, prevBlockHash string, difficulty int) *Block {
	block := &Block{
		Index:         index,
		Timestamp:     time.Now().Unix(),
		Transactions:  transactions,
		PrevBlockHash: prevBlockHash,
		Difficulty:    difficulty,
		Nonce:         0,
	}
	block.setMerkleRoot()
	return block
}

func (block *Block) setMerkleRoot() {
	var txData [][]byte
	for _, transaction := range block.Transactions {
		txBytes, _ := json.Marshal(transaction)
		txData = append(txData, txBytes)
	}
	mTree := newMerkleTree(txData)
	block.MerkleRoot = hex.EncodeToString(mTree.RootNode.Data)
}

func (block *Block) calculateHash() string {
	timestamp := strconv.FormatInt(block.Timestamp, 10)
	nonce := strconv.FormatInt(block.Nonce, 10)
	headers := bytes.Join(
		[][]byte{
			[]byte(strconv.FormatInt(block.Index, 10)),
			[]byte(timestamp),
			[]byte(block.PrevBlockHash),
			[]byte(block.MerkleRoot),
			[]byte(nonce),
		},
		[]byte{},
	)
	hash := sha256.Sum256(headers)
	return hex.EncodeToString(hash[:])
}

func (block *Block) mine() {
	target := strings.Repeat("0", block.Difficulty)
	for {
		block.Hash = block.calculateHash()
		if block.Hash[:block.Difficulty] == target {
			fmt.Printf("Block mined: %s\n", block.Hash)
			break
		}
		block.Nonce++
	}
}
