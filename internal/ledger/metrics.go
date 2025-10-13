package ledger

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
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
)
