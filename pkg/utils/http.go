package utils

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
	"xray-docker/pkg/logger"

	"golang.org/x/net/proxy"
)

// XrayRunning is set to true once Xray is successfully started.
var XrayRunning atomic.Bool

// FetchHTTP performs an HTTP GET, optionally through a local SOCKS5 proxy.
func FetchHTTP(rawURL string, useProxy bool) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	if useProxy {
		dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:1080", nil, proxy.Direct)
		if err != nil {
			return "", fmt.Errorf("socks5 dialer: %w", err)
		}
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "xray-docker/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// FetchWithFallback tries to fetch the URL. It uses the SOCKS5 proxy only if
// XrayRunning is true; otherwise it fetches directly.
func FetchWithFallback(rawURL string) (string, error) {
	if XrayRunning.Load() {
		logger.Debug.Printf("Trying proxy-first fetch for %s", rawURL)
		data, err := FetchHTTP(rawURL, true)
		if err == nil {
			return data, nil
		}
		logger.Warn.Printf("Proxy fetch failed, falling back to direct")
	}
	return FetchHTTP(rawURL, false)
}
