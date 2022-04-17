FROM golang:stretch
LABEL org.opencontainers.image.source https://github.com/lantern-org/ingest-server

WORKDIR /go/src/ingest-server
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

# todo -- COPY go.mod go.sum; RUN go get; COPY . .; RUN go install

# todo -- go mod vendor

# todo -- golang 1.18

ENTRYPOINT ["ingest-server"]
CMD ["--udp-addr=localhost"]
# ENTRYPOINT [ "sleep", "1000000" ]
# no need to have a proxy (yet?)
