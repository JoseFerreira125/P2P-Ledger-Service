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

// ProtocolID is the unique identifier for our application's protocol.
const ProtocolID = "/mils/1.0.0"

// MessageHandler interface for handling incoming messages.
// It now receives a peer.ID instead of net.Addr.
type MessageHandler interface {
	HandleMessage(*Message, peer.ID) error
}

// Node represents a P2P node in the network.
type Node struct {
	Host    host.Host
	handler MessageHandler
	ticker  *time.Ticker
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewNode creates and initializes a new P2P node using libp2p.
func NewNode(listenAddress string, config *Config) *Node {
	ctx, cancel := context.WithCancel(context.Background())

	// Generate a new identity for this host
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to generate libp2p key pair")
	}

	// Parse the listen address
	sourceMultiAddr, err := multiaddr.NewMultiaddr(listenAddress)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse listen address")
	}

	// Create a new libp2p Host
	host, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(privKey),
		libp2p.DisableRelay(),       // Disable relay for simplicity, can be enabled for NAT traversal
		libp2p.NATPortMap(),         // Attempt to open ports using NATPNP or UPnP
		libp2p.EnableHolePunching(), // Enable hole punching for better connectivity
		libp2p.Ping(false),          // Disable built-in ping to manage our own liveness
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

	// Set up network notifiers for peer connections and disconnections
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

// ListenAddress returns the multiaddresses this node is listening on.
func (node *Node) ListenAddress() []multiaddr.Multiaddr {
	return node.Host.Addrs()
}

// GetFirstListenAddress returns the string representation of the first non-loopback listen address.
// This is useful for advertising a stable address for other peers to connect to.
func (node *Node) GetFirstListenAddress() string {
	for _, addr := range node.Host.Addrs() {
		// Filter out loopback addresses and return the first valid IPv4 address
		if strings.Contains(addr.String(), "/ip4/") && !strings.Contains(addr.String(), "/ip4/127.0.0.1/") {
			// Append the peer ID to the multiaddress
			fullAddr := addr.String() + "/p2p/" + node.Host.ID().String()
			logrus.WithFields(logrus.Fields{
				"original_addr": addr.String(),
				"full_addr":     fullAddr,
			}).Debug("GetFirstListenAddress: Constructed full address")
			return fullAddr
		}
	}
	logrus.Warn("GetFirstListenAddress: No suitable listen address found")
	return "" // Or handle error appropriately
}

// IsSelf checks if the given peer.ID is the node's own ID.
func (node *Node) IsSelf(pID peer.ID) bool {
	return node.Host.ID() == pID
}

// GetConnectedPeerIDs returns a slice of peer.ID of all currently connected peers.
func (node *Node) GetConnectedPeerIDs() []peer.ID {
	return node.Host.Network().Peers()
}

// SetHandler sets the message handler for the node.
func (node *Node) SetHandler(handler MessageHandler) {
	node.handler = handler
}

// Send sends a message to a specific peer.
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

