package domain

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PeerCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "p2p_peer_count",
		Help: "The current number of connected peers.",
	})

	BlockHeight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "block_height",
		Help: "The current block height of the blockchain.",
	})

	MempoolSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "mempool_size",
		Help: "The number of transactions in the mempool.",
	})

	MiningTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "mining_time_seconds",
		Help: "The time it takes to mine a new block.",
	})

	TransactionsPerBlock = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "transactions_per_block",
		Help:    "The number of transactions included in each new block.",
		Buckets: prometheus.LinearBuckets(0, 5, 10),
	})
)
