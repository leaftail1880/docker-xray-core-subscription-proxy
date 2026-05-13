package config

import (
	"fmt"
	"net/url"

	"xray-docker/pkg/logger"
	"xray-docker/pkg/update"

	"pira/x2j/models"
	x2jurl "pira/x2j/url"
)

func gatherAllOutbounds(urlVars map[string]string) ([]map[string]any, error) {
	var outbounds []map[string]any
	for name, raw := range urlVars {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" {
			logger.Warn.Printf("[%s] Not a valid URL, skipping: %s", name, raw)
			continue
		}
		switch u.Scheme {
		case "http", "https":
			links, err := update.FetchSubscription(raw, name)
			if err != nil {
				logger.Error.Printf("[%s] Subscription error: %v", name, err)
				continue
			}
			logger.Info.Printf("[%s] Found %d share links in subscription", name, len(links))
			for _, link := range links {
				out, err := convertNode(link)
				if err != nil {
					logger.Error.Printf("[%s] Failed to convert link %q: %v", name, link, err)
					continue
				}
				outbounds = append(outbounds, out)
			}
		default:
			out, err := convertNode(raw)
			if err != nil {
				logger.Error.Printf("[%s] Failed to convert direct link %q: %v", name, raw, err)
				continue
			}
			outbounds = append(outbounds, out)
		}
	}
	return outbounds, nil
}

// convertNode converts a single share link to Xray outbound map.
func convertNode(node string) (map[string]any, error) {
	conf, err := x2jurl.ParseV2RayURL(node)
	if err != nil {
		return nil, fmt.Errorf("x2j parse: %w", err)
	}
	for _, o := range conf.Outbounds {
		if o.Tag == "direct" || o.Tag == "blackhole" {
			continue
		}
		return x2jOutboundToMap(o), nil
	}
	if len(conf.Outbounds) > 0 {
		return x2jOutboundToMap(conf.Outbounds[0]), nil
	}
	return nil, fmt.Errorf("no outbound found in x2j config")
}

func x2jOutboundToMap(o models.OutboundConfig) map[string]any {
	out := map[string]any{
		"protocol": o.Protocol,
		"tag":      o.Tag,
	}
	if o.Settings != nil {
		out["settings"] = o.Settings
	}
	if o.StreamSettings != nil {
		out["streamSettings"] = o.StreamSettings
	}
	if o.Mux != nil {
		out["mux"] = o.Mux
	}
	return out
}