// Broadcast sends a message to all currently connected peers, excluding specified ones.
func (node *Node) Broadcast(msg *Message, excludePeers ...peer.ID) error {
	excludeMap := make(map[peer.ID]struct{})
	for _, p := range excludePeers {
		excludeMap[p] = struct{}{}
	}

	var errs []error
	for _, p := range node.Host.Peerstore().PeersWithAddrs() {
		if p == node.Host.ID() { // Don't send to self
			continue
		}
		if _, ok := excludeMap[p]; ok {
			continue
		}

		// Check if we are actually connected to this peer
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

// AddKnownPeer adds a multiaddress of a known peer to the node's peerstore.
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

	// Add the peer's address to our peerstore
	node.Host.Peerstore().AddAddrs(info.ID, info.Addrs, time.Hour) // Store for an hour
	logrus.WithFields(logrus.Fields{
		"peer_id": info.ID.String(),
		"address": ma.String(),
	}).Debug("AddKnownPeer: Added new known peer")
}

// GetAllKnownPeerAddresses returns all known peer multiaddresses.
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

// GetConnectedPeerStableAddresses returns the multiaddresses of currently connected peers.
func (node *Node) GetConnectedPeerStableAddresses() []string {
	var connectedAddrs []string
	for _, p := range node.Host.Network().Peers() {
		if p == node.Host.ID() {
			continue
		}
		// Get the multiaddresses from the peerstore for the connected peer
		addrs := node.Host.Peerstore().Addrs(p)
		for _, addr := range addrs {
			connectedAddrs = append(connectedAddrs, addr.String())
		}
	}
	return connectedAddrs
}

// Start begins the libp2p host and sets up stream handlers.
func (node *Node) Start() error {
	// Set a stream handler for our protocol.
	// This function will be called when a remote peer opens a stream to us.
	node.Host.SetStreamHandler(ProtocolID, node.handleNewStream)

	logrus.WithFields(logrus.Fields{
		"peer_id": node.Host.ID().String(),
		"addrs":   node.Host.Addrs(),
	}).Info("LibP2P Node started and listening for connections")

	go node.periodicSync()

	return nil
}

// Close shuts down the libp2p host.
func (node *Node) Close() error {
	node.cancel()
	node.ticker.Stop()
	logrus.Info("Shutting down libp2p host...")
	return node.Host.Close()
}

// handleNewStream is called when a new stream for ProtocolID is opened to us.
func (node *Node) handleNewStream(s network.Stream) {
	logrus.WithField("remote_peer", s.Conn().RemotePeer().String()).Debug("Received new stream")

	defer s.Close()

	decoder := gob.NewDecoder(s)
	msg := &Message{}

	for {
		if err := decoder.Decode(msg); err != nil {
			if err == io.EOF {
				break // Connection closed
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
			// Broadcast GetStatus to all currently connected peers
			node.Broadcast(msg)

			// Iterate through all peers in the peerstore
			for _, pID := range node.Host.Peerstore().Peers() {
				// Only attempt to connect if not already connected and not self
				if node.Host.Network().Connectedness(pID) != network.Connected && pID != node.Host.ID() {
					logrus.WithField("peer_id", pID.String()).Debug("Found new peer in known list, attempting to connect")
					go func(targetID peer.ID) {
						addrs := node.Host.Peerstore().Addrs(targetID)
						if len(addrs) == 0 {
							logrus.WithField("peer_id", targetID.String()).Warn("No addresses found for known peer, cannot connect")
							return
						}

						// Try connecting to any of the known addresses for this peer
						for _, addr := range addrs {
							// Ensure the multiaddress contains the peer ID before attempting to connect
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
									"peer_id": targetID.String(),
									"address": addrStr,
								}).Debug("Successfully connected to known peer via multiaddress during periodic sync")
								break // Connected, no need to try other addresses for this peer
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

// Connect attempts to connect to a peer at the given multiaddress.
func (node *Node) Connect(addr string) error {
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddress: %w", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		// If AddrInfoFromP2pAddr fails, it means 'ma' doesn't contain a peer ID.
		// In this case, we cannot connect without a peer ID.
		return fmt.Errorf("multiaddress %s does not contain a peer ID, cannot connect: %w", addr, err)
	}

	logrus.WithFields(logrus.Fields{
		"connect_input_addr": addr,
		"parsed_peer_id":     info.ID.String(),
		"parsed_addrs":       info.Addrs,
	}).Debug("Connect: Parsed AddrInfo from multiaddress")

	if info.ID == node.Host.ID() {
		logrus.WithField("address", addr).Debug("Connect: Attempted to connect to self-address, ignoring")
		return nil // Don't connect to self
	}

	// Check if already connected to this peer
	if node.Host.Network().Connectedness(info.ID) == network.Connected {
		logrus.WithFields(logrus.Fields{
			"peer_id": info.ID.String(),
			"address": addr,
		}).Debug("Already connected to peer, skipping redundant connection attempt")
		return nil
	}

	// Add the peer's address to our peerstore before connecting
	// We now ensure info.ID is always present here.
	node.Host.Peerstore().AddAddrs(info.ID, info.Addrs, time.Hour)

	// Connect to the peer
	if err := node.Host.Connect(node.ctx, *info); err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", info.ID.String(), err)
	}

	logrus.WithFields(logrus.Fields{
		"peer_id": info.ID.String(),
		"address": addr,
	}).Debug("Successfully connected to peer")

	return nil
}
