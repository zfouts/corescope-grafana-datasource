# CoreScope data source

Query a [CoreScope](https://github.com/Kpa-clawbot/CoreScope) server (a MeshCore LoRa mesh-network analyzer) from Grafana: packet rates, observer radio metrics, node activity and battery, and server status. Requires Grafana 13.2 or later. The plugin reads CoreScope's public, read-only API and never uses its admin API key.

## Configure

Add a CoreScope data source and set the URL of your server, for example `https://analyzer.example.net`. Then click Save & test, which reports the CoreScope version and its node and observer counts.

- **URL** (`url`): required, `http` or `https`, no `user:pass@`, query or fragment.
- **Timeout** (`timeoutSeconds`): default 15 seconds, clamped to 1 to 60, covers retries.
- **Basic auth** (`basicAuthUser` and `basicAuthPassword`) or **bearer token** (`bearerToken`): optional, one or the other. Stored as Grafana secrets.
- **Skip TLS verification** (`tlsSkipVerify`): off by default.
- **Allow insecure credentials** (`allowInsecureCredentials`): credentials over plain `http://` to a non-loopback host are refused unless this is on.

## Query types

| Label                   | `queryType`       | History              |
| ----------------------- | ----------------- | -------------------- |
| Server stats            | `stats`           | No, current value    |
| Server health           | `health`          | No, current value    |
| API endpoint latency    | `endpoints`       | No, current value    |
| MQTT sources            | `mqtt`            | No, current value    |
| Packets per hour        | `packetsPerHour`  | Yes                  |
| Packets by payload type | `payloadTypes`    | Totals over range    |
| Retransmissions         | `retransmissions` | Yes                  |
| Observer radio metrics  | `observerMetrics` | Yes                  |
| Node activity           | `nodeActivity`    | Yes                  |
| Node battery            | `nodeBattery`     | Yes                  |
| Nodes                   | `nodes`           | Table, current state |
| Observers               | `observers`       | Table, current state |

Template variables are available for observers and regions.

## Limitations

- The four server operations types return current values only. CoreScope keeps no history of them, so a time series of, say, memory use is not possible with this plugin. API endpoint percentiles are since CoreScope's last restart.
- Responses are cached for 5 seconds. A request may hold at most 50 queries.

## Links

- [Full documentation](https://github.com/zfouts/corescope-grafana-datasource#readme)
- [Source and issues](https://github.com/zfouts/corescope-grafana-datasource)
- [Report a vulnerability](https://github.com/zfouts/corescope-grafana-datasource/security)
- [CoreScope](https://github.com/Kpa-clawbot/CoreScope)
