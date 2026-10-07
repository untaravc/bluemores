# Bluemores

Lightweight server monitoring (think node_exporter + Prometheus in one Go binary), built with Fiber + SQLite.

One binary, two roles — enable either or both via `.env`:

| Role | Enabled by | What it does |
|---|---|---|
| **Exporter** | `ENABLE_EXPORTER=true` + `EXPORTER_KEY` | Serves `GET /mon/proc`, `/mon/mem`, `/mon/dfree` (requires `Authorization: Bearer <EXPORTER_KEY>`) |
| **Monitor + dashboard** | `ENABLE_MONITOR=true` + `DASHBOARD_PASSPHRASE` | Polls the targets in `config.json` on cron schedules, stores results in SQLite, sends Discord alerts, serves the dashboard at `/` |

Both flags default to `true`. With `ENABLE_MONITOR=false`, `GET /` only returns `{"status":"ok"}` (handy as a health check on exporter-only servers).

## Quick start

```bash
go build -o bluemores .
cp .env.example .env              # edit keys / passphrase
cp config.example.json config.json  # central server only
./bluemores
```

Cross-compile for a Linux server (pure Go, no CGO needed): `GOOS=linux GOARCH=amd64 go build -o bluemores .`

## Exporter responses

```json
GET /mon/proc   {"status":true,"data":{"value":120,"total":400}}              // summed per-core %, total = cores*100
GET /mon/mem    {"status":true,"data":{"unit":"MB","value":1200,"total":4096}}  // used = total - available
GET /mon/dfree  {"status":true,"data":{"unit":"GB","value":120,"total":150}}    // free space on DISK_PATH
```

## Config (`config.json`)

See `config.example.json`. Per app:

- `type`: `processor` | `memory` | `disk_free` | `health`
- `schedule`: standard 5-field cron (`*/5 * * * *`) or descriptors (`@every 30s`, `@hourly`)
- `threshold`:
  - `processor` / `memory`: alert when **value > threshold**
  - `disk_free`: alert when **used (total − free) > threshold**, in GB
  - `health`: `"true"` = expect 2xx (and not `{"status": false}`)
- `notification`: Discord webhook URL (optional)
- `key`: bearer key for the exporter; can also be set once per group as `"key"`

Notifications fire only on **state change** (ok → alert/error, and back to ok as "recovered"), so a stuck alert won't spam the channel.

## Dashboard

Open `http://host:3000/`, enter `DASHBOARD_PASSPHRASE`. Cards per group/app show the latest value, status, and a sparkline; click a card for a full chart with threshold line and a "Check now" button. Auto-refreshes every 30s. Results older than `RETENTION_DAYS` are pruned hourly.
