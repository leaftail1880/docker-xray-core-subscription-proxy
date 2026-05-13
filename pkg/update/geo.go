package update

import (
	"os"
	"path/filepath"
	"time"

	"xray-docker/pkg/logger"
	"xray-docker/pkg/utils"
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
	data, err := utils.FetchWithFallback(url)
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
