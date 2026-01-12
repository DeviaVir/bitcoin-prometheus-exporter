package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

const satoshisPerBTC = 1e8

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
	peerMinFeeRateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "peer_min_fee_rate_sats_per_vbyte",
			Help:      "Minimum feerate each peer will accept (minfeefilter/feefilter), expressed in sats/vbyte",
		}, []string{
			"chain",
			"peer",
			"direction",
			"type",
		})
	// NOTE: peer label carries remote address and can churn; monitor cardinality in Prom.
	peerLowFeeFilterPeersGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "peer_low_fee_filter_peers",
			Help:      "Number of peers whose fee filter is at or below this node's min relay tx fee",
		}, []string{
			"chain",
		})
	peerLowFeeIssueGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "peer_low_fee_filter_issue",
			Help:      "1 when no peers accept transactions at the node's min relay tx fee; otherwise 0",
		}, []string{
			"chain",
		})
	lowestPeerFeeFilterGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "peer_lowest_fee_filter_sats_per_vbyte",
			Help:      "Lowest fee filter advertised by any peer, in sats/vbyte; -1 when no peer fee filters are available",
		}, []string{
			"chain",
		})
	nodeMinRelayFeeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "node_min_relay_fee_rate_sats_per_vbyte",
			Help:      "Node minimum relay tx fee (prefers getmempoolinfo.minrelaytxfee, otherwise relayfee), in sats/vbyte",
		}, []string{
			"chain",
		})
	nodeMempoolMinFeeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "blockchain",
			Subsystem: "collector",
			Name:      "node_mempool_min_fee_rate_sats_per_vbyte",
			Help:      "Current mempool minimum feerate (mempoolminfee), in sats/vbyte",
		}, []string{
			"chain",
		})
)

// peerInfoResult mirrors the subset of fields we need from Bitcoin Core's getpeerinfo RPC.
// It captures fee filters and connection metadata used to populate exporter metrics.
type peerInfoResult struct {
	ID             int32   `json:"id"`
	Addr           string  `json:"addr"`
	AddrLocal      string  `json:"addrlocal,omitempty"`
	Inbound        bool    `json:"inbound"`
	MinFeeFilter   float64 `json:"minfeefilter,omitempty"`
	FeeFilter      float64 `json:"feefilter,omitempty"`
	RelayTxes      bool    `json:"relaytxes"`
	Services       string  `json:"services"`
	Network        string  `json:"network,omitempty"`
	ConnectionType string  `json:"connection_type,omitempty"`
}

// mempoolFeeInfo holds fee-related fields from getmempoolinfo.
type mempoolFeeInfo struct {
	MinRelayTxFee float64 `json:"minrelaytxfee"`
	MempoolMinFee float64 `json:"mempoolminfee"`
	MaxMempool    float64 `json:"maxmempool"`
	Loaded        bool    `json:"loaded"`
}

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
			logrus.Debugln(url)
			logrus.Debugln(jsonStr)
			logrus.Debugln(string(body))
			return nil
		}
	}

	return data
}

// btcPerKVByteToSatPerVByte converts a fee rate expressed in BTC/kvByte to sats/vByte.
func btcPerKVByteToSatPerVByte(fee float64) float64 {
	if fee <= 0 {
		return 0
	}
	return fee * satoshisPerBTC / 1000
}

// feeFilterToSatPerVByte normalizes peer fee filters to sats/vByte.
// Newer nodes expose minfeefilter (BTC/kvB); older ones expose feefilter, which may be BTC/kvB (<1) or sat/kvB (>=1).
// We handle both representations to keep compatibility across Core versions. A feefilter of exactly 1.0 is ambiguous (1 BTC/kvB vs 1 sat/kvB);
// we treat >=1 as sat/kvB, which is acceptable given typical fee ranges.
func feeFilterToSatPerVByte(peer peerInfoResult) float64 {
	switch {
	case peer.MinFeeFilter > 0:
		return btcPerKVByteToSatPerVByte(peer.MinFeeFilter)
	case peer.FeeFilter > 0:
		if peer.FeeFilter < 1 {
			return btcPerKVByteToSatPerVByte(peer.FeeFilter)
		}
		return peer.FeeFilter / 1000
	default:
		return 0
	}
}

func getPeerInfo(client *rpcclient.Client) ([]peerInfoResult, error) {
	raw, err := client.RawRequest("getpeerinfo", nil)
	if err != nil {
		return nil, err
	}

	var peers []peerInfoResult
	if err := json.Unmarshal(raw, &peers); err != nil {
		return nil, err
	}

	return peers, nil
}

