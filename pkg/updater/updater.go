package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"xray-balancer/pkg/config"
	"xray-balancer/pkg/fetcher"
	"xray-balancer/pkg/logger"
	"xray-balancer/pkg/xray"

	core "github.com/xtls/xray-core/core"
)

var (
	geoFiles = map[string]string{
		"geosite.dat": "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geosite.dat",
		"geoip.dat":   "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geoip.dat",
	}
	assetDir      = "/usr/share/xray" // default
	assetAgeLimit = 24 * time.Hour
)

// SetAssetDir changes the directory where geo files are stored.
func SetAssetDir(dir string) {
	assetDir = dir
	os.MkdirAll(assetDir, 0700)
}

// SubscribeLoop periodically rebuilds config and restarts Xray.
func SubscribeLoop(urlVars map[string]string, interval time.Duration, currentServer **core.Instance, mu *sync.Mutex) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		logger.Info.Println("Subscription update triggered...")
		newConfig, err := config.Build(urlVars) // no proxy param needed, flag is checked internally
		if err != nil {
			logger.Error.Printf("Update failed: %v", err)
			continue
		}
		mu.Lock()
		newSrv, err := xray.Restart(*currentServer, newConfig)
		if err != nil {
			logger.Error.Printf("Failed to restart Xray: %v", err)
			logger.Error.Fatal("Cannot recover from restart failure, exiting.")
		}
		*currentServer = newSrv
		mu.Unlock()
		logger.Info.Println("Subscription update complete.")
	}
}

// RefreshGeoAssets downloads fresh geo data files if they are missing or older than 24h.
// Returns true if any file was actually updated.
func RefreshGeoAssets() bool {
	updated := false
	for name, url := range geoFiles {
		path := filepath.Join(assetDir, name)
		info, err := os.Stat(path)
		needsDownload := err != nil
		if !needsDownload {
			needsDownload = time.Since(info.ModTime()) > assetAgeLimit
		}
		if needsDownload {
			logger.Info.Printf("Updating geo asset %s …", name)
			if downloadGeoFile(path, url) {
				updated = true
			} else {
				logger.Error.Printf("Could not update %s", name)
			}
		}
	}
	return updated
}

// downloadGeoFile downloads the file and returns true if successful (written).
func downloadGeoFile(path, url string) bool {
	data, err := fetcher.FetchWithFallback(url)
	if err != nil {
		logger.Error.Printf("Download failed for %s: %v", path, err)
		return false
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(data), 0644); err != nil {
		logger.Error.Printf("Write failed for %s: %v", path, err)
		return false
	}
	if err := os.Rename(tmpPath, path); err != nil {
		logger.Error.Printf("Rename failed for %s: %v", path, err)
		return false
	}
	return true
}

// ParseCustomDuration handles strings like "30m", "1.5h", "2d".
func ParseCustomDuration(s string) (time.Duration, error) {
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(m|h|d)$`)
	matches := re.FindStringSubmatch(strings.TrimSpace(s))
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
