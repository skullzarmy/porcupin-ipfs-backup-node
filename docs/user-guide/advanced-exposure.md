# Advanced: Internet Exposure

> ⚠️ **WARNING:** Exposing Porcupin to the internet is NOT recommended and comes with significant security risks. This guide is for advanced users who understand the implications. **You are responsible for securing your deployment.**

## Overview

By default, Porcupin's API server only accepts connections from private IP ranges (LAN). This document explains how to expose Porcupin to the internet if you have a legitimate need (e.g., managing your home server while traveling).

**Before proceeding, consider alternatives:**

-   **VPN**: Connect to your home network via WireGuard, Tailscale, or OpenVPN
-   **SSH Tunnel**: `ssh -L 8085:localhost:8085 your-server`
-   **Cloudflare Tunnel**: Zero-trust access without opening ports

These approaches are significantly safer than direct internet exposure.

---

## Risk Assessment

### What You're Exposing

When you expose Porcupin to the internet, attackers can:

1. **Attempt authentication bypass** - Brute force your API token
2. **Exploit potential vulnerabilities** - Any bugs in the API become attack vectors
3. **DoS your server** - Rate limiting helps but doesn't prevent resource exhaustion
4. **Enumerate your NFT collection** - If compromised, your wallet addresses and NFT holdings are visible

### Mitigations We Provide

-   **bcrypt-hashed tokens** - Timing-safe comparison, resistant to rainbow tables
-   **Rate limiting** - 100 requests/minute per IP by default
-   **TLS support** - Encrypted connections when configured
-   **No destructive operations** - API cannot delete your IPFS data or modify wallets' private keys

### Mitigations You Must Provide

-   **Strong firewall rules** - Restrict source IPs if possible
-   **Fail2ban or similar** - Block IPs after failed auth attempts
-   **Valid TLS certificates** - Self-signed certs are vulnerable to MITM
-   **Monitoring** - Watch for suspicious activity
-   **Regular updates** - Keep Porcupin updated for security patches

---

## Configuration

### 1. Enable Public Access

Add the `--allow-public` flag to permit non-private IPs:

```bash
porcupin-server --serve --allow-public --api-port 8085
```

Or in your systemd service:

```ini
[Service]
ExecStart=/usr/local/bin/porcupin-server --serve --allow-public
```

### 2. Configure TLS (Required for Internet)

**Never expose Porcupin over plain HTTP on the internet.** Your token will be transmitted in cleartext.

#### Option A: Direct TLS

Provide certificate and key files:

```bash
porcupin-server --serve --allow-public \
  --tls-cert /etc/porcupin/cert.pem \
  --tls-key /etc/porcupin/key.pem
```

You can obtain certificates from:

-   **Let's Encrypt** (free, automated via certbot)
-   **Your domain registrar** (if you have a domain)
-   **Self-signed** (not recommended for internet, causes browser warnings)

#### Option B: Reverse Proxy (Recommended)

Use nginx, Caddy, or Traefik to handle TLS termination:

**Caddy (automatic HTTPS):**

```
porcupin.yourdomain.com {
    reverse_proxy localhost:8085
}
```

**nginx:**

```nginx
server {
    listen 443 ssl http2;
    server_name porcupin.yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/porcupin.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/porcupin.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8085;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### 3. Firewall Configuration

Even with `--allow-public`, restrict access where possible:

**UFW (Ubuntu):**

```bash
# Allow from specific IP only
sudo ufw allow from 203.0.113.50 to any port 8085

