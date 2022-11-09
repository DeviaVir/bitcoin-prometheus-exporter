#!/bin/sh

bcli="docker-compose exec bitcoin bitcoin-cli -regtest --conf=/root/.bitcoin/conf/bitcoin.conf"
if $bcli -rpcwallet=btc getbalance; then
    echo "btc wallet already exists"
else
    $bcli createwallet "btc"
    BTC_ADDRESS=`$bcli -rpcwallet=btc getnewaddress`
    $bcli generatetoaddress 101 $BTC_ADDRESS
    $bcli -rpcwallet=btc getbalance
fi

ecli="docker-compose exec elements elements-cli --conf=/root/.elements/conf/elements.conf"
if $ecli -rpcwallet=liquid getbalance; then
    echo "liquid wallet already exists"
else
    $ecli createwallet "liquid"
    ELEMENTS_ADDRESS=`$ecli -rpcwallet=liquid getnewaddress`
    $ecli generatetoaddress 101 $ELEMENTS_ADDRESS
    $ecli -rpcwallet=liquid getbalance
fi