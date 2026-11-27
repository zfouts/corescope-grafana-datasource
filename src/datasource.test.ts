import { DataSource } from './datasource';
import { CoreScopeQuery } from './types';

const replace = jest.fn((v: string, _s?: unknown, fmt?: string) => `${v}|${fmt ?? 'default'}`);
const getResource = jest.fn();

jest.mock('@grafana/runtime', () => ({
  DataSourceWithBackend: class {
    getResource = getResource;
  },
  getTemplateSrv: () => ({ replace }),
}));

const ds = () => new DataSource({} as never);
const q = (p: Partial<CoreScopeQuery>): CoreScopeQuery => ({ refId: 'A', queryType: 'nodes', ...p });

beforeEach(() => {
  replace.mockClear();
  getResource.mockReset();
});

describe('applyTemplateVariables', () => {
  it('interpolates only the fields listed for the query type', () => {
    const out = ds().applyTemplateVariables(
      q({ queryType: 'nodes', region: '$r', search: '$s', observerId: '$o', nodePubkey: '$p' }),
      {}
    );
    expect(out.region).toBe('$r|csv');
    expect(out.search).toBe('$s|default');
    expect(out.observerId).toBe('$o');
    expect(out.nodePubkey).toBe('$p');
  });

  it('uses csv only for region', () => {
    const out = ds().applyTemplateVariables(q({ queryType: 'observerMetrics', observerId: '$o' }), {});
    expect(out.observerId).toBe('$o|default');
  });

  it('leaves types without fields untouched', () => {
    const out = ds().applyTemplateVariables(q({ queryType: 'stats', region: '$r' }), {});
    expect(out.region).toBe('$r');
    expect(replace).not.toHaveBeenCalled();
  });
});

describe('filterQuery', () => {
  it.each([
    [q({ queryType: 'stats' }), true],
    [q({ queryType: 'stats', hide: true }), false],
    [q({ queryType: 'observerMetrics' }), false],
    [q({ queryType: 'observerMetrics', observerId: 'abc' }), true],
    [q({ queryType: 'nodeActivity' }), false],
    [q({ queryType: 'nodeActivity', nodePubkey: '$p' }), true],
    [q({ queryType: 'nodeBattery' }), false],
    [q({ queryType: 'nodeBattery', nodePubkey: 'a'.repeat(64) }), true],
  ])('%j -> %s', (query, want) => {
    expect(ds().filterQuery(query)).toBe(want);
  });
});

describe('metricFindQuery', () => {
  it('maps observers', async () => {
    getResource.mockResolvedValue([{ text: 'Obs 1', value: 'o1', extra: 1 }]);
    const out = await ds().metricFindQuery({ kind: 'observers' });
    expect(getResource).toHaveBeenCalledWith('observers');
    expect(out).toEqual([{ text: 'Obs 1', value: 'o1' }]);
  });

  it('maps regions', async () => {
    getResource.mockResolvedValue([{ text: 'SEA', value: 'SEA' }]);
    expect(await ds().metricFindQuery({ kind: 'regions' })).toEqual([{ text: 'SEA', value: 'SEA' }]);
    expect(getResource).toHaveBeenCalledWith('regions');
  });

  it('returns an empty list for a malformed response', async () => {
    getResource.mockResolvedValue(null);
    expect(await ds().getObservers()).toEqual([]);
  });
});
