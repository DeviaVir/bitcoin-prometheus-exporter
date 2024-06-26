FROM golang:1.22-alpine AS builder

ENV GO111MODULE="on"
ENV CGO_ENABLED="0"

RUN apk add --update git

RUN mkdir -p /go/src/github.com/DeviaVir/bitcoin-prometheus-exporter

COPY . /go/src/github.com/DeviaVir/bitcoin-prometheus-exporter

RUN cd /go/src/github.com/DeviaVir/bitcoin-prometheus-exporter \
 && go mod vendor \
 && go build \
      -mod vendor \
      -o /go/bin/bitcoin-prometheus-exporter

FROM gcr.io/distroless/base-debian11
COPY --from=builder /go/bin/bitcoin-prometheus-exporter /
CMD ["/bitcoin-prometheus-exporter"]
