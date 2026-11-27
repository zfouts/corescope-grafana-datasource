import React, { ChangeEvent, useState } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Combobox, ComboboxOption, InlineField, Input, Stack, Text } from '@grafana/ui';
import { DataSource } from '../datasource';
import { CoreScopeOptions, CoreScopeQuery, NodeRole, QUERY_FIELDS, Resolution } from '../types';
import { QUERY_TYPES, queryTypeInfo } from '../queryTypes';
import { useOptions } from './useOptions';

type Props = QueryEditorProps<DataSource, CoreScopeQuery, CoreScopeOptions>;

// Combobox needs non-empty values, so "no filter" is a sentinel mapped to ''.
const NONE = '__none__';

const ROLE_OPTIONS: Array<ComboboxOption<string>> = [
  { label: 'Any', value: NONE },
  { label: 'repeater', value: 'repeater' },
  { label: 'companion', value: 'companion' },
  { label: 'room', value: 'room' },
  { label: 'sensor', value: 'sensor' },
];

const RESOLUTION_OPTIONS: Array<ComboboxOption<string>> = [
  { label: 'Auto', value: NONE },
  { label: '5m', value: '5m' },
  { label: '1h', value: '1h' },
  { label: '1d', value: '1d' },
];

const TYPE_OPTIONS: Array<ComboboxOption<string>> = QUERY_TYPES.map((t) => ({
  label: t.label,
  value: t.value,
  group: t.group,
  description: t.description,
}));

