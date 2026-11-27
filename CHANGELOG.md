# Changelog

## 0.1.0 (unreleased)

Initial release.

- Backend data source for a CoreScope server, using only fixed read-only `GET` endpoints.
- Server operations queries (current values only): Server stats, Server health, API endpoint latency, MQTT sources.
- Mesh time series queries: Packets per hour, Packets by payload type, Retransmissions, Observer radio metrics, Node activity, Node battery.
- Table queries: Nodes (filter by region, role and name) and Observers.
- Template variable lookups for observers and regions.
- Optional HTTP basic auth or bearer token, stored as secrets; TLS skip-verify and insecure-credentials opt-ins.
- Strict query and settings validation, no redirects followed, a 5 second response cache, and retries on 503 and connection refused.
- "Save & test" reports the CoreScope version and node and observer counts.
- Mesh data only (`meshOnly`): for public dashboards, refuses the server-internals query types and drops server fields from Server stats.
- `All` / `$__all` from a dashboard variable means no filter for both region and role.
