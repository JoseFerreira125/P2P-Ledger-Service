// @title           Minimalist Immutable Ledger Service (MILS)
// @version         1.0
// @description     A decentralized, peer-to-peer immutable ledger service.
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/api"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/sirupsen/logrus"

	_ "github.com/joseferreira/Immutable-Ledger-Service/docs" // Blank import for swag docs
)

func main() {
	connectAddr := flag.String("connect", "", "address of a peer to connect to")
	flag.Parse()

	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)

	config := domain.LoadConfig()

	blockchain := domain.NewBlockchain(config)
	defer blockchain.Close()

	node := domain.NewNode(config.P2PListenAddress, config)
	defer node.Close() // Ensure the libp2p host is properly closed

	blockchain.SetNode(node)
	node.SetHandler(blockchain)

	if err := node.Start(); err != nil {
		logrus.WithError(err).Fatal("Failed to start P2P node")
	}

	if *connectAddr != "" {
		if err := node.Connect(*connectAddr); err != nil {
			logrus.WithError(err).Error("Failed to connect to peer")
		}
	}

	go func() {
		apiServer := api.NewServer(config.HTTPListenAddress, blockchain)
		if err := apiServer.Start(); err != nil {
			logrus.WithError(err).Fatal("Failed to start API server")
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	logrus.Info("Shutting down...")
}
