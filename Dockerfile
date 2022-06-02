# syntax=docker/dockerfile:1.2

FROM --platform=${BUILDPLATFORM} golang:1.17.3-alpine
WORKDIR $GOPATH/src/github.com/b2broker/conductor
ENV CGO_ENABLED=0
COPY . .
RUN go mod download
ARG TARGETOS TARGETARCH
RUN GOOS=${TARGETOS:-"linux"} GOARCH=${TARGETARCH:-"amd64"} \
    go build -ldflags="-w -s" -o /go/bin/conductor ./cmd/conductor

FROM scratch
COPY --from=0 /go/bin/conductor /conductor
ENTRYPOINT ["/conductor"]