func getMempoolFeeInfo(client *rpcclient.Client) (mempoolFeeInfo, error) {
	raw, err := client.RawRequest("getmempoolinfo", nil)
	if err != nil {
		return mempoolFeeInfo{}, err
	}

	var info mempoolFeeInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return mempoolFeeInfo{}, err
	}

	return info, nil
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
			continue
		}
		blockCount64 := float64(blockCount)
		mempoolSize, err := client.GetRawMempool()
		if err != nil {
			logrus.Error(err)
			continue
		}
		mempoolSize64 := float64(len(mempoolSize))
		peerInfo, err := getPeerInfo(client)
		if err != nil {
			logrus.Error(err)
			continue
		}
		networkInfo, err := client.GetNetworkInfo()
		if err != nil {
			logrus.Error(err)
			continue
		}
		mempoolFees, err := getMempoolFeeInfo(client)
		mempoolInfoOK := err == nil
		if err != nil {
			logrus.WithError(err).Warn("Unable to fetch mempool fee info; falling back to relay fee only")
		}

		nodeMinRelaySatVByte := btcPerKVByteToSatPerVByte(networkInfo.RelayFee)
		if mempoolInfoOK && mempoolFees.MinRelayTxFee > 0 {
			nodeMinRelaySatVByte = btcPerKVByteToSatPerVByte(mempoolFees.MinRelayTxFee)
		}
		mempoolMinFeeSatVByte := 0.0
		if mempoolInfoOK {
			mempoolMinFeeSatVByte = btcPerKVByteToSatPerVByte(mempoolFees.MempoolMinFee)
		}

		compatiblePeers := 0
		lowestPeerFeeFilter := math.MaxFloat64

		peerMinFeeRateGauge.Reset()
		for _, peer := range peerInfo {
			peerMinFeeRate := feeFilterToSatPerVByte(peer)
			direction := "outbound"
			if peer.Inbound {
				direction = "inbound"
			}
			peerType := peer.ConnectionType
			if peerType == "" {
				peerType = "unknown"
			}

			peerMinFeeRateGauge.WithLabelValues(chain, peer.Addr, direction, peerType).Set(peerMinFeeRate)

			if peerMinFeeRate > 0 && peerMinFeeRate < lowestPeerFeeFilter {
				lowestPeerFeeFilter = peerMinFeeRate
			}

			if nodeMinRelaySatVByte > 0 && peerMinFeeRate > 0 && peerMinFeeRate <= nodeMinRelaySatVByte {
				compatiblePeers++
			}
		}

		lowestValue := -1.0
		if lowestPeerFeeFilter != math.MaxFloat64 {
			lowestValue = lowestPeerFeeFilter
		}
		lowestPeerFeeFilterGauge.WithLabelValues(chain).Set(lowestValue)
		peerLowFeeFilterPeersGauge.WithLabelValues(chain).Set(float64(compatiblePeers))
		peerLowFeeIssueGauge.WithLabelValues(chain).Set(0)
		if nodeMinRelaySatVByte > 0 && len(peerInfo) > 0 && compatiblePeers == 0 {
			peerLowFeeIssueGauge.WithLabelValues(chain).Set(1)
		}

		nodeMinRelayFeeGauge.WithLabelValues(chain).Set(nodeMinRelaySatVByte)
		if mempoolInfoOK {
			nodeMempoolMinFeeGauge.WithLabelValues(chain).Set(mempoolMinFeeSatVByte)
		}
		if wallet != "UNDEFINED" {
			jsonStr := `{"jsonrpc":"1.0","id":"bitcoin-prometheus-exporter","method":"getbalance","params":["*", 1]}`
			walletUrl := url
			if wallet != "" {
				walletUrl = fmt.Sprintf("%s/wallet/%s", url, wallet)
			}
			data := requestRPC(walletUrl, jsonStr)
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
		connectedPeersGauge.WithLabelValues(chain).Set(float64(len(peerInfo)))
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
	prometheus.MustRegister(peerMinFeeRateGauge)
	prometheus.MustRegister(peerLowFeeFilterPeersGauge)
	prometheus.MustRegister(peerLowFeeIssueGauge)
	prometheus.MustRegister(lowestPeerFeeFilterGauge)
	prometheus.MustRegister(nodeMinRelayFeeGauge)
	prometheus.MustRegister(nodeMempoolMinFeeGauge)
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
