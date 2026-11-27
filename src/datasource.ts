import { CoreApp, DataSourceInstanceSettings, MetricFindValue, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';

import { CoreScopeOptions, CoreScopeQuery, DEFAULT_QUERY, MetricFindOption, QUERY_FIELDS } from './types';

export type MetricFindKind = 'observers' | 'regions';

const STRING_FIELDS = ['observerId', 'nodePubkey', 'region', 'role', 'resolution', 'search'] as const;

export class DataSource extends DataSourceWithBackend<CoreScopeQuery, CoreScopeOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<CoreScopeOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<CoreScopeQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: CoreScopeQuery, scopedVars: ScopedVars): CoreScopeQuery {
    const used = QUERY_FIELDS[query.queryType] ?? [];
    const out: CoreScopeQuery = { ...query };
    const srv = getTemplateSrv();
    for (const field of STRING_FIELDS) {
      const value = query[field];
      if (!used.includes(field) || typeof value !== 'string' || value === '') {
        continue;
      }
      // Multi-value variables become "a,b,c" for region; the backend splits.
      out[field] = (
        field === 'region' ? srv.replace(value, scopedVars, 'csv') : srv.replace(value, scopedVars)
      ) as never;
    }
    return out;
  }

  filterQuery(query: CoreScopeQuery): boolean {
    if (query.hide) {
      return false;
    }
    switch (query.queryType) {
      case 'observerMetrics':
        return !!query.observerId;
      case 'nodeActivity':
      case 'nodeBattery':
        return !!query.nodePubkey;
      default:
        return true;
    }
  }

  private async options(kind: MetricFindKind): Promise<MetricFindOption[]> {
    const res = await this.getResource(kind);
    return Array.isArray(res) ? (res as MetricFindOption[]) : [];
  }

  getObservers(): Promise<MetricFindOption[]> {
    return this.options('observers');
  }

  getRegions(): Promise<MetricFindOption[]> {
    return this.options('regions');
  }

  async metricFindQuery(query: { kind: MetricFindKind }): Promise<MetricFindValue[]> {
    const list = await this.options(query?.kind === 'regions' ? 'regions' : 'observers');
    return list.map((o) => ({ text: o.text, value: o.value }));
  }
}
