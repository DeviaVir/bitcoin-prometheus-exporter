# bitcoin-prometheus-exporter
A prom exporter for the bitcoind/elementsd projects

## Docker

```
docker run -p 9111:9111 -e BITCOIN_RPC_USER=bitcoind -e BITCOIN_RPC_PASS=pass -e BITCOIN_RPC_HOST=mainnet.bitcoin.svc -e HTTP_LISTENADDR=":9112" -it --rm deviavir/bitcoin-prometheus-exporter:latest
```

## Props

Heavily inspired by https://github.com/arjunrn/bitcoin-prometheus-exporter
