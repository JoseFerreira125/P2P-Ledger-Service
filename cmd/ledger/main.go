package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/ledger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var bc *ledger.Blockchain

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)

	config := ledger.LoadConfig()
	bc = ledger.NewBlockchain(config)
	defer bc.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logrus.Info("Shutting down...")
		bc.Close()
		os.Exit(0)
	}()

	http.HandleFunc("/transaction", transactionHandler)
	http.HandleFunc("/mine", mineHandler)
	http.HandleFunc("/chain/verify", verifyChainHandler)
	http.HandleFunc("/block/", getBlockHandler)
	http.Handle("/metrics", promhttp.Handler())

	logrus.Info("Ledger service running on port 8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logrus.WithError(err).Fatal("Failed to start server")
	}
}

func transactionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logrus.WithError(err).Error("Error reading request body")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error reading request body: %v", err)
		return
	}

	var tx ledger.Transaction
	if err := json.Unmarshal(body, &tx); err != nil {
		logrus.WithError(err).Error("Error unmarshalling transaction")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error unmarshalling transaction: %v", err)
		return
	}

	bc.AddTransactionToMempool(&tx)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, "Transaction added to mempool")
}

func mineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// As per the README, this should be asynchronous.
	go bc.AddBlock()

	logrus.Info("Mining process started")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, "Mining process started")
}

func verifyChainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	isValid := bc.VerifyChain()
	response := map[string]bool{"is_valid": isValid}

	logrus.WithField("is_valid", isValid).Info("Chain verification completed")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getBlockHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		logrus.Warn("Missing block index in request")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing block index")
		return
	}

	index, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		logrus.WithError(err).Warn("Invalid block index")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid block index: %v", err)
		return
	}

	bc.Mu.Lock()
	defer bc.Mu.Unlock()

	if index < 0 || int(index) >= len(bc.Blocks) {
		logrus.WithField("index", index).Warn("Block not found")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Block not found")
		return
	}

	block := bc.Blocks[index]
	logrus.WithField("index", index).Info("Retrieved block")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}
