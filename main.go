package main

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	blockCountGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "block_count",
			Help:      "The local blockchain length",
		}, []string{
			"chain",
		})
	rawMempoolSizeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "raw_mempool_size",
			Help:      "The number of txes in rawmempool",
		}, []string{
			"chain",
		})
	connectedPeersGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "connected_peers",
			Help:      "The number of connected peers",
		}, []string{
			"chain",
		})
)

func getEnvDefault(name string, defaultVal string) string {
	envValue, ok := os.LookupEnv(name)
	if ok {
		return envValue
	}
	return defaultVal
}

func loop(client *rpcclient.Client, chain string, interval string) {
	intInterval, err := strconv.Atoi(interval)
	if err != nil {
		logrus.Error(err)
		intInterval = 15
	}

	for range time.Tick((time.Second * time.Duration(intInterval))) {
		blockCount, err := client.GetBlockCount()
		if err != nil {
			logrus.Error(err)
		}
		blockCount64 := float64(blockCount)
		mempoolSize, err := client.GetRawMempool()
		if err != nil {
			panic(err)
		}
		mempoolSize64 := float64(len(mempoolSize))
		peerInfo, err := client.GetPeerInfo()
		if err != nil {
			panic(err)
		}
		peerInfo64 := float64(len(peerInfo))

		blockCountGauge.WithLabelValues(chain).Set(blockCount64)
		rawMempoolSizeGauge.WithLabelValues(chain).Set(mempoolSize64)
		connectedPeersGauge.WithLabelValues(chain).Set(peerInfo64)
	}
}

func init() {
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)

	prometheus.MustRegister(blockCountGauge)
	prometheus.MustRegister(rawMempoolSizeGauge)
	prometheus.MustRegister(connectedPeersGauge)
}

func main() {
	chain := getEnvDefault("BITCOIN_CHAIN", "bitcoin-mainnet")
	bitcoinUser := getEnvDefault("BITCOIN_RPC_USER", "")
	bitcoinPass := getEnvDefault("BITCOIN_RPC_PASS", "")
	bitcoinHost := getEnvDefault("BITCOIN_RPC_HOST", "")
	interval := getEnvDefault("BITCOIN_INTERVAL", "15")
	listendAddr := getEnvDefault("HTTP_LISTENADDR", ":9112")
	config := &rpcclient.ConnConfig{
		Host:         bitcoinHost,
		User:         bitcoinUser,
		Pass:         bitcoinPass,
		DisableTLS:   true,
		HTTPPostMode: true,
	}
	client, err := rpcclient.New(config, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	go loop(client, chain, interval)

	http.Handle("/metrics", promhttp.Handler())
	logrus.Info("Now listening on ", listendAddr)
	logrus.Fatal(http.ListenAndServe(listendAddr, nil))
}
