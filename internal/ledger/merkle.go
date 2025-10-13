package ledger

import (
	"crypto/sha256"
)

// MerkleTree represents a Merkle tree.
type MerkleTree struct {
	RootNode *MerkleNode
}

// MerkleNode represents a node in the Merkle tree.
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  []byte
}

// NewMerkleNode creates a new Merkle tree node.
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	if left == nil && right == nil {
		// Leaf node
		hash := sha256.Sum256(data)
		node.Data = hash[:]
	} else {
		// Internal node
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:]
	}

	node.Left = left
	node.Right = right

	return &node
}

// NewMerkleTree creates a new Merkle tree from a sequence of data.
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []MerkleNode

	if len(data) == 0 {
        hash := sha256.Sum256([]byte{})
        rootNode := MerkleNode{Data: hash[:]}
        return &MerkleTree{RootNode: &rootNode}
    }

	// Create leaf nodes
	for _, datum := range data {
		node := NewMerkleNode(nil, nil, datum)
		nodes = append(nodes, *node)
	}

	// Build the tree
	for len(nodes) > 1 {
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		var newLevel []MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(&nodes[i], &nodes[i+1], nil)
			newLevel = append(newLevel, *node)
		}
		nodes = newLevel
	}

	return &MerkleTree{&nodes[0]}
}
