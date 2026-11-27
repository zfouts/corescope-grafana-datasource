import React from 'react';
import { InlineField, RadioButtonGroup } from '@grafana/ui';
import { MetricFindKind } from '../datasource';

export interface VariableQuery {
  kind: MetricFindKind;
}

interface Props {
  query: VariableQuery;
  onChange: (query: VariableQuery, definition: string) => void;
}

const KINDS: Array<{ label: string; value: MetricFindKind }> = [
  { label: 'Observers', value: 'observers' },
  { label: 'Regions', value: 'regions' },
];

export function VariableQueryEditor({ query, onChange }: Props) {
  const kind: MetricFindKind = query?.kind === 'regions' ? 'regions' : 'observers';
  return (
    <InlineField label="Values" labelWidth={14} tooltip="Which list of CoreScope values the variable offers.">
      <RadioButtonGroup
        options={KINDS}
        value={kind}
        onChange={(k) => onChange({ kind: k }, k === 'regions' ? 'Regions' : 'Observers')}
      />
    </InlineField>
  );
}
