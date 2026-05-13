package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/proxy"

	"pira/x2j/models"
	x2jurl "pira/x2j/url"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
)

var (
	cacheDir          = "/etc/xray/cache"
	assetDir          = "/usr/share/xray"
	defaultSocksPort  = 1080
	defaultHTTPPort   = 8080
	updateIntervalEnv = "SUBSCRIPTION_UPDATE_INTERVAL"
	assetUpdateFreq   = 24 * time.Hour
)

var geoFiles = map[string]string{
	"geosite.dat": "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geosite.dat",
	"geoip.dat":   "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geoip.dat",
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("xray-balancer starting...")

	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		log.Fatalf("Failed to create cache dir: %v", err)
	}

	urlVars := getURLEnvVars()
	if len(urlVars) == 0 {
		log.Fatal("No URL* environment variables found.")
	}

	// List URLs being used
	log.Println("Subscription/Direct URLs:")
	for k, v := range urlVars {
		log.Printf("  %s = %s\n", k, v)
	}

	refreshGeoAssets()

	// Initial config build: no proxy yet (Xray not started)
	config, err := buildConfig(urlVars, false)
	if err != nil {
		log.Printf("ERROR: Could not build config: %v", err)
		log.Fatalf("Exiting because no proxy outbounds could be configured.")
	}

	server, err := startXray(config)
	if err != nil {
		log.Fatalf("Failed to start Xray: %v", err)
	}
	log.Println("Xray started successfully.")

	interval := getUpdateInterval()
	if interval > 0 {
		log.Printf("Subscription update interval: %v", interval)
		go periodicUpdate(urlVars, interval, server)
	} else {
		log.Println("No subscription update interval set – running once.")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)
	server.Close()
}

func getURLEnvVars() map[string]string {
	vars := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		if strings.HasPrefix(key, "URL") {
			vars[key] = parts[1]
		}
	}
	return vars
}

// buildConfig creates the final Xray JSON config. useProxy indicates whether to
// try the SOCKS5 proxy for fetching (only after Xray has started).
func buildConfig(urlVars map[string]string, useProxy bool) (*core.Config, error) {
	outbounds, err := gatherAllOutbounds(urlVars, useProxy)
	if err != nil {
		return nil, err
	}
	if len(outbounds) == 0 {
		return nil, fmt.Errorf("no valid proxy outbounds found from all sources")
	}

	// Tag outbounds
	for i := range outbounds {
		outbounds[i]["tag"] = fmt.Sprintf("proxy%d", i+1)
	}

	configMap := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"inbounds": []any{
			socksInboundJSON(defaultSocksPort),
			httpInboundJSON(defaultHTTPPort),
		},
		"outbounds": outbounds,
		"routing":   buildRoutingJSON(len(outbounds)),
		"dns": map[string]any{
			"servers": []string{"1.1.1.1", "8.8.8.8"},
		},
	}

	jsonBytes, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	log.Println("========== Generated Xray Config ==========")
	log.Println(string(jsonBytes))
	log.Println("===========================================")

	conf, err := core.LoadConfig("json", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("LoadConfig: %w", err)
	}
	return conf, nil
}

func socksInboundJSON(port int) map[string]any {
	return map[string]any{
		"tag":      "socks-in",
		"protocol": "socks",
		"port":     port,
		"settings": map[string]any{
			"auth": "noauth",
			"udp":  true,
		},
	}
}

func httpInboundJSON(port int) map[string]any {
	return map[string]any{
		"tag":      "http-in",
		"protocol": "http",
		"port":     port,
	}
}

func buildRoutingJSON(proxyCount int) map[string]any {
	selector := make([]string, proxyCount)
	for i := 0; i < proxyCount; i++ {
		selector[i] = fmt.Sprintf("proxy%d", i+1)
	}
	return map[string]any{
		"domainStrategy": "IPIfNonMatch",
		"rules": []map[string]any{
			{
				"inboundTag":  []string{"socks-in", "http-in"},
				"balancerTag": "balancer",
			},
		},
		"balancers": []map[string]any{
			{
				"tag":      "balancer",
				"selector": selector,
				"strategy": map[string]any{
					"type": "random",
				},
			},
		},
	}
}

