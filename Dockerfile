FROM golang:latest as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION="docker"
RUN env GOOS=linux CGO_ENABLED=0 go build -o /app/nettica-client -ldflags "-X main.Version=$VERSION" .

FROM alpine:latest
RUN apk add --no-cache wireguard-tools
RUN apk add --no-cache iptables
RUN mkdir -p /etc/nettica
COPY --from=builder /app/nettica-client /usr/bin
COPY wg-hack/wg-quick /usr/bin
RUN chmod +x /usr/bin/wg-quick
USER root
CMD ["/usr/bin/nettica-client"]
