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
	loadedWalletFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "wallet_errors",
			Help:      "Failures to load wallets",
		}, []string{
			"chain",
		})
	balanceWalletsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "wallet_balance",
			Help:      "The balance on the selected wallet",
		}, []string{
			"chain",
			"wallet",
		})
)

func getEnvDefault(name string, defaultVal string) string {
	envValue, ok := os.LookupEnv(name)
	if ok {
		return envValue
	}
	return defaultVal
}

func loop(client *rpcclient.Client, chain, interval, wallet string) {
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
		if wallet != "UNDEFINED" {
			balance, err := client.GetBalance(wallet)
			if err != nil {
				logrus.Debugln(err)
				loadedWalletFailureCounter.WithLabelValues(chain).Inc()
			} else {
				balanceWalletsGauge.WithLabelValues(chain, wallet).Set(float64(balance))
			}
		}

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
	prometheus.MustRegister(loadedWalletFailureCounter)
	prometheus.MustRegister(balanceWalletsGauge)
}

func main() {
	chain := getEnvDefault("CHAIN", "bitcoin-mainnet")
	user := getEnvDefault("RPC_USER", "")
	password := getEnvDefault("RPC_PASS", "")
	host := getEnvDefault("RPC_HOST", "")
	interval := getEnvDefault("INTERVAL", "15")
	listendAddr := getEnvDefault("HTTP_LISTENADDR", ":9112")
	wallet := getEnvDefault("WALLET", "UNDEFINED") // Do not use `""` as default, default wallet is empty string.
	config := &rpcclient.ConnConfig{
		Host:         host,
		User:         user,
		Pass:         password,
		DisableTLS:   true,
		HTTPPostMode: true,
	}
	client, err := rpcclient.New(config, nil)
	if err != nil {
		logrus.Fatal(err)
	}
	defer client.Shutdown()

	go loop(client, chain, interval, wallet)

	http.Handle("/metrics", promhttp.Handler())
	logrus.Info("Now listening on ", listendAddr)
	logrus.Fatal(http.ListenAndServe(listendAddr, nil))
}
