package config

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
