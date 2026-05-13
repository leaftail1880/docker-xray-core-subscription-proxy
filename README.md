# 🚀 XRay Docker – Zero‑Config proxy with subscription support

[![License](https://img.shields.io/github/license/leaftail1880/docker-xray-core-subscription-proxy?style=flat-square)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/leaftail1880/docker-xray-core-subscription-proxy?style=flat-square)](go.mod)

**xray-docker** automatically turns your proxy subscriptions urls into a running VPN without any config file. Just set your URLs and let it handle updates, health checks, and failover. Perfect as proxy for other containers.

## ✨ Features

- 📡 **Subscription auto‑update** Fetches your proxy lists every few hours (default 5h), automatically rebuilds Xray config and restarts proxy.
- 🌍 **Geo‑asset refresh** Downloads fresh `geoip.dat` / `geosite.dat` once a day for accurate routing.
- 🔀 **Configurable load balancing** Random, round‑robin, least‑ping, or least‑load – choose with a single env var.
- 🛡️ **Fallback to direct** If all proxies go down, traffic still flows through your local internet (no blackhole).
- 💾 **Subscription caching** When the subscription server is unreachable, the last cached list is used (no downtime).
- 🧩 **Supports all common protocols** VMess, VLESS, Trojan, Shadowsocks, SOCKS – everything Xray supports.
- 🔄 **Proxy‑aware fetching** After Xray starts, subscription fetches go through the existing proxy.

## 🚀 Quick Start

### Minimal example – internal proxy for other containers

Create a `docker-compose.yml`:

```yaml
services:
  vpn:
    image: ghcr.io/leaftail1880/docker-xray-core-subscription-proxy:latest
    restart: unless-stopped
    environment:
      - URL1=https://your-subscription-link
      - URL2=vless://example@example.com:443?encryption=none&security=tls&sni=example.com&fp=chrome&type=tcp&headerType=none

  my-app:
    image: your-app
    environment:
      - PROXY=socks5://vpn:1080/
    depends_on:
      - vpn
```

> The `vpn` service does **not** expose any ports – it is only reachable via Docker’s internal network. Your app uses the `PROXY` environment variable to route traffic through it.

---

### Full example – with custom update interval, balancer strategy, and host‑mounted cache

```yaml
services:
  vpn:
    image: ghcr.io/leaftail1880/docker-xray-core-subscription-proxy:latest
    container_name: xray-docker
    restart: unless-stopped
    environment:
      - URL1=https://my-subscription.com/list
      - URL2=vless://user@example.com:443?...
      - URL3=trojan://password@trojan-server.com:443?...
      - SUBSCRIPTION_UPDATE_INTERVAL=2h
      - XRAY_BALANCER_STRATEGY=leastPing
      - XRAY_OBSERVATORY_INTERVAL=30s
    volumes:
      - /host/path/to/cache:/usr/share/xray # bind mount for persistent geoip/geosite/subscription cache
    # No ports – internal use only

  another-app:
    image: another-service
    environment:
      - HTTP_PROXY=http://vpn:8080
      - HTTPS_PROXY=http://vpn:8080
      - SOCKS_PROXY=socks5://vpn:1080
    depends_on:
      - vpn
```

---

## 🔧 Environment Variables

| Variable                       | Description                                                                                                                          | Default     |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------ | ----------- |
| `URL1`, `URL2`, … `URLN`       | Subscription URL (http/https) **or** a single share link (vmess://, vless://, trojan://, ss://, socks://). The prefix must be `URL`. | (required)  |
| `SUBSCRIPTION_UPDATE_INTERVAL` | How often to check for changes. Format: `30m`, `1.5h`, `2d`.                                                                         | `5h`        |
| `XRAY_BALANCER_STRATEGY`       | Load‑balancing algorithm: `random`, `roundRobin`, `leastPing`, `leastLoad`.                                                          | `leastPing` |
| `XRAY_OBSERVATORY_INTERVAL`    | When using `leastPing` or `leastLoad`, how often Xray probes each proxy.                                                             | `5m`        |

> 💡 `leastPing` (default) automatically picks the fastest proxy – great for general browsing.  
> `leastLoad` uses more advanced health metrics (requires observatory).

---

## 🧠 How It Works Under the Hood

1. **Initialisation**
   - Reads all environment variables starting with `URL`.
   - For each `http://` or `https://` URL, it fetches the subscription content (decodes base64 if needed) and extracts all share links.
   - Direct share links are used as‑is.

2. **Xray Configuration**
   - Builds a complete Xray JSON config with **one outbound per proxy** + a `direct` (freedom) outbound.
   - Creates a balancer (strategy configurable) that selects among all proxies.
   - If health checks are needed (`leastPing`, `leastLoad`), an **observatory** is added to probe them periodically.
   - The config is fed to Xray core.

3. **Auto‑Update Loop**
   - Every `SUBSCRIPTION_UPDATE_INTERVAL`, the container re‑fetches **all subscriptions** and compares raw content with the cache.
   - Also checks if `geoip.dat` / `geosite.dat` are older than 24h, and updates them if needed.
   - Only if **something changed** (subscription content or geo files) does it rebuild the config and restart Xray – otherwise it does nothing.
   - Subscription cache is stored in `/usr/share/xray/subscription-cache/` (persist it with a volume).

4. **Fallback & Reliability**
   - If a subscription server fails, the last cached list is used.
   - If **all** proxies become unreachable (according to the observatory), the balancer falls back to the `direct` outbound – your internet keeps working.

---

## 🧪 Local Build (with submodule)

Because the project uses the [x2j](https://github.com/code3-dev/x2j) submodule to parse share links, you need to initialise it before building.

```bash
git clone --recursive https://github.com/leaftail1880/docker-xray-core-subscription-proxy.git
cd docker-xray-core-subscription-proxy

# Build the Docker image
docker build -t xray-docker:local .
```

Alternatively, build the standalone binary (for debugging):

```bash
go mod download
go build
```

---

## 🙏 Credits & Acknowledgements

This project stands on these amazing open‑source projects:

- **[Xray-core](https://github.com/XTLS/Xray-core)** – The core proxy engine.
- **[x2j](https://github.com/code3-dev/x2j)** – Converts share links to Xray outbound configs.
- **[Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)** – GeoIP and geosite databases.

---

## 🤝 Contributing

Issues and pull requests are welcome! Please make sure to test any changes with the provided submodule.

---

## 📄 License

[MIT](LICENSE)
