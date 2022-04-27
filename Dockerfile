FROM golang:stretch AS builder

WORKDIR /go/src/ingest-server
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ingest-server .

FROM bash
LABEL org.opencontainers.image.source https://github.com/lantern-org/ingest-server

COPY --from=builder /go/src/ingest-server/ingest-server ./

ENTRYPOINT ["./ingest-server"]
CMD ["--udp-addr=localhost"]
# ENTRYPOINT [ "sleep", "1000000" ]
# no need to have a proxy (yet?)
