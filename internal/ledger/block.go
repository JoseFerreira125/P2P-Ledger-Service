package ledger

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

func (b *Block) setMerkleRoot() {
	var txData [][]byte
	for _, tx := range b.Transactions {
		txBytes, _ := json.Marshal(tx)
		txData = append(txData, txBytes)
	}
	mTree := newMerkleTree(txData)
	b.MerkleRoot = hex.EncodeToString(mTree.RootNode.Data)
}

func (b *Block) CalculateHash() string {
	timestamp := strconv.FormatInt(b.Timestamp, 10)
	nonce := strconv.FormatInt(b.Nonce, 10)
	headers := bytes.Join(
		[][]byte{
			[]byte(strconv.FormatInt(b.Index, 10)),
			[]byte(timestamp),
			[]byte(b.PrevBlockHash),
			[]byte(b.MerkleRoot),
			[]byte(nonce),
		},
		[]byte{},
	)
	hash := sha256.Sum256(headers)
	return hex.EncodeToString(hash[:])
}

func (b *Block) Mine() {
	target := strings.Repeat("0", b.Difficulty)
	for {
		b.Hash = b.CalculateHash()
		if b.Hash[:b.Difficulty] == target {
			fmt.Printf("Block mined: %s\n", b.Hash)
			break
		}
		b.Nonce++
	}
}
