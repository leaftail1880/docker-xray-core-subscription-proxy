# ---- Stage 1: Build Go binary ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /xray-docker .

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
    && mkdir -p /var/log/xray /usr/share/xray /usr/share/xray/subscription-cache

ENV TZ=UTC

# Xray core will look for geo files here
ENV XRAY_LOCATION_ASSET=/usr/share/xray

# Copy geo assets
COPY --from=assets /geosite.dat /usr/share/xray/geosite.dat
COPY --from=assets /geoip.dat /usr/share/xray/geoip.dat

# Copy the Go binary
COPY --from=builder /xray-docker /usr/local/bin/xray-docker

VOLUME /usr/share
EXPOSE 1080 8080

ENTRYPOINT ["xray-docker"]