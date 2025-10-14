package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

const (
	blocksBucket = "blocks"
)

type Blockchain struct {
	Blocks     []*Block
	Mempool    map[string]*Transaction
	Mu         sync.Mutex
	Node       *Node
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

func createGenesisBlock() *Block {
	tx := &Transaction{Data: "Genesis Transaction"}
	tx.Hash()
	genesis := newBlock(0, []*Transaction{tx}, "0", 4)
	genesis.mine()
	return genesis
}

func NewBlockchain(config *Config) *Blockchain {
	dbPath := getDBPath()
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		logrus.WithError(err).WithField("db_path", dbPath).Fatal("Failed to open database")
	}

	logrus.WithField("db_path", dbPath).Info("Database opened")

	var blocks []*Block

	err = db.Update(func(boltTx *bolt.Tx) error {
		bucket := boltTx.Bucket([]byte(blocksBucket))

		if bucket == nil {
			genesis := createGenesisBlock()

			newBucket, err := boltTx.CreateBucket([]byte(blocksBucket))
			if err != nil {
				return err
			}

			encoded, err := json.Marshal(genesis)
			if err != nil {
				return err
			}

			err = newBucket.Put([]byte(genesis.Hash), encoded)
			if err != nil {
				return err
			}

			blocks = append(blocks, genesis)
			logrus.WithFields(logrus.Fields{
				"index": genesis.Index,
				"hash":  genesis.Hash,
			}).Info("Created and stored genesis block")
		} else {
			c := bucket.Cursor()
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

	blockchain := &Blockchain{
		Blocks:     blocks,
		Mempool:    make(map[string]*Transaction),
		db:         db,
		difficulty: config.InitialDifficulty,
		config:     config,
	}

	BlockHeight.Set(float64(len(blockchain.Blocks)))
	MempoolSize.Set(0)

	return blockchain
}

func (blockchain *Blockchain) SetNode(node *Node) {
	blockchain.Node = node
}

func (blockchain *Blockchain) HandleMessage(msg *Message, from peer.ID) error {
	switch msg.Type {
	case MessageTypeTx:
		var transaction Transaction
		if err := json.Unmarshal(msg.Payload, &transaction); err != nil {
			return err
		}
		transaction.Hash()
		return blockchain.AddTransactionToMempool(&transaction, true, from)

	case MessageTypeBlock:
		var block Block
		if err := json.Unmarshal(msg.Payload, &block); err != nil {
			return err
		}
		return blockchain.addBlockFromNetwork(&block, from)

	case MessageTypeGetStatus:
		logrus.WithField("from", from.String()).Info("Received GetStatus request")

		var gossipedPeers []string
		uniquePeers := make(map[string]struct{})

		for _, pID := range blockchain.Node.Host.Peerstore().Peers() {
			if blockchain.Node.IsSelf(pID) {
				continue
			}

			if pID.String() == "" {
				logrus.WithField("peer_id_object", pID).Warn("Skipping gossiped peer due to empty Peer ID object")
				continue
			}

			addrs := blockchain.Node.Host.Peerstore().Addrs(pID)
			logrus.WithFields(logrus.Fields{
				"peer_id": pID.String(),
				"addrs_from_peerstore": addrs,
			}).Debug("Addresses retrieved from peerstore for gossiping")

			for _, addr := range addrs {
				addrStr := addr.String()
				logrus.WithField("addr_string_from_peerstore", addrStr).Debug("Multiaddress string from peerstore")

				if (strings.Contains(addrStr, "/ip4/") || strings.Contains(addrStr, "/dns4/")) && !strings.Contains(addrStr, "/ip4/127.0.0.1/") {
					fullPeerAddr := fmt.Sprintf("%s/p2p/%s", addrStr, pID.String())

					logrus.WithFields(logrus.Fields{
						"original_addr_from_peerstore": addrStr,
						"peer_id_string_for_gossip": pID.String(),
						"constructed_full_peer_addr": fullPeerAddr,
					}).Debug("Constructing gossiped peer address")

					if _, exists := uniquePeers[fullPeerAddr]; !exists {
						uniquePeers[fullPeerAddr] = struct{}{}
						gossipedPeers = append(gossipedPeers, fullPeerAddr)
					}
				}
			}
		}

		statusPayload := &StatusPayload{
			CurrentHeight:       uint32(len(blockchain.Blocks) - 1),
			PeerAddresses:       gossipedPeers,
			SenderListenAddress: blockchain.Node.GetFirstListenAddress(),
		}
		payload, err := json.Marshal(statusPayload)
		if err != nil {
			return err
		}
		responseMsg := &Message{
			Type:    MessageTypeStatus,
			Payload: payload,
		}
		return blockchain.Node.Send(from, responseMsg)

	case MessageTypeStatus:
		var status StatusPayload
		if err := json.Unmarshal(msg.Payload, &status); err != nil {
			return err
		}
		logrus.WithFields(logrus.Fields{
			"from":                  from.String(),
			"height":                status.CurrentHeight,
			"peers":                 status.PeerAddresses,
			"sender_listen_address": status.SenderListenAddress,
		}).Info("Received Status message")

		if status.SenderListenAddress != "" {
			blockchain.Node.AddKnownPeer(status.SenderListenAddress)
		}

		for _, peerAddr := range status.PeerAddresses {
			logrus.WithField("gossiped_peer_addr_received", peerAddr).Debug("Processing gossiped peer address")
			peerMa, err := multiaddr.NewMultiaddr(peerAddr)
			if err != nil {
				logrus.WithError(err).WithField("peer_address", peerAddr).Warn("Failed to parse gossiped multiaddress")
				continue
			}

			peerInfo, err := peer.AddrInfoFromP2pAddr(peerMa)
			if err != nil {
				logrus.WithError(err).WithField("peer_address", peerAddr).Warn("Failed to extract AddrInfo from gossiped address. Check if /p2p/PEER_ID is missing.")
				continue
			}

			if blockchain.Node.IsSelf(peerInfo.ID) {
				continue
			}

			if blockchain.Node.Host.Network().Connectedness(peerInfo.ID) != network.Connected {
				logrus.WithFields(logrus.Fields{
					"address": peerAddr,
					"peer_id": peerInfo.ID.String(),
				}).Debug("Found new peer via gossip, attempting to connect")

				go func(addr string) {
					if err := blockchain.Node.Connect(addr); err != nil {
						logrus.WithError(err).WithField("address", addr).Error("Failed to connect to gossiped peer")
					}
				}(peerAddr)
			} else {
				logrus.WithFields(logrus.Fields{
					"address": peerAddr,
					"peer_id": peerInfo.ID.String(),
				}).Debug("Already connected to gossiped peer, skipping connection attempt")
			}
		}
	}
	return nil
}

func (blockchain *Blockchain) getDifficulty() int {
	latestBlock := blockchain.Blocks[len(blockchain.Blocks)-1]
	if latestBlock.Index%blockchain.config.DifficultyAdjustmentInterval == 0 && latestBlock.Index != 0 {
		return blockchain.calculateDifficulty()
	}
	return latestBlock.Difficulty
}

func (blockchain *Blockchain) calculateDifficulty() int {
	firstBlock := blockchain.Blocks[len(blockchain.Blocks)-int(blockchain.config.DifficultyAdjustmentInterval)]
	actualTimeTaken := blockchain.Blocks[len(blockchain.Blocks)-1].Timestamp - firstBlock.Timestamp
	expectedTimeTaken := int64(blockchain.config.BlockGenerationInterval) * blockchain.config.DifficultyAdjustmentInterval

	if actualTimeTaken < expectedTimeTaken/2 {
		return blockchain.difficulty + 1
	} else if actualTimeTaken > expectedTimeTaken*2 {
		newDifficulty := blockchain.difficulty - 1
		if newDifficulty < 1 {
			return 1
		}
		return newDifficulty
	}
	return blockchain.difficulty
}

func (blockchain *Blockchain) AddBlock() *Block {
	blockchain.Mu.Lock()
	defer blockchain.Mu.Unlock()

	startTime := time.Now()

	txs := make([]*Transaction, 0, len(blockchain.Mempool))
	for _, tx := range blockchain.Mempool {
		txs = append(txs, tx)
	}

	prevBlock := blockchain.Blocks[len(blockchain.Blocks)-1]
	difficulty := blockchain.getDifficulty()
	newBlock := newBlock(prevBlock.Index+1, txs, prevBlock.Hash, difficulty)
	newBlock.mine()

	duration := time.Since(startTime)
	MiningTime.Observe(duration.Seconds())
	TransactionsPerBlock.Observe(float64(len(newBlock.Transactions)))

	if err := blockchain.persistBlock(newBlock); err != nil {
		logrus.WithError(err).Panic("Failed to save block to database")
	}

	blockchain.Blocks = append(blockchain.Blocks, newBlock)
	blockchain.Mempool = make(map[string]*Transaction)

	BlockHeight.Inc()
	MempoolSize.Set(0)

	logrus.WithFields(logrus.Fields{
		"index":      newBlock.Index,
		"hash":       newBlock.Hash,
		"nonce":      newBlock.Nonce,
		"duration":   duration,
		"difficulty": difficulty,
	}).Info("Block mined")

	if err := blockchain.broadcastBlock(newBlock); err != nil {
		logrus.WithError(err).Error("Failed to broadcast block")
	}

	return newBlock
}

func (blockchain *Blockchain) addBlockFromNetwork(block *Block, from peer.ID) error {
	blockchain.Mu.Lock()
	defer blockchain.Mu.Unlock()

	latestBlock := blockchain.Blocks[len(blockchain.Blocks)-1]

	if block.Index <= latestBlock.Index {
		logrus.WithFields(logrus.Fields{
			"received_index": block.Index,
			"current_index":  latestBlock.Index,
		}).Debug("Received block is not higher than current block, ignoring")
		return nil
	}

	if block.PrevBlockHash != latestBlock.Hash {
		return errors.New("invalid previous block hash")
	}
	if block.calculateHash() != block.Hash {
		return errors.New("invalid block hash")
	}

	if err := blockchain.persistBlock(block); err != nil {
		return err
	}

	blockchain.Blocks = append(blockchain.Blocks, block)
	blockchain.removeMempoolTxs(block.Transactions)

	BlockHeight.Inc()
	TransactionsPerBlock.Observe(float64(len(block.Transactions)))

	logrus.WithFields(logrus.Fields{
		"index": block.Index,
		"hash":  block.Hash,
	}).Info("Added block from network")

	if err := blockchain.broadcastBlock(block, from); err != nil {
		logrus.WithError(err).Error("Failed to relay block")
	}

	return nil
}

func (blockchain *Blockchain) removeMempoolTxs(blockTxs []*Transaction) {
	for _, tx := range blockTxs {
		delete(blockchain.Mempool, tx.ID)
	}
	MempoolSize.Set(float64(len(blockchain.Mempool)))
}

func (blockchain *Blockchain) persistBlock(block *Block) error {
	return blockchain.db.Update(func(boltTx *bolt.Tx) error {
		bucket := boltTx.Bucket([]byte(blocksBucket))
		encoded, err := json.Marshal(block)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(block.Hash), encoded)
	})
}

func (blockchain *Blockchain) AddTransactionToMempool(transaction *Transaction, fromNetwork bool, excludePeer ...peer.ID) error {
	blockchain.Mu.Lock()
	defer blockchain.Mu.Unlock()

	if transaction.ID == "" {
		transaction.Hash()
	}

	logrus.WithFields(logrus.Fields{
		"transaction_id": transaction.ID,
		"from_network":   fromNetwork,
	}).Debug("Attempting to add transaction to mempool")

	if _, exists := blockchain.Mempool[transaction.ID]; exists {
		logrus.WithFields(logrus.Fields{
			"transaction_id": transaction.ID,
			"from_network":   fromNetwork,
		}).Debug("Transaction already exists in mempool, skipping")
		return nil
	}

	blockchain.Mempool[transaction.ID] = transaction
	MempoolSize.Inc()

	if !fromNetwork || (len(excludePeer) > 0) {
		if err := blockchain.broadcastTransaction(transaction, excludePeer...); err != nil {
			logrus.WithError(err).Error("Failed to broadcast transaction")
		}
	}

	logrus.WithFields(logrus.Fields{
		"transaction_id": transaction.ID,
		"from_network":   fromNetwork,
	}).Info("Transaction added to mempool")

	return nil
}

func (blockchain *Blockchain) broadcastTransaction(transaction *Transaction, excludePeers ...peer.ID) error {
	payload, err := json.Marshal(transaction)
	if err != nil {
		return err
	}

	msg := &Message{
		Type:    MessageTypeTx,
		Payload: payload,
	}

	return blockchain.Node.Broadcast(msg, excludePeers...)
}

func (blockchain *Blockchain) broadcastBlock(block *Block, excludePeers ...peer.ID) error {
	payload, err := json.Marshal(block)
	if err != nil {
		return err
	}

	msg := &Message{
		Type:    MessageTypeBlock,
		Payload: payload,
	}

	return blockchain.Node.Broadcast(msg, excludePeers...)
}

func (blockchain *Blockchain) VerifyChain() bool {
	for i := 1; i < len(blockchain.Blocks); i++ {
		currentBlock := blockchain.Blocks[i]
		prevBlock := blockchain.Blocks[i-1]

		if currentBlock.Hash != currentBlock.calculateHash() {
			logrus.WithFields(logrus.Fields{
				"block_index":   currentBlock.Index,
				"expected_hash": currentBlock.calculateHash(),
				"actual_hash":   currentBlock.Hash,
			}).Warn("Chain verification failed: block hash mismatch")
			return false
		}

		if currentBlock.PrevBlockHash != prevBlock.Hash {
			logrus.WithFields(logrus.Fields{
				"block_index":        currentBlock.Index,
				"expected_prev_hash": prevBlock.Hash,
				"actual_prev_hash":   currentBlock.PrevBlockHash,
			}).Warn("Chain verification failed: previous block hash mismatch")
			return false
		}
	}
	logrus.Info("Chain verification successful")
	return true
}

func (blockchain *Blockchain) Close() {
	logrus.Info("Closing database")
	if err := blockchain.db.Close(); err != nil {
		logrus.WithError(err).Error("Failed to close database")
	}
}
