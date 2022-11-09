# docker

Stands up a regtest bitcoin and elements, so that we may test against real
life applications.

Guided by 
https://www.willianantunes.com/blog/2022/04/bitcoin-node-with-regtest-mode-using-docker/

## Usage

```
$ docker-compose up -d
```

This starts up tor (for elements), elements and bitcoin in regtest.

### Wallets

Create wallets by running `./create-wallet.sh`, this creates a `btc` wallet in bitcoin
and a `liquid` wallet in elements.

### Mining

Mine transactions by running `./mine.sh`.