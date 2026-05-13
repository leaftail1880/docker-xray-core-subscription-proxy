package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	updateInterval "xray-docker/pkg/interval"
	"xray-docker/pkg/logger"
	"xray-docker/pkg/update"
	"xray-docker/pkg/utils"
	"xray-docker/pkg/xray"
)

func main() {
	logger.Info.Println("docker-xray-core-subscripion-proxy is starting...")

	// Determine asset & cache paths (Docker vs local)
	setRuntimePaths()

	// Collect URL* env vars
	urlVars := getURLEnvVars()
	if len(urlVars) == 0 {
		logger.Error.Fatal("No URL* environment variables found.")
	}
	logger.Info.Println("Subscription/Direct URLs:")
	for k, v := range urlVars {
		fmt.Printf("  %s = %s\n", k, v)
	}

	// Build initial config (Xray not running yet → no proxy used)
	coreConfig, err := xray.BuildConfig(urlVars)
	if err != nil {
		logger.Error.Printf("Could not build config: %v", err)
		logger.Error.Fatal("Exiting because no proxy outbounds could be configured.")
	}

	// Start Xray
	server, err := xray.Start(coreConfig)
	if err != nil {
		logger.Error.Fatalf("Failed to start Xray: %v", err)
	}
	logger.Info.Println("Xray started successfully.")
	utils.XrayRunning.Store(true) // enable proxy for future fetches

	// Start combined update loop (subscription + geo) with change detection
	intervalStr := os.Getenv("SUBSCRIPTION_UPDATE_INTERVAL")
	if intervalStr == "" {
		intervalStr = "5h"
		logger.Info.Println("SUBSCRIPTION_UPDATE_INTERVAL not set – defaulting to 5h")
	}
	interval, err := utils.ParseCustomDuration(intervalStr)
	if err != nil {
		logger.Error.Printf("Invalid SUBSCRIPTION_UPDATE_INTERVAL %q: %v, will not auto-update.", intervalStr, err)
	} else {
		logger.Info.Printf("Subscription update interval: %v", interval)
		go updateInterval.StartUpdateLoop(urlVars, interval, &server)
	}

	// Wait for shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	sig := <-sigCh
	logger.Info.Printf("Received signal %v, shutting down...", sig)
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

// setRuntimePaths configures asset and cache directories.
func setRuntimePaths() {
	// Determine if we are inside Docker
	inDocker := false
	if _, err := os.Stat("/.dockerenv"); err == nil {
		inDocker = true
	}
	if os.Getenv("container") == "docker" {
		inDocker = true
	}

	if inDocker {
		// Use standard Docker paths
		utils.SetCacheDir("/etc/xray/cache")
		update.SetAssetDir("/usr/share/xray")
		os.Setenv("XRAY_LOCATION_ASSET", "/usr/share/xray")
	} else {
		// Use paths relative to the executable
		execPath, err := os.Executable()
		if err != nil {
			logger.Error.Fatalf("Cannot determine executable path: %v", err)
		}
		baseDir := filepath.Dir(execPath)
		dataDir := filepath.Join(baseDir, "data")
		assetDir := filepath.Join(dataDir, "xray")
		cacheDir := filepath.Join(dataDir, "cache")

		os.MkdirAll(assetDir, 0700)
		os.MkdirAll(cacheDir, 0700)

		utils.SetCacheDir(cacheDir)
		update.SetAssetDir(assetDir)
		os.Setenv("XRAY_LOCATION_ASSET", assetDir)
	}
}
