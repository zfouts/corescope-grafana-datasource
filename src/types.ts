import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

// Mirrors pkg/plugin/query.go. Keep the two in sync.
export type QueryType =
  | 'stats'
  | 'health'
  | 'endpoints'
  | 'mqtt'
  | 'packetsPerHour'
  | 'payloadTypes'
  | 'retransmissions'
  | 'observerMetrics'
  | 'nodeActivity'
  | 'nodeBattery'
  | 'nodes'
  | 'observers';

export type NodeRole = '' | 'repeater' | 'companion' | 'room' | 'sensor';
export type Resolution = '' | '5m' | '1h' | '1d';

export interface CoreScopeQuery extends DataQuery {
  queryType: QueryType;
  observerId?: string; // observerMetrics
  nodePubkey?: string; // nodeActivity, nodeBattery
  region?: string; // packetsPerHour, payloadTypes, retransmissions, nodes
  role?: NodeRole; // nodes
  resolution?: Resolution; // observerMetrics
  limit?: number; // nodes
  search?: string; // nodes
}

export const DEFAULT_QUERY: Partial<CoreScopeQuery> = {
  queryType: 'packetsPerHour',
};

// Which optional fields each query type uses. The editor shows only these;
// applyTemplateVariables interpolates only these string fields.
export const QUERY_FIELDS: Record<QueryType, Array<keyof CoreScopeQuery>> = {
  stats: [],
  health: [],
  endpoints: [],
  mqtt: [],
  packetsPerHour: ['region'],
  payloadTypes: ['region'],
  retransmissions: ['region'],
  observerMetrics: ['observerId', 'resolution'],
  nodeActivity: ['nodePubkey'],
  nodeBattery: ['nodePubkey'],
  nodes: ['region', 'role', 'limit', 'search'],
  observers: [],
};

/** jsonData. See pkg/models/settings.go. */
export interface CoreScopeOptions extends DataSourceJsonData {
  url?: string;
  timeoutSeconds?: number;
  basicAuthUser?: string;
  tlsSkipVerify?: boolean;
  allowInsecureCredentials?: boolean;
  meshOnly?: boolean;
}

/** secureJsonData, write-only. */
export interface CoreScopeSecureJsonData {
  basicAuthPassword?: string;
  bearerToken?: string;
}

/** CallResource 'observers' and 'regions' return this. */
export interface MetricFindOption {
  text: string;
  value: string;
}
