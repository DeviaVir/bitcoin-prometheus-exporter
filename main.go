package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func requestRPC(url, jsonStr string) map[string]interface{} {
	jsonBytes := []byte(jsonStr)
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		logrus.WithError(err).Error("Error creating request")
		return nil
	}
	request.Header.Set("content-type", "application/json")
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		logrus.WithError(err).Error("Error executing client")
		return nil
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		logrus.WithError(err).Error("Error unmarshalling response")
		return nil
	}

	if v, ok := data["error"].(map[string]interface{}); ok {
		if v["code"] != nil {
			logrus.Debugln(string(body))
			return nil
		}
	}

	return data
}

func loop(client *rpcclient.Client, url, chain, interval, wallet string) {
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
			jsonStr := `{"jsonrpc":"1.0","id":"bitcoin-prometheus-exporter","method":"getbalance","params":["*", 1]}`
			if wallet != "" {
				url = url + "/wallet/" + wallet
			}
			data := requestRPC(url, jsonStr)
			if data == nil {
				loadedWalletFailureCounter.WithLabelValues(chain).Inc()
			} else {
				if v, ok := data["result"].(float64); ok {
					balanceWalletsGauge.WithLabelValues(chain, wallet).Set(v)
				}

				if v, ok := data["result"].(map[string]interface{}); ok {
					balanceWalletsGauge.WithLabelValues(chain, wallet).Set(v["bitcoin"].(float64))
				}
			}
		}

		blockCountGauge.WithLabelValues(chain).Set(blockCount64)
		rawMempoolSizeGauge.WithLabelValues(chain).Set(mempoolSize64)
		connectedPeersGauge.WithLabelValues(chain).Set(peerInfo64)
	}
}

func init() {
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.DebugLevel)

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

	// URL for custom RPC calls.
	url := fmt.Sprintf("http://%s:%s@%s", user, password, host)

	go loop(client, url, chain, interval, wallet)

	http.Handle("/metrics", promhttp.Handler())
	logrus.Info("Now listening on ", listendAddr)
	logrus.Fatal(http.ListenAndServe(listendAddr, nil))
}
