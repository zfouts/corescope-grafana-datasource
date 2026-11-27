import { QueryType } from './types';

export interface QueryTypeInfo {
  value: QueryType;
  label: string;
  group: string;
  description: string;
}

export const GROUP_OPS = 'Server operations (current values)';
export const GROUP_MESH = 'Mesh (time series)';
export const GROUP_BREAKDOWNS = 'Breakdowns (totals over the range)';
export const GROUP_TABLES = 'Tables';

export const QUERY_TYPES: QueryTypeInfo[] = [
  {
    value: 'stats',
    label: 'Server stats',
    group: GROUP_OPS,
    description: 'Current packet, node and observer counts. Not history.',
  },
  {
    value: 'health',
    label: 'Server health',
    group: GROUP_OPS,
    description:
      'Current server health: uptime, memory, event-loop lag, caches, websocket clients, slow queries. Not history.',
  },
  {
    value: 'endpoints',
    label: 'API endpoint latency',
    group: GROUP_OPS,
    description: 'Current latency per API endpoint. Not history.',
  },
  {
    value: 'mqtt',
    label: 'MQTT sources',
    group: GROUP_OPS,
    description: 'Current state of each MQTT ingest source. Not history.',
  },
  {
    value: 'packetsPerHour',
    label: 'Packets per hour',
    group: GROUP_MESH,
    description: 'Packet count per hour, optionally for one region.',
  },
  {
    value: 'retransmissions',
    label: 'Retransmissions',
    group: GROUP_MESH,
    description: 'How often packets were heard again via other paths.',
  },
  {
    value: 'observerMetrics',
    label: 'Observer radio metrics',
    group: GROUP_MESH,
    description: 'Noise floor, airtime and other radio metrics of one observer.',
  },
  {
    value: 'nodeActivity',
    label: 'Node activity',
    group: GROUP_MESH,
    description: 'Packet activity of one node over time.',
  },
  {
    value: 'nodeBattery',
    label: 'Node battery',
    group: GROUP_MESH,
    description: 'Battery voltage of one node over time.',
  },
  {
    value: 'payloadTypes',
    label: 'Packets by payload type',
    group: GROUP_BREAKDOWNS,
    description: 'Total packets per payload type over the selected time range.',
  },
  {
    value: 'nodes',
    label: 'Nodes',
    group: GROUP_TABLES,
    description: 'Table of known nodes, filterable by region, role and name.',
  },
  { value: 'observers', label: 'Observers', group: GROUP_TABLES, description: 'Table of observers and their status.' },
];

export function queryTypeInfo(t: QueryType | undefined): QueryTypeInfo | undefined {
  return QUERY_TYPES.find((q) => q.value === t);
}
