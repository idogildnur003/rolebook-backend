# Tailscale gateway — publishes the private Rolebook MongoDB onto your Tailnet
# over raw TCP, so Mongo needs no public Railway TCP proxy. Mirrors the tabshare
# gateway, but forwards TCP (via socat) instead of HTTP (via Caddy).
#
#   you (on your Tailnet) ──TCP 27017──> tailscale serve --tcp ──> socat (localhost)
#                                                                    └─> mongodb.railway.internal:27017
#
# NOTE: `:latest` matches the tabshare gateway. For reproducibility, pin to a
# specific release (run the image once and read `tailscale version`, then set
# e.g. tailscale/tailscale:v1.86.2).
FROM tailscale/tailscale:latest

# socat bridges localhost -> mongodb.railway.internal. The tailscale image is
# Alpine-based, so apk is available at build time.
RUN apk add --no-cache socat

RUN mkdir -p /var/run/tailscale /var/lib/tailscale

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
