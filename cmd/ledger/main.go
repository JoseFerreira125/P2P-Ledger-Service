// @title           Minimalist Immutable Ledger Service (MILS)
// @version         1.0
// @description     A decentralized, peer-to-peer immutable ledger service.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/api"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/infra"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/persistence"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/service"
	"github.com/sirupsen/logrus"

	_ "github.com/joseferreira/Immutable-Ledger-Service/docs" // Blank import for swag docs
)

func main() {
	connectAddr := flag.String("connect", "", "address of a peer to connect to")
	flag.Parse()

	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel) // Set log level to Debug

	config := infra.LoadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := persistence.NewPersistenceService()
	defer repo.Close()

	mempoolService := service.NewMempoolService()
	blockchainService := service.NewBlockchainService(ctx, repo, mempoolService, config)

	p2pService, err := service.NewP2PService(ctx, config)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create P2P service")
	}

	nodeService := service.NewNodeService(ctx, blockchainService, mempoolService, p2pService)
	defer nodeService.Close()

	nodeService.Start()

	if *connectAddr != "" {
		if err := p2pService.Connect(*connectAddr); err != nil {
			logrus.WithError(err).Error("Failed to connect to peer")
		}
	}

	apiServer := api.NewServer(config.HTTPListenAddress, nodeService)
	go func() {
		if err := apiServer.Start(); err != nil {
			logrus.WithError(err).Fatal("Failed to start API server")
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	logrus.Info("Shutting down...")
}
