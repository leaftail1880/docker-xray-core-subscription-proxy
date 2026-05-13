# ---- Stage 1: Build Go binary ----
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY main.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /xray-balancer .

# ---- Stage 2: Download Xray geo data files ----
FROM alpine:latest AS assets
RUN apk add --no-cache wget
RUN wget -q -O /geosite.dat \
      https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geosite.dat \
    && wget -q -O /geoip.dat \
      https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geoip.dat

# ---- Stage 3: Minimal runtime image ----
FROM alpine:latest

# Required runtime packages (mirroring teddysun/xray essentials)
RUN apk add --no-cache bash tzdata ca-certificates openssl \
    && mkdir -p /var/log/xray /usr/share/xray /etc/xray/cache

ENV TZ=UTC

# Xray core will look for geo files here
ENV XRAY_LOCATION_ASSET=/usr/share/xray

# Copy geo assets
COPY --from=assets /geosite.dat /usr/share/xray/geosite.dat
COPY --from=assets /geoip.dat /usr/share/xray/geoip.dat

# Copy the Go binary
COPY --from=builder /xray-balancer /usr/local/bin/xray-balancer

VOLUME /etc/xray/cache
EXPOSE 1080 8080

ENTRYPOINT ["xray-balancer"]