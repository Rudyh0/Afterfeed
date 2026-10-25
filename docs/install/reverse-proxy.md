# Reverse proxy and HTTPS

Afterfeed works on plain HTTP inside your own network. To use it from outside, put it behind a reverse proxy that adds HTTPS, or use a VPN.

## Requirements for the proxy

- **Request size:** uploads are sent in chunks of 16 MB. Allow request bodies of at least 32 MB.
- **Timeouts:** keep the default read timeout of 60 seconds or more.
- **Forwarded protocol:** pass `X-Forwarded-Proto` so Afterfeed marks its cookies as secure.

## Caddy

```txt
afterfeed.example.com {
    reverse_proxy afterfeed:8080
}
```

Caddy has no request size limit by default and sets the forwarded headers.

## Nginx

```nginx
server {
    listen 443 ssl;
    server_name afterfeed.example.com;

    client_max_body_size 64m;

    location / {
        proxy_pass http://afterfeed:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## Nginx Proxy Manager

Create a proxy host for `afterfeed` on port `8080`, request an SSL certificate, and add this under **Advanced**:

```nginx
client_max_body_size 64m;
```

## Traefik

```yaml
    labels:
      - traefik.enable=true
      - traefik.http.routers.afterfeed.rule=Host(`afterfeed.example.com`)
      - traefik.http.routers.afterfeed.entrypoints=websecure
      - traefik.http.routers.afterfeed.tls.certresolver=letsencrypt
      - traefik.http.services.afterfeed.loadbalancer.server.port=8080
```

## Extra protection

Afterfeed has its own sign-in with optional [two-factor authentication](/security#two-factor-authentication). You can add a second layer with your proxy's authentication (for example Authelia or Authentik) in front of it.
