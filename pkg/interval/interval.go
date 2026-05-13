package interval

import (
	"time"

	"xray-docker/pkg/logger"
	"xray-docker/pkg/update"
	"xray-docker/pkg/utils"
	"xray-docker/pkg/xray"

	"github.com/xtls/xray-core/core"
)

// StartUpdateLoop periodically checks for subscription changes and geo updates.
// It only triggers a restart if either the subscription content has changed
// (compared to cached raw content) or geo assets were updated.
func StartUpdateLoop(urlVars map[string]string, interval time.Duration, serverPtr **core.Instance) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Also run geo refresh on a separate 24h ticker, but we'll handle it within the same loop
	// to avoid overlapping restarts. We'll just call RefreshGeoAssets each time and track
	// whether it updated. Geo assets have their own age check (24h), so calling it more often
	// is fine (it will quickly skip).
	for range ticker.C {
		logger.Info.Println("Checking for updates...")
		needRestart := false

		// Check subscription changes
		if checkSubscriptionsChanged(urlVars) {
			logger.Info.Println("Subscription content changed, will rebuild config")
			needRestart = true
		}

		// Check geo assets
		if update.RefreshGeoAssets() {
			logger.Info.Println("Geo assets updated, will restart Xray")
			needRestart = true
		}

		if needRestart {
			logger.Info.Println("Rebuilding config and restarting Xray")
			newConfig, err := xray.BuildConfig(urlVars)
			if err != nil {
				logger.Error.Printf("Failed to rebuild config: %v", err)
				continue
			}
			newSrv, err := xray.Restart(*serverPtr, newConfig)
			if err != nil {
				logger.Error.Printf("Failed to restart Xray: %v", err)
				continue
			}
			*serverPtr = newSrv
			logger.Info.Println("Update complete, Xray restarted")
		} else {
			logger.Info.Println("No changes detected, skipping restart")
		}
	}
}

// checkSubscriptionsChanged fetches fresh subscription content for each URL var
// and compares it with the cached raw content. Returns true if any subscription
// content has changed.
func checkSubscriptionsChanged(urlVars map[string]string) bool {
	changed := false
	for name, rawURL := range urlVars {
		// Only check URLs that are http/https (subscriptions)
		// For direct proxy URLs, there's no subscription to check.
		// We'll skip non-http schemes.
		// Simple check: if scheme is http or https, treat as subscription.
		// Actually we need to parse URL. We'll do quick prefix.
		if len(rawURL) > 4 && (rawURL[:4] == "http" || rawURL[:5] == "https") {
			// Fetch fresh content (without using cache) and compare
			freshRaw, err := utils.FetchWithFallback(rawURL)
			if err != nil {
				logger.Warn.Printf("[%s] Failed to fetch subscription for change detection: %v", name, err)
				continue
			}
			cachedRaw, _ := utils.ReadCache(rawURL)
			if freshRaw != cachedRaw {
				logger.Info.Printf("[%s] Subscription content changed (different from cache)", name)
				changed = true
				// Update cache immediately so next check doesn't report false positive
				if err := utils.WriteCache(rawURL, freshRaw); err != nil {
					logger.Error.Printf("[%s] Failed to update cache after change: %v", name, err)
				}
			}
		}
	}
	return changed
}
