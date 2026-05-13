package config

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"xray-balancer/pkg/fetcher"
	"xray-balancer/pkg/logger"

	"pira/x2j/models"
	x2jurl "pira/x2j/url"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
)

var cacheDir = "/etc/xray/cache" // default, may be overridden

// SetCacheDir updates the cache directory path.
func SetCacheDir(dir string) {
	cacheDir = dir
	os.MkdirAll(cacheDir, 0700)
}

const (
	socksPort = 1080
	httpPort  = 8080
)

// Build creates the final Xray config from a map of URL env vars.
// It never uses the proxy (Xray may not be running when this is first called).
func Build(urlVars map[string]string) (*core.Config, error) {
	outbounds, err := gatherAllOutbounds(urlVars)
	if err != nil {
		return nil, err
	}
	if len(outbounds) == 0 {
		return nil, fmt.Errorf("no valid proxy outbounds found from all sources")
	}

	for i := range outbounds {
		outbounds[i]["tag"] = fmt.Sprintf("proxy%d", i+1)
	}

	configMap := map[string]any{
		"inbounds": []any{
			socksInboundJSON(socksPort),
			httpInboundJSON(httpPort),
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
	logger.Info.Println("========== Generated Xray Config ==========")
	logger.Info.Println(string(jsonBytes))
	logger.Info.Println("===========================================")

	conf, err := serial.LoadJSONConfig(bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("xray LoadConfig: %w", err)
	}
	return conf, nil
}

func socksInboundJSON(port int) map[string]any {
	return map[string]any{
		"tag":      "socks-in",
		"protocol": "socks",
		"port":     port,
		"listen":   "0.0.0.0",
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
		"listen":   "0.0.0.0",
	}
}

func buildRoutingJSON(count int) map[string]any {
	selector := make([]string, count)
	for i := 0; i < count; i++ {
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

func gatherAllOutbounds(urlVars map[string]string) ([]map[string]any, error) {
	var outbounds []map[string]any
	for name, raw := range urlVars {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" {
			logger.Warn.Printf("[%s] Not a valid URL, skipping: %s", name, raw)
			continue
		}
		switch u.Scheme {
		case "http", "https":
			links, err := fetchSubscription(raw, name)
			if err != nil {
				logger.Error.Printf("[%s] Subscription error: %v", name, err)
				continue
			}
			logger.Info.Printf("[%s] Found %d share links in subscription", name, len(links))
			for _, link := range links {
				out, err := convertNode(link)
				if err != nil {
					logger.Error.Printf("[%s] Failed to convert link %q: %v", name, link, err)
					continue
				}
				outbounds = append(outbounds, out)
			}
		default:
			out, err := convertNode(raw)
			if err != nil {
				logger.Error.Printf("[%s] Failed to convert direct link %q: %v", name, raw, err)
				continue
			}
			outbounds = append(outbounds, out)
		}
	}
	return outbounds, nil
}

func fetchSubscription(subURL, name string) ([]string, error) {
	cacheFile := filepath.Join(cacheDir, fmt.Sprintf("%x.txt", md5.Sum([]byte(subURL))))

	raw, err := fetcher.FetchWithFallback(subURL)
	if err != nil {
		logger.Error.Printf("[%s] Fetch failed: %v", name, err)
		if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
			logger.Warn.Printf("[%s] No cache file exists at %s", name, cacheFile)
			return nil, fmt.Errorf("fetch error %w, no cache available", err)
		}
		logger.Info.Printf("[%s] Using cached subscription from %s", name, cacheFile)
		data, err := os.ReadFile(cacheFile)
		if err != nil {
			return nil, fmt.Errorf("cache read error: %w", err)
		}
		raw = string(data)
	}

	logger.Info.Printf("[%s] Raw subscription content (first 5000 chars):\n%s", name, truncate(raw, 5000))
	decoded, isB64 := tryDecodeB64(raw)
	if isB64 {
		logger.Info.Printf("[%s] Base64 decoded content (first 5000 chars):\n%s", name, truncate(decoded, 5000))
	} else {
		logger.Info.Printf("[%s] Content is not base64 or already plain text", name)
	}

	parsed := parseSubscriptionContent(decoded)
	if len(parsed) == 0 {
		logger.Warn.Printf("[%s] NO valid share links found after parsing.", name)
		if cachedRaw, err := os.ReadFile(cacheFile); err == nil {
			logger.Info.Printf("[%s] Trying to use old cache.", name)
			parsed = parseSubscriptionContent(string(cachedRaw))
		} else {
			logger.Warn.Printf("[%s] No old cache exists either.", name)
		}
	} else {
		logger.Info.Printf("[%s] Found %d valid share links, updating cache.", name, len(parsed))
		if err := os.WriteFile(cacheFile, []byte(raw), 0600); err != nil {
			logger.Error.Printf("[%s] Could not write cache: %v", name, err)
		}
	}
	return parsed, nil
}

func tryDecodeB64(raw string) (string, bool) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err == nil && isPrintable(string(b)) {
		return string(b), true
	}
	return raw, false
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
		if r > 126 {
			return false
		}
	}
	return true
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
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

var shareLinkSchemes = map[string]bool{
	"vless": true, "vmess": true, "trojan": true, "ss": true, "socks": true,
}

func isShareLink(s string) bool {
	u, err := url.Parse(s)
	return err == nil && shareLinkSchemes[u.Scheme]
}

func convertNode(node string) (map[string]any, error) {
	conf, err := x2jurl.ParseV2RayURL(node)
	if err != nil {
		return nil, fmt.Errorf("x2j parse: %w", err)
	}
	for _, o := range conf.Outbounds {
		if o.Tag == "direct" || o.Tag == "blackhole" {
			continue
		}
		return x2jOutboundToMap(o), nil
	}
	if len(conf.Outbounds) > 0 {
		return x2jOutboundToMap(conf.Outbounds[0]), nil
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
