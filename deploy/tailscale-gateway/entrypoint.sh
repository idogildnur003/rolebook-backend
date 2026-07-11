#!/bin/sh
# Bring up socat (TCP bridge) + Tailscale (userspace), then publish the private
# Rolebook MongoDB onto the Tailnet as raw TCP.
#
# Env (set as Railway variables on this service):
#   TS_AUTHKEY     required — Tailscale auth key (reusable + non-ephemeral so
#                  redeploys re-register cleanly; tag it for ACL scoping).
#   TS_HOSTNAME    Tailnet node name. Default: rolebook-gateway.
#   MONGO_TARGET   host:port of Mongo on the Railway private net.
#                  Default: mongodb.railway.internal:27017.
#   LISTEN_PORT    Tailnet TCP port to publish Mongo on. Default: 27017.
#   TS_STATE       tailscaled state path. Default /var/lib/tailscale/tailscaled.state
#                  — mount a Railway volume there (plus a non-ephemeral key) for a
#                  stable node identity/IP across redeploys.
#   TS_DEBUG_MTU   Optional. Lower the tunnel MTU (e.g. 1280) if large transfers
#                  stall — railnet0 MTU is 1316 and userspace WireGuard adds
#                  overhead. Read by tailscaled directly from the environment.
set -e

: "${MONGO_TARGET:=mongodb.railway.internal:27017}"
: "${TS_HOSTNAME:=rolebook-gateway}"
: "${LISTEN_PORT:=27017}"
: "${TS_STATE:=/var/lib/tailscale/tailscaled.state}"

if [ -z "${TS_AUTHKEY}" ]; then
  echo "[gateway] TS_AUTHKEY is required (create a reusable, non-ephemeral key in the Tailscale admin console)." >&2
  exit 1
fi

# 1. TCP bridge: localhost:LISTEN_PORT -> Mongo on the Railway private network.
#    The private net is IPv6-only, so dial the target over IPv6 (TCP6).
socat TCP4-LISTEN:"${LISTEN_PORT}",bind=127.0.0.1,fork,reuseaddr TCP6:"${MONGO_TARGET}" &

# 2. Tailscale daemon — userspace networking (Railway has no /dev/net/tun).
tailscaled \
  --tun=userspace-networking \
  --state="${TS_STATE}" \
  --socket=/var/run/tailscale/tailscaled.sock &
TAILSCALED_PID=$!

# Wait for the control socket before issuing commands.
i=0
while [ ! -S /var/run/tailscale/tailscaled.sock ] && [ "$i" -lt 30 ]; do
  i=$((i + 1)); sleep 0.5
done

# 3. Join the tailnet. --accept-dns=false keeps the container using Railway's
#    resolver (fd12::10) so MONGO_TARGET keeps resolving.
if ! tailscale up --authkey="${TS_AUTHKEY}" --hostname="${TS_HOSTNAME}" --accept-dns=false; then
  echo "[gateway] 'tailscale up' failed — check TS_AUTHKEY is a reusable, non-ephemeral auth key (tskey-auth-...)." >&2
  exit 1
fi

# 4. Publish Mongo onto the Tailnet as raw TCP (no TLS; the WireGuard tunnel
#    already encrypts it).
tailscale serve --bg --tcp="${LISTEN_PORT}" tcp://127.0.0.1:"${LISTEN_PORT}"

echo "[gateway] MongoDB on tailnet: ${TS_HOSTNAME}.<your-tailnet>.ts.net:${LISTEN_PORT}"

# Keep the container alive on the daemon.
wait "${TAILSCALED_PID}"
