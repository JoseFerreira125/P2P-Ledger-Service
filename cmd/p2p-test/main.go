package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/sirupsen/logrus"
)

func main() {
	listenAddr := flag.String("listen", ":3000", "address for the node to listen on")
	connectAddr := flag.String("connect", "", "address of a peer to connect to")
	flag.Parse()

	logrus.SetFormatter(&logrus.JSONFormatter{})

	node := domain.NewNode(*listenAddr)
	if err := node.Start(); err != nil {
		logrus.WithError(err).Fatal("Failed to start node")
	}

	if *connectAddr != "" {
		if err := node.Connect(*connectAddr); err != nil {
			logrus.WithError(err).Error("Failed to connect to peer")
		}
	}

	fmt.Println("Node is running. Press Ctrl+C to exit.")
	// Keep the application running
	for {
		time.Sleep(10 * time.Second)
	}
}
