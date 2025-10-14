package domain

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"

	"github.com/libp2p/go-libp2p/core/crypto"
)

const ProtocolID = "/mils/1.0.0"

type MessageHandler interface {
	HandleMessage(*Message, peer.ID) error
}

type Node struct {
	Host    host.Host
	handler MessageHandler
	ticker  *time.Ticker
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewNode(listenAddress string, config *Config) *Node {
	ctx, cancel := context.WithCancel(context.Background())

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to generate libp2p key pair")
	}

	sourceMultiAddr, err := multiaddr.NewMultiaddr(listenAddress)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse listen address")
	}

	host, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(privKey),
		libp2p.DisableRelay(),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.Ping(false),
		libp2p.UserAgent("Immutable-Ledger-Service"),
	)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create libp2p host")
	}

	logrus.WithFields(logrus.Fields{
		"peer_id": host.ID().String(),
		"addrs":   host.Addrs(),
	}).Info("LibP2P Host created")

	node := &Node{
		Host:   host,
		ticker: time.NewTicker(config.PeriodicSyncInterval),
		ctx:    ctx,
		cancel: cancel,
	}

	node.Host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, conn network.Conn) {
			logrus.WithField("peer_id", conn.RemotePeer().String()).Info("Peer connected")
			PeerCount.Inc()
		},
		DisconnectedF: func(_ network.Network, conn network.Conn) {
			logrus.WithField("peer_id", conn.RemotePeer().String()).Info("Peer disconnected")
			PeerCount.Dec()
		},
	})

	return node
}

func (node *Node) ListenAddress() []multiaddr.Multiaddr {
	return node.Host.Addrs()
}

func (node *Node) GetFirstListenAddress() string {
	for _, addr := range node.Host.Addrs() {
		if strings.Contains(addr.String(), "/ip4/") && !strings.Contains(addr.String(), "/ip4/127.0.0.1/") {
			fullAddr := addr.String() + "/p2p/" + node.Host.ID().String()
			logrus.WithFields(logrus.Fields{
				"original_addr": addr.String(),
				"full_addr":     fullAddr,
			}).Debug("GetFirstListenAddress: Constructed full address")
			return fullAddr
		}
	}
	logrus.Warn("GetFirstListenAddress: No suitable listen address found")
	return ""
}

func (node *Node) IsSelf(pID peer.ID) bool {
	return node.Host.ID() == pID
}

func (node *Node) GetConnectedPeerIDs() []peer.ID {
	return node.Host.Network().Peers()
}

func (node *Node) SetHandler(handler MessageHandler) {
	node.handler = handler
}

func (node *Node) Send(to peer.ID, msg *Message) error {
	s, err := node.Host.NewStream(node.ctx, to, ProtocolID)
	if err != nil {
		return fmt.Errorf("failed to open stream to %s: %w", to.String(), err)
	}
	defer s.Close()

	encoder := gob.NewEncoder(s)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message for %s: %w", to.String(), err)
	}
	return nil
}

func (node *Node) Broadcast(msg *Message, excludePeers ...peer.ID) error {
	excludeMap := make(map[peer.ID]struct{})
	for _, p := range excludePeers {
		excludeMap[p] = struct{}{}
	}

	var errs []error
	for _, p := range node.Host.Peerstore().PeersWithAddrs() {
		if p == node.Host.ID() {
			continue
		}
		if _, ok := excludeMap[p]; ok {
			continue
		}

		if node.Host.Network().Connectedness(p) != network.Connected {
			continue
		}

		if err := node.Send(p, msg); err != nil {
			logrus.WithError(err).WithField("peer_id", p.String()).Error("Failed to broadcast message to peer")
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("broadcast encountered errors: %v", errs)
	}
	return nil
}

func (node *Node) AddKnownPeer(addr string) {
	logrus.WithField("input_addr", addr).Debug("AddKnownPeer: Attempting to add known peer")
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		logrus.WithError(err).WithField("address", addr).Warn("AddKnownPeer: Invalid multiaddress")
		return
	}

	info, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		logrus.WithError(err).WithField("address", addr).Warn("AddKnownPeer: Failed to parse AddrInfo from multiaddress")
		return
	}

	logrus.WithFields(logrus.Fields{
		"input_addr":   addr,
		"parsed_peer_id": info.ID.String(),
		"parsed_addrs":   info.Addrs,
	}).Debug("AddKnownPeer: Parsed AddrInfo")

	if info.ID == node.Host.ID() {
		logrus.WithField("address", addr).Debug("AddKnownPeer: Not adding self-address to known peers")
		return
	}

	node.Host.Peerstore().AddAddrs(info.ID, info.Addrs, time.Hour)
	logrus.WithFields(logrus.Fields{
		"peer_id": info.ID.String(),
		"address": ma.String(),
	}).Debug("AddKnownPeer: Added new known peer")
}

