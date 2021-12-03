FROM golang:stretch
LABEL org.opencontainers.image.source https://github.com/lantern-org/ingest-server

WORKDIR /go/src/ingest-server
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

ENTRYPOINT ["ingest-server"]
CMD ["--udp-addr=localhost"]
# ENTRYPOINT [ "sleep", "1000000" ]
# no need to have a proxy (yet?)
