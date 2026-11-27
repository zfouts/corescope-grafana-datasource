# CoreScope data source for Grafana

A Grafana data source plugin for [CoreScope](https://github.com/Kpa-clawbot/CoreScope), the analyzer for MeshCore LoRa mesh networks. It reads a CoreScope server's public API and turns packet rates, observer radio metrics, node activity and battery, and server status into Grafana data frames. A backend (Go) component makes the requests, so credentials stay on the Grafana server.

## Features

- 12 query types: 4 for the CoreScope server itself, 6 mesh time series, 2 tables (see [Query types](#query-types)).
- Template variables for observers and regions.
- Optional HTTP basic auth or bearer token, for a CoreScope instance behind a reverse proxy.
- Read-only: fixed `GET` endpoints only. The plugin never uses CoreScope's admin API key.
- "Save & test" reports the CoreScope version and its node and observer counts.

## Requirements

- Grafana 13.2 or later.
- A CoreScope server reachable from the Grafana server. The endpoints the plugin uses are public and need no CoreScope login.

## Install

There is no signed release yet. Signing and a Grafana plugin catalog listing are planned. Until then:

1. Download the plugin zip from the [GitHub releases](https://github.com/zfouts/corescope-grafana-datasource/releases) page (releases are planned; until one exists, build from source, see [Development](#development)).
2. Unpack it into the Grafana plugins directory, so that you have `<plugins dir>/zfouts-corescope-datasource/`.
3. Allow the unsigned plugin and restart Grafana:

   ```bash
   GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=zfouts-corescope-datasource
   ```

## Configure

Add a data source of type CoreScope (Connections, Data sources, Add data source), fill in the settings, and click Save & test.

| Setting                    | Key                        | Default         | Notes                                                                                                                                                                                                                                        |
| -------------------------- | -------------------------- | --------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| URL                        | `url`                      | none (required) | CoreScope base URL, for example `https://analyzer.example.net`. A base path such as `https://host/corescope` is kept; a trailing `/` is removed. Must be `http` or `https`, and must not contain `user:pass@`, a query string or a fragment. |
| Timeout                    | `timeoutSeconds`           | 15              | Budget for one request including retries. Values are clamped to 1 to 60.                                                                                                                                                                     |
| Basic auth user            | `basicAuthUser`            | empty           | Used with the password below; a password needs a user.                                                                                                                                                                                       |
| Basic auth password        | `basicAuthPassword`        | empty           | Secret (`secureJsonData`).                                                                                                                                                                                                                   |
| Bearer token               | `bearerToken`              | empty           | Secret (`secureJsonData`). Mutually exclusive with basic auth.                                                                                                                                                                               |
| Skip TLS verification      | `tlsSkipVerify`            | off             | Turns off certificate checks. "Save & test" shows a warning while it is on.                                                                                                                                                                  |
| Allow insecure credentials | `allowInsecureCredentials` | off             | Credentials over plain `http://` to a host other than `localhost` or a loopback IP are refused unless this is on.                                                                                                                            |
| Mesh data only             | `meshOnly`                 | off             | For a data source behind a public or anonymous dashboard. Refuses the Server health, API endpoint latency and MQTT sources queries, and drops memory, version and commit from Server stats. See Security.                                    |

Provisioning example (`datasources.yml`):

```yaml
apiVersion: 1

datasources:
  - name: CoreScope
    uid: corescope
    type: zfouts-corescope-datasource
    access: proxy
    jsonData:
      url: https://analyzer.example.net
      timeoutSeconds: 15
      # Only if the server sits behind basic auth:
      basicAuthUser: grafana
    secureJsonData:
      basicAuthPassword: ${CORESCOPE_PASSWORD}
```

For a bearer token, drop `basicAuthUser` and set `secureJsonData.bearerToken` instead.

## Query types

Labels are as shown in the query editor. The mesh types read recorded history. The server operations types return the current value only.

| Label                   | `queryType`       | Fields                                | Output                                                                                                                          | History           |
| ----------------------- | ----------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | ----------------- |
| Server stats            | `stats`           | none                                  | One row: packet, node and observer counts, memory, version                                                                      | No                |
| Server health           | `health`          | none                                  | One row: uptime, memory, cache, WebSocket and request counters, status                                                          | No                |
| API endpoint latency    | `endpoints`       | none                                  | One row per endpoint: count, avg, p50, p95 and max ms                                                                           | No                |
| MQTT sources            | `mqtt`            | none                                  | One row per source: connection state, last connect, disconnect and packet times, packet counts                                  | No                |
| Packets per hour        | `packetsPerHour`  | `region`                              | Time series: `packets`, `avgSnr`                                                                                                | Yes               |
| Packets by payload type | `payloadTypes`    | `region`                              | One row per payload type: `type`, `count` (totals over the dashboard range, not a time series)                                  | Totals over range |
| Retransmissions         | `retransmissions` | `region`                              | Time series: `packets`, `repeaterSum`, `observers`, `avgRepeaters`                                                              | Yes               |
| Observer radio metrics  | `observerMetrics` | `observerId` (required), `resolution` | Time series: `noiseFloor`, `batteryMv`, `txAirtimePct`, `rxAirtimePct`, `recvErrorRate`, `packetsSent`, `packetsRecv`, `reboot` | Yes               |
| Node activity           | `nodeActivity`    | `nodePubkey` (required)               | Time series: `packets`, `certain`                                                                                               | Yes               |
| Node battery            | `nodeBattery`     | `nodePubkey` (required)               | Time series: `batteryMv`                                                                                                        | Yes               |
| Nodes                   | `nodes`           | `region`, `role`, `search`, `limit`   | Table of known nodes                                                                                                            | Current state     |
| Observers               | `observers`       | none                                  | Table of observers and their status                                                                                             | Current state     |

Field rules:

- `region`: optional. One or more region codes, comma-separated (up to 16). Empty, `All` or `$__all` means no filter. A multi-value variable works here.
- `observerId`: letters, digits, `_` and `-`, up to 128 characters.
- `nodePubkey`: 64 hex characters; upper case is accepted.
- `resolution`: `5m`, `1h` or `1d`. Empty is automatic: a dashboard range up to 2 days uses 5m, up to 30 days 1h, longer 1d.
- `role` (nodes): `repeater`, `companion`, `room` or `sensor`.
- `limit` (nodes): default 100, clamped to 1 to 500.
- `search` (nodes): up to 64 characters.
- `nodeActivity` and `nodeBattery` ask CoreScope for whole days: the dashboard range rounded up, 1 to 365. Rows outside the range are dropped.

A field that fails these rules gives that query a 400 error naming the field. Other queries in the same request still run. A request may hold at most 50 queries; 4 run at a time.

## Template variables

Use the query variable editor with this data source. Two lookups are available:

- Observers: values are observer IDs, labels are observer names (the ID when there is no name). Use it for `observerId`.
- Regions: values are region codes, labels are region names. Use it for `region`.

For a multi-value region variable with an All option, set the variable's custom all value to `All`: the plugin reads `All` (and `$__all`) as no region filter, and a list of every region can exceed the 16-region limit. The same `All` works for a role variable.

## Example dashboard

The development environment provisions a "CoreScope Overview" dashboard (`provisioning/dashboards/corescope-overview.json` in this repository). Import that file into Grafana to use it against your own server.

## Limitations

- **Server operations queries are current values only.** CoreScope keeps no history of server stats, health, API endpoint latency or MQTT source state, so this plugin cannot draw a time series of, for example, memory use. Each refresh returns the value now. To graph these over time, have Grafana store them another way (for example, scrape them into a time series database).
- API endpoint percentiles cover the period since CoreScope last restarted, not the dashboard range.
- Mesh types return CoreScope's recorded history, within the limits of the CoreScope server's own retention.
- Responses are cached for 5 seconds, so refresh intervals under that return the same data.
- Responses over 8 MiB (32 MiB for the nodes list) are rejected.
- Retries happen only on HTTP 503 and connection-refused errors, up to 3 attempts within the timeout. Other failures are reported straight away.

## Security

What the plugin does:

- Any Grafana Viewer can send queries to a data source, so every query field is validated strictly (see the field rules above) before it is used to build a request. Failures return fixed messages that do not repeat the input.
- Only a fixed list of read-only `GET` endpoints is requested; the plugin does not forward arbitrary paths.
- Redirects are not followed, so credentials are never sent to another host.
- Basic auth passwords and bearer tokens are stored as Grafana secrets (`secureJsonData`) and only decrypted in the backend. Error messages never include credentials or setting values.
- The URL may not embed credentials, and credentials over plain `http://` to a non-loopback host are refused unless you opt in.
- With Mesh data only on, the server-internals query types are refused in the backend, not just left off a dashboard. A Viewer can send any query type to a data source, so hiding panels alone would not keep them private.

Your part as the operator:

- Only Org Admins can set the URL. The Grafana server will make requests to whatever host it names. To restrict what it can reach, use Grafana's [secure SOCKS proxy](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/proxy/) or network egress rules.
- Grafana's process honours `HTTP_PROXY`/`HTTPS_PROXY`. With an `http://` CoreScope URL, a proxy set there sees the request, including any basic-auth or bearer credentials. Use `https://` when credentials are configured.
- Prefer `https://`, and leave Skip TLS verification off.

To report a vulnerability, use GitHub private vulnerability reporting on the [repository's Security tab](https://github.com/zfouts/corescope-grafana-datasource/security). Please do not open a public issue.

## Development

Requirements: Node.js, npm, Docker. No local Go install is needed.

```bash
npm install
npm run dev:data        # download CoreScope's public test fixture, seed synthetic observer metrics into dev/data
npm run build:backend   # build the Go backend through Docker
npm run build           # or `npm run dev` to rebuild on change
docker compose up
```

Grafana is then at http://localhost:3000 and CoreScope at http://localhost:13582, with a provisioned "CoreScope" data source and the "CoreScope Overview" dashboard.

Tests:

```bash
npm run test:ci
npm run typecheck
npm run lint
docker run --rm -v cs-gomod:/go/pkg/mod -v "$PWD":/src -w /src golang:1.27 go test -race ./pkg/...
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
