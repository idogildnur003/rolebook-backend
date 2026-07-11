# tailscale-gateway

Publishes the private Rolebook **MongoDB** onto the Tailnet over raw TCP, so
Mongo needs no public Railway TCP proxy. Serve-based (not a subnet router), so
it coexists with the tabshare gateway on the same tailnet.

## Railway service config
- **Root directory:** `deploy/tailscale-gateway/`
- **Watch paths:** `deploy/tailscale-gateway/**` (so backend pushes don't rebuild it)
- **Volume:** mount at `/var/lib/tailscale` (stable node identity across redeploys)
- **Public networking:** none

## Variables
| Key | Value |
|---|---|
| `TS_AUTHKEY` | secret — reusable, non-ephemeral Tailscale auth key |
| `TS_HOSTNAME` | `rolebook-gateway` |
| `MONGO_TARGET` | `mongodb.railway.internal:27017` |
| `TS_STATE` | `/var/lib/tailscale/tailscaled.state` |
| `TS_DEBUG_MTU` | `1280` (optional; for the 1316 railnet MTU — tune/remove during verify) |

## Connecting (from a device on the tailnet)
```
mongodb://<MONGOUSER>:<MONGOPASSWORD>@rolebook-gateway.<your-tailnet>.ts.net:27017/?directConnection=true&authSource=admin
```
`directConnection=true` because Mongo is a standalone `mongod`. Credentials are
Mongo's `MONGOUSER` / `MONGOPASSWORD` (also embedded in `MONGO_PUBLIC_URL`).

## Break-glass
If you lose tailnet access, temporarily re-enable the MongoDB service's public
TCP proxy in Railway (Settings → Networking → TCP Proxy on port 27017).
