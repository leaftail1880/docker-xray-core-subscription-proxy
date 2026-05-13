package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"xray-balancer/pkg/config"
	"xray-balancer/pkg/fetcher"
	"xray-balancer/pkg/logger"
	"xray-balancer/pkg/updater"
	"xray-balancer/pkg/xray"
)

func main() {
	logger.Info.Println("xray-balancer starting...")

	// Collect URL* env vars
	urlVars := getURLEnvVars()
	if len(urlVars) == 0 {
		logger.Error.Fatal("No URL* environment variables found.")
	}
	logger.Info.Println("Subscription/Direct URLs:")
	for k, v := range urlVars {
		fmt.Printf("  %s = %s\n", k, v)
	}

	// Refresh geo assets (no proxy yet)
	updater.RefreshGeoAssets(false)

	// Build initial config (no proxy)
	coreConfig, err := config.Build(urlVars, false)
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
	fetcher.XrayRunning.Store(true) // enable proxy for future fetches

	// Optional periodic subscription update
	intervalStr := os.Getenv("SUBSCRIPTION_UPDATE_INTERVAL")
	if intervalStr != "" {
		interval, err := updater.ParseCustomDuration(intervalStr)
		if err != nil {
			logger.Error.Printf("Invalid SUBSCRIPTION_UPDATE_INTERVAL %q: %v, will not auto-update.", intervalStr, err)
		} else {
			logger.Info.Printf("Subscription update interval: %v", interval)
			var mu sync.Mutex
			go updater.SubscribeLoop(urlVars, interval, &server, &mu)
		}
	} else {
		logger.Info.Println("No subscription update interval set – running once.")
	}

	// Periodically refresh geo assets (once a day, using proxy)
	go geoRefreshLoop()

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

// geoRefreshLoop runs every 24 hours, with a grace period on first run.
func geoRefreshLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		updater.RefreshGeoAssets(true) // proxy is now available
	}
}