func gatherAllOutbounds(urlVars map[string]string, useProxy bool) ([]map[string]any, error) {
	var allOutbounds []map[string]any
	for name, raw := range urlVars {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" {
			log.Printf("[%s] Not a valid URL, skipping: %s", name, raw)
			continue
		}

		switch u.Scheme {
		case "http", "https":
			nodes, err := fetchSubscription(raw, name, useProxy)
			if err != nil {
				log.Printf("[%s] Subscription error: %v", name, err)
				continue
			}
			log.Printf("[%s] Found %d share links in subscription", name, len(nodes))
			for _, node := range nodes {
				out, err := convertNode(node)
				if err != nil {
					log.Printf("[%s] Failed to convert link %q: %v", name, node, err)
					continue
				}
				allOutbounds = append(allOutbounds, out)
			}
		default:
			out, err := convertNode(raw)
			if err != nil {
				log.Printf("[%s] Failed to convert direct link %q: %v", name, raw, err)
				continue
			}
			allOutbounds = append(allOutbounds, out)
		}
	}
	return allOutbounds, nil
}

// fetchSubscription downloads a subscription URL, decodes base64 if needed,
// and returns the list of share links. Caching rules:
// - Cache the raw content under <cacheDir>/<md5(url)>.txt.
// - If fetch fails and cache exists, use cache.
// - If fetch succeeds but yields zero valid links, do NOT update the cache.
// - Otherwise, overwrite the cache.
// useProxy determines whether to attempt fetching through the Xray SOCKS5 proxy first (only when Xray is alive).
func fetchSubscription(subURL, name string, useProxy bool) ([]string, error) {
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%x.txt", md5.Sum([]byte(subURL))))

	raw, err := fetchWithFallback(subURL, useProxy)
	if err != nil {
		log.Printf("[%s] Fetch failed: %v", name, err)
		if _, statErr := os.Stat(cacheFile); os.IsNotExist(statErr) {
			log.Printf("[%s] No cache file exists at %s", name, cacheFile)
			return nil, fmt.Errorf("fetch error %w, no cache available", err)
		}
		log.Printf("[%s] Using cached subscription from %s", name, cacheFile)
		data, readErr := os.ReadFile(cacheFile)
		if readErr != nil {
			return nil, fmt.Errorf("cache read error %v", readErr)
		}
		raw = string(data)
	}

	// Show what was fetched (truncated to avoid flooding logs)
	log.Printf("[%s] Raw subscription content (first 5000 chars):\n%s", name, truncate(raw, 5000))
	// Try base64 decode
	decoded, isB64 := tryDecodeB64(raw)
	if isB64 {
		log.Printf("[%s] Base64 decoded content (first 5000 chars):\n%s", name, truncate(decoded, 5000))
	} else {
		log.Printf("[%s] Content is not base64 or already plain text", name)
	}

	parsed := parseSubscriptionContent(decoded)
	if len(parsed) == 0 {
		log.Printf("[%s] NO valid share links found after parsing.", name)
		log.Printf("[%s] Trying to use old cache if possible.", name)
		if cachedRaw, err := os.ReadFile(cacheFile); err == nil {
			log.Printf("[%s] Old cache exists, reparsing.", name)
			parsed = parseSubscriptionContent(string(cachedRaw))
		} else {
			log.Printf("[%s] No old cache exists either.", name)
		}
	} else {
		log.Printf("[%s] Found %d valid share links, updating cache.", name, len(parsed))
		// Overwrite cache
		if err := os.WriteFile(cacheFile, []byte(raw), 0600); err != nil {
			log.Printf("[%s] Warning: cannot write cache: %v", name, err)
		}
	}
	return parsed, nil
}

// tryDecodeB64 attempts to decode base64; returns (decodedStr, true) if it looks like printable text.
func tryDecodeB64(raw string) (string, bool) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err == nil && isPrintable(string(b)) {
		return string(b), true
	}
	return raw, false
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

var shareLinkSchemes = map[string]bool{
	"vless": true, "vmess": true, "trojan": true, "ss": true, "socks": true,
}

func isShareLink(s string) bool {
	u, err := url.Parse(s)
	return err == nil && shareLinkSchemes[u.Scheme]
}

func parseSubscriptionContent(raw string) []string {
	lines := strings.Split(raw, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if isShareLink(line) {
			out = append(out, line)
		}
	}
	return out
}

