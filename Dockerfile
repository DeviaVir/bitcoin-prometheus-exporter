FROM golang:alpine AS builder

ENV GO111MODULE="on"
ENV CGO_ENABLED="0"

RUN mkdir -p /go/src/github.com/DeviaVir/bitcoin-prometheus-exporter

COPY . /go/src/github.com/DeviaVir/bitcoin-prometheus-exporter

RUN cd /go/src/github.com/DeviaVir/bitcoin-prometheus-exporter \
 && go mod vendor \
 && go build \
      -mod vendor \
      -o /go/bin/bitcoin-prometheus-exporter

FROM alpine
COPY --from=builder /go/bin/bitcoin-prometheus-exporter /usr/local/bin/bitcoin-prometheus-exporter
CMD ["/usr/local/bin/bitcoin-prometheus-exporter"]
