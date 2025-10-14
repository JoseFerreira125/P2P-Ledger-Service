package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/swaggo/http-swagger"
	"github.com/sirupsen/logrus"
)

// VerifyResponse defines the structure for the chain verification response.
type VerifyResponse struct {
	IsValid bool `json:"is_valid"`
}

// NodeInfoResponse defines the structure for the node information response.
type NodeInfoResponse struct {
	PeerID          string   `json:"peer_id"`
	ListenAddresses []string `json:"listen_addresses"`
}

// transactionRequest represents the expected structure of an incoming transaction request.
type transactionRequest struct {
	Data string `json:"data"`
}

type Server struct {
	blockchain *domain.Blockchain
	listenAddr string
}

func NewServer(listenAddr string, blockchain *domain.Blockchain) *Server {
	return &Server{
		listenAddr: listenAddr,
		blockchain: blockchain,
	}
}

func (s *Server) Start() error {
	http.HandleFunc("/transaction", s.transactionHandler)
	http.HandleFunc("/mine", s.mineHandler)
	http.HandleFunc("/chain/verify", s.verifyChainHandler)
	http.HandleFunc("/block/", s.getBlockHandler)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	http.HandleFunc("/node/info", s.nodeInfoHandler) // New endpoint for node info

	logrus.WithField("address", s.listenAddr).Info("Ledger service running")
	return http.ListenAndServe(s.listenAddr, nil)
}

// transactionHandler godoc
// @Summary      Submit a new transaction
// @Description  Adds a new transaction to the mempool and broadcasts it to peers.
// @Tags         Blockchain
// @Accept       json
// @Produce      plain
// @Param        transaction  body      transactionRequest  true  "The transaction data to add"
// @Success      201 Created     {string}  string "Transaction added to mempool and broadcasted"
// @Failure      400          {string}  string "Error reading request body or unmarshalling transaction"
// @Router       /transaction [post]
func (s *Server) transactionHandler(w http.ResponseWriter, r *http.Request) {
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

	var req transactionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logrus.WithError(err).Error("Error unmarshalling transaction request")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error unmarshalling transaction request: %v", err)
		return
	}

	// Create a new transaction using the constructor to ensure a unique timestamp and ID
	transaction := domain.NewTransaction(req.Data)
	transaction.Hash()

	logrus.WithFields(logrus.Fields{
		"transaction_id": transaction.ID,
		"data":           transaction.Data,
		"timestamp":      transaction.Timestamp,
	}).Debug("API: Generated new transaction with ID")

	if err := s.blockchain.AddTransactionToMempool(transaction, false); err != nil {
		logrus.WithError(err).Error("Failed to add transaction to mempool")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to add transaction: %v", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, "Transaction added to mempool and broadcasted")
}

// mineHandler godoc
// @Summary      Mine a new block
// @Description  Instructs the node to mine a new block from transactions in the mempool. This is an asynchronous operation.
// @Tags         Blockchain
// @Produce      plain
// @Success      202  {string}  string "Mining process started"
// @Router       /mine [post]
func (s *Server) mineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	go s.blockchain.AddBlock()

	logrus.Info("Mining process started")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, "Mining process started")
}

// verifyChainHandler godoc
// @Summary      Verify the blockchain
// @Description  Verifies the integrity of the node's entire local blockchain by checking the hash linkage of all blocks.
// @Tags         Blockchain
// @Produce      json
// @Success      200  {object}  VerifyResponse
// @Router       /chain/verify [get]
func (s *Server) verifyChainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	isValid := s.blockchain.VerifyChain()
	response := VerifyResponse{IsValid: isValid}

	logrus.WithField("is_valid", isValid).Info("Chain verification completed")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getBlockHandler godoc
// @Summary      Get a block by index
// @Description  Retrieves a specific block from the local blockchain by its index.
// @Tags         Blockchain
// @Produce      json
// @Param        index  path      int  true  "Block index"
// @Success      200    {object}  domain.Block
// @Failure      400    {string}  string "Missing or invalid block index"
// @Failure      404    {string}  string "Block not found"
// @Router       /block/{index} [get]
func (s *Server) getBlockHandler(w http.ResponseWriter, r *http.Request) {
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

	s.blockchain.Mu.Lock()
	defer s.blockchain.Mu.Unlock()

	if index < 0 || int(index) >= len(s.blockchain.Blocks) {
		logrus.WithField("index", index).Warn("Block not found")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Block not found")
		return
	}

	block := s.blockchain.Blocks[index]
	logrus.WithField("index", index).Info("Retrieved block")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

// nodeInfoHandler godoc
// @Summary      Get node information
// @Description  Retrieves the node's Peer ID and listen addresses.
// @Tags         Node
// @Produce      json
// @Success      200  {object}  NodeInfoResponse
// @Router       /node/info [get]
func (s *Server) nodeInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	peerID := s.blockchain.Node.Host.ID().String()
	listenAddrs := make([]string, 0)
	for _, addr := range s.blockchain.Node.ListenAddress() {
		listenAddrs = append(listenAddrs, addr.String())
	}

	response := NodeInfoResponse{
		PeerID:          peerID,
		ListenAddresses: listenAddrs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
