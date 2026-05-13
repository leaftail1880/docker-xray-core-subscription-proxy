package utils

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
)

// CacheDir is the directory where subscription cache files are stored.
// It is set by SetCacheDir from the config package.
var CacheDir = "/etc/xray/cache"

// SetCacheDir updates the cache directory.
func SetCacheDir(dir string) {
	CacheDir = dir
	os.MkdirAll(CacheDir, 0700)
}

// CachePath returns the file path for a given subscription URL.
func CachePath(subURL string) string {
	hash := md5.Sum([]byte(subURL))
	return filepath.Join(CacheDir, fmt.Sprintf("%x.txt", hash))
}

// ReadCache reads cached content for a subscription URL.
// Returns empty string and no error if cache does not exist.
func ReadCache(subURL string) (string, error) {
	path := CachePath(subURL)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// WriteCache stores content for a subscription URL.
func WriteCache(subURL, content string) error {
	path := CachePath(subURL)
	return os.WriteFile(path, []byte(content), 0600)
}

// CacheExists checks if a cache file exists.
func CacheExists(subURL string) bool {
	_, err := os.Stat(CachePath(subURL))
	return err == nil
}