const LABEL_WIDTH = 16;

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const fields = QUERY_FIELDS[query.queryType] ?? [];
  const info = queryTypeInfo(query.queryType);
  const has = (f: keyof CoreScopeQuery) => fields.includes(f);

  // Lists load once per editor mount, and the first time a type that shows the field is selected.
  const observers = useOptions(() => datasource.getObservers(), has('observerId'));
  const regions = useOptions(() => datasource.getRegions(), has('region'));

  const update = (patch: Partial<CoreScopeQuery>, run: boolean) => {
    onChange({ ...query, ...patch });
    if (run) {
      onRunQuery();
    }
  };

  // Text inputs keep a local draft (undefined = show the saved value) and commit on blur.
  const [drafts, setDrafts] = useState<{ nodePubkey?: string; search?: string; limit?: string }>({});
  const draft = {
    nodePubkey: drafts.nodePubkey ?? query.nodePubkey ?? '',
    search: drafts.search ?? query.search ?? '',
  };
  const clearDraft = (f: keyof typeof drafts) => setDrafts((d) => ({ ...d, [f]: undefined }));

  const commitText = (field: 'nodePubkey' | 'search') => {
    const value = draft[field].trim();
    if ((query[field] ?? '') !== value) {
      update({ [field]: value }, true);
    }
    clearDraft(field);
  };

  const limitDraft = drafts.limit ?? (query.limit === undefined ? '' : String(query.limit));
  const limitNum = Number(limitDraft);
  const limitInvalid = limitDraft !== '' && (!Number.isInteger(limitNum) || limitNum < 1 || limitNum > 500);
  const commitLimit = () => {
    if (limitInvalid) {
      return;
    }
    const next = limitDraft === '' ? undefined : limitNum;
    if (next !== query.limit) {
      update({ limit: next }, true);
    }
    clearDraft('limit');
  };

  const nodePubkey = draft.nodePubkey.trim();
  const pubkeyHint =
    nodePubkey && !nodePubkey.includes('$') && !/^[0-9a-fA-F]{64}$/.test(nodePubkey)
      ? 'Expected 64 hexadecimal characters, or a $variable.'
      : undefined;

  const withCurrent = (opts: Array<{ text: string; value: string }>, current?: string) => {
    const list: Array<ComboboxOption<string>> = opts.map((o) => ({ label: o.text, value: o.value }));
    return current && !list.some((o) => o.value === current) ? [{ label: current, value: current }, ...list] : list;
  };

  return (
    <Stack direction="column" gap={1}>
      <InlineField label="Query type" labelWidth={LABEL_WIDTH} grow>
        <Combobox
          id="query-editor-type"
          options={TYPE_OPTIONS}
          value={query.queryType}
          width={40}
          onChange={(o) => update({ queryType: o.value as CoreScopeQuery['queryType'] }, true)}
        />
      </InlineField>
      {info && (
        <Text variant="bodySmall" color="secondary">
          {info.description}
        </Text>
      )}

      {has('observerId') && (
        <InlineField
          label="Observer"
          labelWidth={LABEL_WIDTH}
          required
          tooltip="Pick an observer, type an ID, or use a $variable."
        >
          <Combobox
            id="query-editor-observer"
            options={withCurrent(observers.options, query.observerId)}
            value={query.observerId || null}
            loading={observers.loading}
            createCustomValue
            isClearable
            placeholder="Select or type an observer ID"
            width={40}
            onChange={(o) => update({ observerId: (o?.value ?? '').trim() }, true)}
          />
        </InlineField>
      )}
      {has('resolution') && (
        <InlineField label="Resolution" labelWidth={LABEL_WIDTH}>
          <Combobox
            id="query-editor-resolution"
            options={RESOLUTION_OPTIONS}
            value={query.resolution || NONE}
            width={20}
            onChange={(o) => update({ resolution: (o.value === NONE ? '' : o.value) as Resolution }, true)}
          />
        </InlineField>
      )}
      {has('nodePubkey') && (
        <InlineField
          label="Node public key"
          labelWidth={LABEL_WIDTH}
          required
          tooltip="64 hexadecimal characters, or a $variable."
          invalid={!!pubkeyHint}
          error={pubkeyHint}
        >
          <Input
            id="query-editor-pubkey"
            value={draft.nodePubkey}
            placeholder="64 hex characters or $variable"
            width={80}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setDrafts({ ...drafts, nodePubkey: e.target.value })}
            onBlur={() => commitText('nodePubkey')}
          />
        </InlineField>
      )}
      {has('region') && (
        <InlineField
          label="Region"
          labelWidth={LABEL_WIDTH}
          tooltip="Leave as All for every region. A $variable may hold several."
        >
          <Combobox
            id="query-editor-region"
            options={[{ label: 'All', value: NONE }, ...withCurrent(regions.options, query.region)]}
            value={query.region || NONE}
            loading={regions.loading}
            createCustomValue
            width={30}
            onChange={(o) => update({ region: o.value === NONE ? '' : o.value }, true)}
          />
        </InlineField>
      )}
      {has('role') && (
        <InlineField label="Role" labelWidth={LABEL_WIDTH}>
          <Combobox
            id="query-editor-role"
            options={ROLE_OPTIONS}
            value={query.role || NONE}
            width={20}
            onChange={(o) => update({ role: (o.value === NONE ? '' : o.value) as NodeRole }, true)}
          />
        </InlineField>
      )}
      {has('search') && (
        <InlineField label="Search" labelWidth={LABEL_WIDTH} tooltip="Match node names.">
          <Input
            id="query-editor-search"
            value={draft.search}
            placeholder="Node name contains"
            width={40}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setDrafts({ ...drafts, search: e.target.value })}
            onBlur={() => commitText('search')}
          />
        </InlineField>
      )}
      {has('limit') && (
        <InlineField
          label="Limit"
          labelWidth={LABEL_WIDTH}
          tooltip="Maximum rows, 1 to 500."
          invalid={limitInvalid}
          error="Enter a whole number from 1 to 500."
        >
          <Input
            id="query-editor-limit"
            type="number"
            min={1}
            max={500}
            value={limitDraft}
            placeholder="Default"
            width={12}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setDrafts({ ...drafts, limit: e.target.value })}
            onBlur={commitLimit}
          />
        </InlineField>
      )}
    </Stack>
  );
}
