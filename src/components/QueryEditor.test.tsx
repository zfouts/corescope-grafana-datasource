import React from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { QueryEditor } from './QueryEditor';
import { CoreScopeQuery, QUERY_FIELDS, QueryType } from '../types';

type Props = React.ComponentProps<typeof QueryEditor>;

function setup(query: Partial<CoreScopeQuery>) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const datasource = {
    getObservers: jest.fn().mockResolvedValue([{ text: 'Obs 1', value: 'o1' }]),
    getRegions: jest.fn().mockResolvedValue([{ text: 'SEA', value: 'SEA' }]),
  };
  const props = {
    query: { refId: 'A', queryType: 'stats', ...query } as CoreScopeQuery,
    onChange,
    onRunQuery,
    datasource,
  };
  const view = render(<QueryEditor {...(props as unknown as Props)} />);
  return { onChange, onRunQuery, datasource, view, props };
}

const FIELD_IDS: Record<string, string> = {
  observerId: 'query-editor-observer',
  resolution: 'query-editor-resolution',
  nodePubkey: 'query-editor-pubkey',
  region: 'query-editor-region',
  role: 'query-editor-role',
  search: 'query-editor-search',
  limit: 'query-editor-limit',
};

describe('QueryEditor', () => {
  it.each(Object.keys(QUERY_FIELDS) as QueryType[])('shows only the fields of %s', async (queryType) => {
    const { view } = setup({ queryType });
    await act(async () => {});
    for (const [field, id] of Object.entries(FIELD_IDS)) {
      const shown = view.container.querySelector(`#${id}`) !== null;
      expect(shown).toBe((QUERY_FIELDS[queryType] as string[]).includes(field));
    }
  });

  it('describes ops queries as current values', () => {
    setup({ queryType: 'stats' });
    expect(screen.getByText(/Not history/)).toBeInTheDocument();
  });

  it('loads each list once, not per rerender', async () => {
    const { datasource, view, props } = setup({ queryType: 'nodes' });
    view.rerender(<QueryEditor {...(props as unknown as Props)} query={{ ...props.query, search: 'a' }} />);
    view.rerender(<QueryEditor {...(props as unknown as Props)} query={{ ...props.query, search: 'ab' }} />);
    expect(datasource.getObservers).not.toHaveBeenCalled();
    expect(datasource.getRegions).toHaveBeenCalledTimes(1);
  });

  it('commits text on blur, not on keystroke', () => {
    const { onChange, onRunQuery } = setup({ queryType: 'nodes' });
    const input = screen.getByPlaceholderText('Node name contains');
    fireEvent.change(input, { target: { value: 'hill' } });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.blur(input);
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ queryType: 'nodes', search: 'hill' }));
    expect(onRunQuery).toHaveBeenCalledTimes(1);
  });

  it('commits a valid limit and refuses an invalid one', () => {
    const { onChange } = setup({ queryType: 'nodes' });
    const input = screen.getByPlaceholderText('Default');
    fireEvent.change(input, { target: { value: '900' } });
    expect(screen.getByText(/1 to 500/)).toBeInTheDocument();
    fireEvent.blur(input);
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value: '50' } });
    fireEvent.blur(input);
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ limit: 50 }));
  });

  it('hints on a malformed public key but accepts a variable', () => {
    setup({ queryType: 'nodeBattery', nodePubkey: 'abc' });
    expect(screen.getByText(/64 hexadecimal/)).toBeInTheDocument();
  });

  it('shows no hint for a $variable key', () => {
    setup({ queryType: 'nodeBattery', nodePubkey: '$node' });
    expect(screen.queryByText(/Expected 64/)).not.toBeInTheDocument();
  });
});

describe('QueryEditor trimming and lazy loading', () => {
  it('trims text on commit', () => {
    const { onChange } = setup({ queryType: 'nodes' });
    const input = screen.getByPlaceholderText('Node name contains');
    fireEvent.change(input, { target: { value: '  hill  ' } });
    fireEvent.blur(input);
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ search: 'hill' }));
  });

  it('does not load option lists for types that do not use them', () => {
    const { datasource } = setup({ queryType: 'stats' });
    expect(datasource.getObservers).not.toHaveBeenCalled();
    expect(datasource.getRegions).not.toHaveBeenCalled();
  });
});
