package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/joseferreira/Immutable-Ledger-Service/internal/domain"
	"github.com/joseferreira/Immutable-Ledger-Service/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	httpSwagger "github.com/swaggo/http-swagger"
)

type VerifyResponse struct {
	IsValid bool `json:"is_valid"`
}

type transactionRequest struct {
	Data string `json:"data"`
}

type Server struct {
	nodeService *service.NodeService
	listenAddr  string
}

func NewServer(listenAddr string, nodeService *service.NodeService) *Server {
	return &Server{
		listenAddr:  listenAddr,
		nodeService: nodeService,
	}
}

func (s *Server) Start() error {
	http.HandleFunc("/transaction", s.transactionHandler)
	http.HandleFunc("/mine", s.mineHandler)
	http.HandleFunc("/chain/verify", s.verifyChainHandler)
	http.HandleFunc("/block/", s.getBlockHandler)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/swagger/", httpSwagger.WrapHandler)
	http.HandleFunc("/node/info", s.nodeInfoHandler)

	logrus.WithField("address", s.listenAddr).Info("Ledger service running")
	return http.ListenAndServe(s.listenAddr, nil)
}

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

	transaction := domain.NewTransaction(req.Data)

	if err := s.nodeService.AddTransaction(transaction); err != nil {
		logrus.WithError(err).Error("Failed to add transaction")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Failed to add transaction: %v", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, "Transaction added to mempool and broadcasted")
}

func (s *Server) mineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	go func() {
		if _, err := s.nodeService.MineBlock(); err != nil {
			logrus.WithError(err).Error("Failed to mine block")
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, "Mining process started")
}

func (s *Server) verifyChainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	isValid := s.nodeService.VerifyChain()
	response := VerifyResponse{IsValid: isValid}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) getBlockHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing block index")
		return
	}

	index, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid block index: %v", err)
		return
	}

	block := s.nodeService.GetBlockByIndex(index)
	if block == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Block not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

func (s *Server) nodeInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	info := s.nodeService.GetNodeInfo()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}
