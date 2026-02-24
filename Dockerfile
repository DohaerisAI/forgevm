FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" -o /forgevm ./cmd/forgevm
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /forgevm-agent ./cmd/forgevm-agent

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
COPY --from=builder /forgevm /usr/local/bin/forgevm
COPY --from=builder /forgevm-agent /usr/local/bin/forgevm-agent

EXPOSE 7423

ENTRYPOINT ["forgevm"]
CMD ["serve"]
