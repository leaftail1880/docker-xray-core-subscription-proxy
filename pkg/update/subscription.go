package update

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"xray-docker/pkg/logger"
	"xray-docker/pkg/utils"
)

// FetchSubscription retrieves a subscription URL, decodes it, and returns a list of share links.
// It uses the cache: if fetch fails, cached content is used; if fetch succeeds and content changed,
// the cache is updated and the function returns the new links. The change detection is done by comparing
// the raw fetched content with cached content.
func FetchSubscription(subURL, name string) ([]string, error) {
	// Try to fetch fresh content
	freshRaw, fetchErr := utils.FetchWithFallback(subURL)

	// Read cached content
	cachedRaw, cacheErr := utils.ReadCache(subURL)

	// Determine which raw content to use for parsing
	var rawContent string
	var fromCache bool

	if fetchErr == nil {
		rawContent = freshRaw
		fromCache = false
		// Compare with cache to see if changed
		if cacheErr == nil && freshRaw == cachedRaw {
			logger.Info.Printf("[%s] Subscription content unchanged (cached)", name)
		} else if cacheErr == nil && freshRaw != cachedRaw {
			logger.Info.Printf("[%s] Subscription content changed, updating cache", name)
			if err := utils.WriteCache(subURL, freshRaw); err != nil {
				logger.Error.Printf("[%s] Failed to write cache: %v", name, err)
			}
		} else if cacheErr != nil {
			// No cache, write fresh
			logger.Info.Printf("[%s] No cache exists, storing fresh content", name)
			if err := utils.WriteCache(subURL, freshRaw); err != nil {
				logger.Error.Printf("[%s] Failed to write cache: %v", name, err)
			}
		}
	} else {
		// Fetch failed, use cache if available
		if cacheErr == nil {
			logger.Info.Printf("[%s] Fetch failed, using cached subscription", name)
			rawContent = cachedRaw
			fromCache = true
		} else {
			return nil, fmt.Errorf("fetch error %w and no cache available", fetchErr)
		}
	}

	// Log raw content preview
	logger.Info.Printf("[%s] Raw subscription content (first 5000 chars):\n%s", name, truncate(rawContent, 5000))
	decoded, isB64 := tryDecodeB64(rawContent)
	if isB64 {
		logger.Info.Printf("[%s] Base64 decoded content (first 5000 chars):\n%s", name, truncate(decoded, 5000))
	} else {
		logger.Info.Printf("[%s] Content is not base64 or already plain text", name)
	}

	// Parse share links
	parsed := parseSubscriptionContent(decoded)
	if len(parsed) == 0 {
		logger.Warn.Printf("[%s] NO valid share links found after parsing.", name)
		if fromCache && cacheErr == nil {
			// If we are using cache and still no links, maybe cache is corrupt? Try to re-parse raw?
			// For safety, return empty list but don't error.
		}
		return parsed, nil
	}
	logger.Info.Printf("[%s] Found %d valid share links", name, len(parsed))
	return parsed, nil
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
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
		if r > 126 {
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