func (node *Node) GetAllKnownPeerAddresses() []string {
	addresses := make([]string, 0)
	for _, p := range node.Host.Peerstore().Peers() {
		addrs := node.Host.Peerstore().Addrs(p)
		for _, addr := range addrs {
			addresses = append(addresses, addr.String())
		}
	}
	return addresses
}

func (node *Node) GetConnectedPeerStableAddresses() []string {
	var connectedAddrs []string
	for _, p := range node.Host.Network().Peers() {
		if p == node.Host.ID() {
			continue
		}
		addrs := node.Host.Peerstore().Addrs(p)
		for _, addr := range addrs {
			connectedAddrs = append(connectedAddrs, addr.String())
		}
	}
	return connectedAddrs
}

func (node *Node) Start() error {
	node.Host.SetStreamHandler(ProtocolID, node.handleNewStream)

	logrus.WithFields(logrus.Fields{
		"peer_id": node.Host.ID().String(),
		"addrs":   node.Host.Addrs(),
	}).Info("LibP2P Node started and listening for connections")

	go node.periodicSync()

	return nil
}

func (node *Node) Close() error {
	node.cancel()
	node.ticker.Stop()
	logrus.Info("Shutting down libp2p host...")
	return node.Host.Close()
}

func (node *Node) handleNewStream(s network.Stream) {
	logrus.WithField("remote_peer", s.Conn().RemotePeer().String()).Debug("Received new stream")

	defer s.Close()

	decoder := gob.NewDecoder(s)
	msg := &Message{}

	for {
		if err := decoder.Decode(msg); err != nil {
			if err == io.EOF {
				break
			}
			logrus.WithError(err).WithField("remote_peer", s.Conn().RemotePeer().String()).Error("Error decoding message from stream")
			return
		}

		if node.handler != nil {
			if err := node.handler.HandleMessage(msg, s.Conn().RemotePeer()); err != nil {
				logrus.WithError(err).WithField("remote_peer", s.Conn().RemotePeer().String()).Error("Error handling message from stream")
			}
		}
	}
}

func (node *Node) periodicSync() {
	for {
		select {
		case <-node.ticker.C:
			logrus.Debug("Running periodic sync")
			msg := &Message{Type: MessageTypeGetStatus}
			node.Broadcast(msg)

			for _, pID := range node.Host.Peerstore().Peers() {
				if node.Host.Network().Connectedness(pID) != network.Connected && pID != node.Host.ID() {
					logrus.WithField("peer_id", pID.String()).Debug("Found new peer in known list, attempting to connect")
					go func(targetID peer.ID) {
						addrs := node.Host.Peerstore().Addrs(targetID)
						if len(addrs) == 0 {
							logrus.WithField("peer_id", targetID.String()).Warn("No addresses found for known peer, cannot connect")
							return
						}

						for _, addr := range addrs {
							addrStr := addr.String()
							if !strings.Contains(addrStr, "/p2p/") {
								addrStr = fmt.Sprintf("%s/p2p/%s", addrStr, targetID.String())
							}

							if err := node.Connect(addrStr); err != nil {
								logrus.WithError(err).WithFields(logrus.Fields{
									"peer_id": targetID.String(),
									"address": addrStr,
								}).Warn("Failed to connect to known peer via multiaddress")
							} else {
								logrus.WithFields(logrus.Fields{
									"peer_id": targetID.ID.String(),
									"address": addrStr,
								}).Debug("Successfully connected to known peer via multiaddress during periodic sync")
								break
							}
						}
					}(pID)
				}
			}
		case <-node.ctx.Done():
			return
		}
	}
}

func (node *Node) Connect(addr string) error {
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddress: %w", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("multiaddress %s does not contain a peer ID, cannot connect: %w", addr, err)
	}

	logrus.WithFields(logrus.Fields{
		"connect_input_addr": addr,
		"parsed_peer_id":     info.ID.String(),
		"parsed_addrs":       info.Addrs,
	}).Debug("Connect: Parsed AddrInfo from multiaddress")

	if info.ID == node.Host.ID() {
		logrus.WithField("address", addr).Debug("Connect: Attempted to connect to self-address, ignoring")
		return nil
	}

	if node.Host.Network().Connectedness(info.ID) == network.Connected {
		logrus.WithFields(logrus.Fields{
			"peer_id": info.ID.String(),
			"address": addr,
		}).Debug("Already connected to peer, skipping redundant connection attempt")
		return nil
	}

	node.Host.Peerstore().AddAddrs(info.ID, info.Addrs, time.Hour)

	if err := node.Host.Connect(node.ctx, *info); err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", info.ID.String(), err)
	}

	logrus.WithFields(logrus.Fields{
		"peer_id": info.ID.String(),
		"address": addr,
	}).Debug("Successfully connected to peer")

	return nil
}
