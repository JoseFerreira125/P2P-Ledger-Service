package service

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/infra"
	metric "github.com/joseferreira/Immutable-Ledger-Service/internal/infra"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sirupsen/logrus"
)

const ProtocolID = "/mils/1.0.0"

type MessageHandler interface {
	HandleMessage(*domain.Message, peer.ID) error
}

type P2PService struct {
	Host    host.Host
	handler MessageHandler
	config  *infra.Config
	ticker  *time.Ticker
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewP2PService(ctx context.Context, config *infra.Config) (*P2PService, error) {
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to generate libp2p key pair: %w", err)
	}

	sourceMultiAddr, err := multiaddr.NewMultiaddr(config.P2PListenAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse listen address: %w", err)
	}

	h, err := libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(privKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	childCtx, cancel := context.WithCancel(ctx)
	p2p := &P2PService{
		Host:   h,
		config: config,
		ticker: time.NewTicker(config.PeerDiscoveryInterval),
		ctx:    childCtx,
		cancel: cancel,
	}

	p2p.Host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, conn network.Conn) {
			logrus.WithField("peer_id", conn.RemotePeer().String()).Info("Peer connected")
			metric.PeerCount.Inc()
		},
		DisconnectedF: func(_ network.Network, conn network.Conn) {
			logrus.WithField("peer_id", conn.RemotePeer().String()).Info("Peer disconnected")
			metric.PeerCount.Dec()
		},
	})

	logrus.WithFields(logrus.Fields{
		"peer_id": h.ID().String(),
		"addrs":   h.Addrs(),
	}).Info("LibP2P Host created")

	return p2p, nil
}

func (p *P2PService) SetHandler(handler MessageHandler) {
	p.handler = handler
	p.Host.SetStreamHandler(ProtocolID, p.handleNewStream)
}

func (p *P2PService) Start() {
	go p.runPeerMaintenance()
	logrus.WithFields(logrus.Fields{
		"peer_id": p.Host.ID().String(),
		"addrs":   p.Host.Addrs(),
	}).Info("P2P Service started and listening for connections")
}

func (p *P2PService) Close() error {
	p.cancel()
	p.ticker.Stop()
	logrus.Info("Shutting down libp2p host...")
	return p.Host.Close()
}

func (p *P2PService) handleNewStream(s network.Stream) {
	defer s.Close()

	decoder := gob.NewDecoder(s)
	msg := &domain.Message{}

	if err := decoder.Decode(msg); err != nil {
		if err != io.EOF {
			logrus.WithError(err).WithField("remote_peer", s.Conn().RemotePeer().String()).Error("Error decoding message from stream")
		}
		return
	}

	if p.handler != nil {
		if err := p.handler.HandleMessage(msg, s.Conn().RemotePeer()); err != nil {
			logrus.WithError(err).WithField("remote_peer", s.Conn().RemotePeer().String()).Error("Error handling message")
		}
	}
}

func (p *P2PService) Broadcast(msg *domain.Message, excludePeers ...peer.ID) error {
	excludeMap := make(map[peer.ID]struct{})
	for _, pID := range excludePeers {
		excludeMap[pID] = struct{}{}
	}

	var errs []error
	for _, pID := range p.Host.Network().Peers() {
		if pID == p.Host.ID() {
			continue
		}
		if _, ok := excludeMap[pID]; ok {
			continue
		}

		if err := p.Send(pID, msg); err != nil {
			logrus.WithError(err).WithField("peer_id", pID.String()).Error("Failed to broadcast message to peer")
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("broadcast encountered errors: %v", errs)
	}
	return nil
}

func (p *P2PService) Send(to peer.ID, msg *domain.Message) error {
	s, err := p.Host.NewStream(p.ctx, to, ProtocolID)
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

func (p *P2PService) GetConnectedPeerAddresses() []string {
	var addrs []string
	peerAddrs := make(map[peer.ID]string)

	for _, pID := range p.Host.Network().Peers() {
		if pID == p.Host.ID() {
			continue
		}

		if _, ok := peerAddrs[pID]; ok {
			continue
		}

		var chosenAddr multiaddr.Multiaddr
		for _, addr := range p.Host.Peerstore().Addrs(pID) {
			if !strings.Contains(addr.String(), "127.0.0.1") {
				chosenAddr = addr
				break
			}
		}

		if chosenAddr == nil && len(p.Host.Peerstore().Addrs(pID)) > 0 {
			chosenAddr = p.Host.Peerstore().Addrs(pID)[0]
		}

		if chosenAddr != nil {
			fullAddr := fmt.Sprintf("%s/p2p/%s", chosenAddr.String(), pID.String())
			peerAddrs[pID] = fullAddr
		}
	}

	for _, addr := range peerAddrs {
		addrs = append(addrs, addr)
	}
	return addrs
}

func (p *P2PService) Connect(addr string) error {
	if strings.Contains(addr, p.Host.ID().String()) {
		return nil
	}

	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddress: %w", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("failed to get peer info from multiaddress: %w", err)
	}

	if p.Host.Network().Connectedness(info.ID) == network.Connected {
		return nil
	}

	p.Host.Peerstore().AddAddrs(info.ID, info.Addrs, 24*time.Hour)

	if err := p.Host.Connect(p.ctx, *info); err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", info.ID.String(), err)
	}

	logrus.WithField("peer_id", info.ID.String()).Info("Successfully connected to peer")
	return nil
}

func (p *P2PService) runPeerMaintenance() {
	for {
		select {
		case <-p.ticker.C:
			msg := &domain.Message{Type: domain.MessageTypeGetPeers}
			if err := p.Broadcast(msg); err != nil {
				logrus.WithError(err).Warn("Failed to broadcast GetPeers message")
			}
		case <-p.ctx.Done():
			return
		}
	}
}