# Or allow from anywhere (less secure)
sudo ufw allow 8085/tcp
```

**iptables:**

```bash
# Allow from specific IP
iptables -A INPUT -p tcp --dport 8085 -s 203.0.113.50 -j ACCEPT
iptables -A INPUT -p tcp --dport 8085 -j DROP
```

### 4. Fail2ban Integration

Create `/etc/fail2ban/filter.d/porcupin.conf`:

```ini
[Definition]
failregex = ^.*API auth failed from <HOST>.*$
ignoreregex =
```

Create `/etc/fail2ban/jail.d/porcupin.conf`:

```ini
[porcupin]
enabled = true
port = 8085
filter = porcupin
logpath = /var/log/porcupin/server.log
maxretry = 5
bantime = 3600
```

> **Note:** You'll need to configure Porcupin to log to a file and include auth failures with IP addresses for fail2ban to work.

---

## Router/NAT Configuration

To access Porcupin from the internet, you'll need to configure port forwarding on your router:

1. **Find your server's local IP** (e.g., `192.168.1.100`)
2. **Log into your router** (usually `192.168.1.1` or `192.168.0.1`)
3. **Find port forwarding settings** (often under "NAT", "Virtual Servers", or "Port Forwarding")
4. **Create a rule:**

    - External port: 8085 (or your chosen port)
    - Internal IP: Your server's IP
    - Internal port: 8085
    - Protocol: TCP

5. **Find your public IP:** Visit https://whatismyip.com

You can now connect using: `https://YOUR_PUBLIC_IP:8085`

### Dynamic DNS

If your ISP assigns dynamic IPs, use a Dynamic DNS service:

-   **DuckDNS** (free)
-   **No-IP** (free tier available)
-   **Cloudflare** (if you own a domain)

This gives you a stable hostname like `myporcupin.duckdns.org`.

---

## Connecting from the GUI

In the Porcupin desktop app:

1. Go to **Settings** → **Remote Server**
2. Enter your public IP or dynamic DNS hostname
3. Enter port `8085` (or your configured port)
4. Check **Use TLS** (required for internet)
5. Enter your API token
6. Click **Test Connection**

If using a self-signed certificate, the connection may fail due to certificate validation. This is expected - use a proper certificate from Let's Encrypt or a trusted CA.

---

## Security Checklist

Before exposing to the internet, verify:

-   [ ] TLS is configured and working
-   [ ] Certificate is from a trusted CA (not self-signed)
-   [ ] `--allow-public` flag is set
-   [ ] Firewall is configured
-   [ ] API token is strong (the auto-generated token is sufficient)
-   [ ] You have a plan for monitoring access
-   [ ] You understand and accept the risks

---

## Troubleshooting

### Connection Refused

-   Check firewall rules allow the port
-   Verify port forwarding on your router
-   Ensure Porcupin is running with `--serve --allow-public`

### Certificate Errors

-   Verify the certificate path is correct
-   Check certificate hasn't expired: `openssl x509 -enddate -noout -in cert.pem`
-   Ensure the certificate matches your domain/IP

### Authentication Failures

-   Double-check your API token
-   Verify you're using `Bearer` authentication: `Authorization: Bearer your_token`
-   Check server logs for auth failure messages

### Slow Connections

-   Your home upload speed affects remote access performance
-   Consider if a VPN might provide better performance
-   Large asset lists may take time to transfer

---

## Alternatives (Recommended)

### Tailscale (Easiest)

1. Install Tailscale on your server and client devices
2. Run Porcupin normally (no `--allow-public` needed)
3. Connect using the Tailscale IP (e.g., `100.x.y.z:8085`)

Tailscale provides end-to-end encryption, NAT traversal, and works without opening ports.

### WireGuard VPN

1. Set up a WireGuard server on your network
2. Configure clients with WireGuard
3. Connect to Porcupin using its LAN IP through the VPN

### SSH Tunnel

```bash
# On your client machine
ssh -L 8085:localhost:8085 user@your-server

# Then connect to localhost:8085 in the Porcupin app
```

This encrypts traffic through SSH without exposing ports.

---

## Final Warning

Internet exposure significantly increases your attack surface. The Porcupin team:

-   **Does not recommend** internet exposure for most users
-   **Cannot guarantee** the API is free of vulnerabilities
-   **Will not be responsible** for any security incidents

If you must expose Porcupin to the internet, use a VPN or tunnel instead. If that's not possible, follow all the security recommendations in this guide and monitor your server closely.
