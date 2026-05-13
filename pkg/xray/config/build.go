package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"xray-docker/pkg/logger"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
)

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

	// Append a direct (freedom) outbound as a fallback
	directOutbound := map[string]any{
		"protocol": "freedom",
		"tag":      "direct",
	}
	outbounds = append(outbounds, directOutbound)

	// Build selector tags for the balancer (exclude the direct outbound)
	selectorTags := make([]string, len(outbounds)-1)
	for i := 0; i < len(outbounds)-1; i++ {
		selectorTags[i] = fmt.Sprintf("proxy%d", i+1)
	}

	// Determine balancer strategy from environment
	strategy := strings.ToLower(os.Getenv("XRAY_BALANCER_STRATEGY"))
	if strategy == "" {
		strategy = "leastPing"
	}
	// Validate strategy
	validStrategies := map[string]bool{
		"random": true, "roundRobin": true, "leastPing": true, "leastLoad": true,
	}
	if !validStrategies[strategy] {
		logger.Warn.Printf("Invalid balancer strategy %q, falling back to leastPing", strategy)
		strategy = "leastPing"
	}
	logger.Info.Printf("Using balancer strategy: %s", strategy)

	// Build routing, balancer, and optional observatory
	routing, observatory := buildRoutingAndObservatory(selectorTags, strategy)

	configMap := map[string]any{
		"inbounds": []any{
			socksInboundJSON(socksPort),
			httpInboundJSON(httpPort),
		},
		"outbounds": outbounds,
		"routing":   routing,
		"dns": map[string]any{
			"servers": []string{"1.1.1.1", "8.8.8.8"},
		},
	}
	if observatory != nil {
		configMap["observatory"] = observatory
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
