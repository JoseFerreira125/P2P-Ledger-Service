package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

type NodeInfo struct {
	PeerID          string   `json:"peer_id"`
	ListenAddresses []string `json:"listen_addresses"`
}

type NodeService struct {
	Blockchain *BlockchainService
	Mempool    *MempoolService
	P2P        *P2PService
	ctx        context.Context
}

func NewNodeService(ctx context.Context, bc *BlockchainService, mempool *MempoolService, p2p *P2PService) *NodeService {
	ns := &NodeService{
		Blockchain: bc,
		Mempool:    mempool,
		P2P:        p2p,
		ctx:        ctx,
	}
	p2p.SetHandler(ns)
	return ns
}

func (ns *NodeService) Start() {
	ns.P2P.Start()
	ns.Blockchain.Start()
}

func (ns *NodeService) Close() {
	ns.Blockchain.Close()
	ns.P2P.Close()
}

func (ns *NodeService) HandleMessage(msg *domain.Message, from peer.ID) error {
	switch msg.Type {
	case domain.MessageTypeTx:
		var tx domain.Transaction
		if err := json.Unmarshal(msg.Payload, &tx); err != nil {
			return fmt.Errorf("failed to unmarshal transaction: %w", err)
		}
		_, err := ns.Mempool.AddTransaction(&tx)
		if err != nil {
			return err
		}
	case domain.MessageTypeBlock:
		var block domain.Block
		if err := json.Unmarshal(msg.Payload, &block); err != nil {
			return fmt.Errorf("failed to unmarshal block: %w", err)
		}
		_, err := ns.Blockchain.AddBlockFromNetwork(&block)
		if err != nil {
			return err
		}
	case domain.MessageTypeGetPeers:
		peers := ns.P2P.GetConnectedPeerAddresses()
		payload, err := json.Marshal(domain.PeersPayload{Peers: peers})
		if err != nil {
			return fmt.Errorf("failed to marshal peers payload: %w", err)
		}
		response := &domain.Message{
			Type:    domain.MessageTypePeers,
			Payload: payload,
		}
		return ns.P2P.Send(from, response)

	case domain.MessageTypePeers:
		var payload domain.PeersPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			return fmt.Errorf("failed to unmarshal peers payload: %w", err)
		}
		for _, addr := range payload.Peers {
			go func(addr string) {
				if err := ns.P2P.Connect(addr); err != nil {
					logrus.WithError(err).WithField("addr", addr).Warn("Failed to connect to peer from peer list")
				}
			}(addr)
		}

	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
	return nil
}

func (ns *NodeService) AddTransaction(tx *domain.Transaction) error {
	isNew, err := ns.Mempool.AddTransaction(tx)
	if err != nil {
		return err
	}

	if !isNew {
		return nil // Transaction already in mempool
	}

	payload, err := json.Marshal(tx)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %w", err)
	}

	msg := &domain.Message{
		Type:    domain.MessageTypeTx,
		Payload: payload,
	}

	return ns.P2P.Broadcast(msg)
}

func (ns *NodeService) MineBlock() (*domain.Block, error) {
	block := ns.Blockchain.AddBlock()

	payload, err := json.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block: %w", err)
	}

	msg := &domain.Message{
		Type:    domain.MessageTypeBlock,
		Payload: payload,
	}

	if err := ns.P2P.Broadcast(msg); err != nil {
		logrus.WithError(err).Error("Failed to broadcast block")
	}

	return block, nil
}

func (ns *NodeService) VerifyChain() bool {
	return ns.Blockchain.VerifyChain()
}

func (ns *NodeService) GetBlockByIndex(index int64) *domain.Block {
	return ns.Blockchain.GetBlockByIndex(index)
}

func (ns *NodeService) GetNodeInfo() *NodeInfo {
	addrs := make([]string, 0)
	for _, addr := range ns.P2P.Host.Addrs() {
		addrs = append(addrs, addr.String())
	}
	return &NodeInfo{
		PeerID:          ns.P2P.Host.ID().String(),
		ListenAddresses: addrs,
	}
}
