package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

func getEnv(name string) string {
	envValue, ok := os.LookupEnv(name)
	if ok {
		return envValue
	}
	panic(fmt.Sprintf("Missing environment variable: %s", name))
}

func getEnvDefault(name string, defaultVal string) string {
	envValue, ok := os.LookupEnv(name)
	if ok {
		return envValue
	}
	return defaultVal
}

func setGauge(name string, help string, callback func() float64) {
	gaugeFunc := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "bitcoind",
		Subsystem: "blockchain",
		Name:      name,
		Help:      help,
	}, callback)
	prometheus.MustRegister(gaugeFunc)
}

func main() {
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
	liquidUser := getEnv("BITCOIN_RPC_USER")
	liquidPass := getEnv("BITCOIN_RPC_PASS")
	liquidHost := getEnv("BITCOIN_RPC_HOST")
	listendAddr := getEnvDefault("HTTP_LISTENADDR", ":9112")
	config := &rpcclient.ConnConfig{
		Host:         liquidHost,
		User:         liquidUser,
		Pass:         liquidPass,
		DisableTLS:   true,
		HTTPPostMode: true,
	}
	client, err := rpcclient.New(config, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()
	setGauge("block_count", "The local blockchain length", func() float64 {
		blockCount, err := client.GetBlockCount()
		if err != nil {
			panic(err)
		}
		return float64(blockCount)
	})
	setGauge("raw_mempool_size", "The number of txes in rawmempool", func() float64 {
		hashes, err := client.GetRawMempool()
		if err != nil {
			panic(err)
		}
		return float64(len(hashes))
	})
	setGauge("connected_peers", "The number of connected peers", func() float64 {
		peerInfo, err := client.GetPeerInfo()
		if err != nil {
			panic(err)
		}
		return float64(len(peerInfo))
	})
	http.Handle("/metrics", promhttp.Handler())
	logrus.Info("Now listening on ", listendAddr)
	logrus.Fatal(http.ListenAndServe(listendAddr, nil))
}