// fetchWithFallback tries to fetch the URL. If useProxy is true, it first tries through SOCKS5 proxy.
func fetchWithFallback(rawURL string, useProxy bool) (string, error) {
	if useProxy {
		log.Printf("Trying proxy‑first fetch for %s", rawURL)
		if data, err := fetchHTTP(rawURL, true); err == nil {
			return data, nil
		}
		log.Printf("Proxy fetch failed, falling back to direct connection")
	}
	return fetchHTTP(rawURL, false)
}

func fetchHTTP(rawURL string, useProxy bool) (string, error) {
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
	req.Header.Set("User-Agent", "xray-balancer/1.0")

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

func convertNode(node string) (map[string]any, error) {
	config, err := x2jurl.ParseV2RayURL(node)
	if err != nil {
		return nil, fmt.Errorf("x2j parse: %w", err)
	}

	for _, o := range config.Outbounds {
		if o.Tag == "direct" || o.Tag == "blackhole" {
			continue
		}
		return x2jOutboundToMap(o), nil
	}

	if len(config.Outbounds) > 0 {
		return x2jOutboundToMap(config.Outbounds[0]), nil
	}
	return nil, fmt.Errorf("no outbound found in x2j config")
}

func x2jOutboundToMap(o models.OutboundConfig) map[string]any {
	out := map[string]any{
		"protocol": o.Protocol,
		"tag":      o.Tag,
	}
	if o.Settings != nil {
		out["settings"] = o.Settings
	}
	if o.StreamSettings != nil {
		out["streamSettings"] = o.StreamSettings
	}
	if o.Mux != nil {
		out["mux"] = o.Mux
	}
	return out
}

func startXray(config *core.Config) (*core.Instance, error) {
	server, err := core.New(config)
	if err != nil {
		return nil, err
	}
	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}

func getUpdateInterval() time.Duration {
	val := os.Getenv(updateIntervalEnv)
	if val == "" {
		return 0
	}
	dur, err := parseCustomDuration(val)
	if err != nil {
		log.Printf("Invalid SUBSCRIPTION_UPDATE_INTERVAL %q: %v, will not auto-update.", val, err)
		return 0
	}
	return dur
}

var durationRegex = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(m|h|d)$`)

func parseCustomDuration(s string) (time.Duration, error) {
	matches := durationRegex.FindStringSubmatch(strings.TrimSpace(s))
	if len(matches) != 3 {
		return 0, fmt.Errorf("unsupported format, use e.g. 30m, 1.5h, 2d")
	}
	num, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}
	unit := matches[2]
	switch unit {
	case "m":
		return time.Duration(num * float64(time.Minute)), nil
	case "h":
		return time.Duration(num * float64(time.Hour)), nil
	case "d":
		return time.Duration(num * 24 * float64(time.Hour)), nil
	default:
		return 0, fmt.Errorf("unknown unit %q", unit)
	}
}

func periodicUpdate(urlVars map[string]string, interval time.Duration, oldServer *core.Instance) {
	mu := sync.Mutex{}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		log.Println("Periodic update triggered...")
		// Now Xray is running, so allow proxy fetching
		newConfig, err := buildConfig(urlVars, true)
		if err != nil {
			log.Printf("Update failed: %v", err)
			continue
		}
		log.Println("Restarting Xray with new config...")
		mu.Lock()
		oldServer.Close()
		newSrv, err := startXray(newConfig)
		if err != nil {
			log.Printf("Failed to restart Xray: %v", err)
			log.Fatal("cannot recover from restart failure")
		}
		oldServer = newSrv
		mu.Unlock()
		log.Println("Update complete.")
	}
}

func refreshGeoAssets() {
	for name, downloadURL := range geoFiles {
		path := filepath.Join(assetDir, name)
		info, err := os.Stat(path)
		needsDownload := err != nil
		if !needsDownload {
			needsDownload = time.Since(info.ModTime()) > assetUpdateFreq
		}
		if needsDownload {
			log.Printf("Updating geo asset %s …", name)
			if err := downloadFileWithProxyFallback(path, downloadURL); err != nil {
				log.Printf("Warning: could not update %s: %v", name, err)
			}
		}
	}
}

func downloadFileWithProxyFallback(path, downloadURL string) error {
	// For geo assets we can also try proxy if Xray is running, but here we are called before Xray start,
	// so we'll just use direct fetch. (This is only called at startup, so proxy not yet available.)
	data, err := fetchHTTP(downloadURL, false)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(data), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
