#!/bin/sh

bcli="docker-compose exec bitcoin bitcoin-cli -regtest --conf=/root/.bitcoin/conf/bitcoin.conf"
$bcli -rpcwallet=btc -generate 1
$bcli getblockcount

ecli="docker-compose exec elements elements-cli --conf=/root/.elements/conf/elements.conf"
$ecli -rpcwallet=liquid -generate 1
$ecli getblockcount