package config

import (
	"os"
	"time"

	"xray-docker/pkg/logger"
)

// buildRoutingAndObservatory returns the routing object and an optional observatory object
// based on the selected balancer strategy.
func buildRoutingAndObservatory(selector []string, strategy string) (map[string]any, map[string]any) {
	balancer := map[string]any{
		"tag":         "balancer",
		"selector":    selector,
		"strategy":    map[string]any{"type": strategy},
		"fallbackTag": "direct", // use direct outbound if all selected are down
	}

	routing := map[string]any{
		"domainStrategy": "IPIfNonMatch",
		"rules": []map[string]any{
			{
				"inboundTag":  []string{"socks-in", "http-in"},
				"balancerTag": "balancer",
			},
		},
		"balancers": []map[string]any{balancer},
	}

	// For strategies that rely on health data, add an observatory
	var observatory map[string]any
	if strategy == "leastPing" || strategy == "leastLoad" {
		intervalStr := os.Getenv("XRAY_OBSERVATORY_INTERVAL")
		if intervalStr == "" {
			intervalStr = "5m"
		}
		interval, err := time.ParseDuration(intervalStr)
		if err != nil {
			logger.Error.Printf("Invalid XRAY_OBSERVATORY_INTERVAL %q, using 5m", intervalStr)
			interval = 5 * time.Minute
		}
		observatory = map[string]any{
			"subjectSelector":   selector,
			"probeInterval":     interval.String(),
			"enableConcurrency": false,
		}
		logger.Info.Printf("Observatory enabled with probe interval %v", interval)
	}

	return routing, observatory
}
