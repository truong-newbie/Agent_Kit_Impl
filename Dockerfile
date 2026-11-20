FROM golang:1.27.4 AS builder

ARG VERSION=main
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY schemas ./schemas

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-s -w -X github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo.Version=${VERSION} -X github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo.Commit=${COMMIT} -X github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo.Date=${DATE}" \
    -o /dtkit ./cmd/dtkit

FROM alpine:3.21
RUN apk add --no-cache git ca-certificates
COPY --from=builder /dtkit /usr/local/bin/dtkit
ENTRYPOINT ["/usr/local/bin/dtkit"]
