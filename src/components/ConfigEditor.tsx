import React, { ChangeEvent, useState } from 'react';
import { Alert, FieldSet, InlineField, InlineSwitch, Input, RadioButtonGroup, SecretInput } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { CoreScopeOptions, CoreScopeSecureJsonData } from '../types';

interface Props extends DataSourcePluginOptionsEditorProps<CoreScopeOptions, CoreScopeSecureJsonData> {}

type AuthType = 'none' | 'basic' | 'bearer';

const AUTH_OPTIONS: Array<{ label: string; value: AuthType }> = [
  { label: 'None', value: 'none' },
  { label: 'Basic auth', value: 'basic' },
  { label: 'Bearer token', value: 'bearer' },
];

export const DEFAULT_TIMEOUT_SECONDS = 15;

/** Returns an error message for a bad URL, or undefined when it is acceptable (or still empty). */
export function validateUrl(url: string | undefined): string | undefined {
  if (!url) {
    return undefined;
  }
  if (!/^https?:\/\//i.test(url)) {
    return 'The URL must start with http:// or https://.';
  }
  if (/^https?:\/\/[^/?#]*@/i.test(url)) {
    return 'Do not put a user name or password in the URL. Use the Authentication section instead.';
  }
  return undefined;
}

export function validateTimeout(t: number | undefined): string | undefined {
  if (t === undefined) {
    return undefined;
  }
  return Number.isInteger(t) && t >= 1 && t <= 60 ? undefined : 'Enter a whole number of seconds from 1 to 60.';
}

export function ConfigEditor({ onOptionsChange, options }: Props) {
  const { jsonData, secureJsonFields, secureJsonData } = options;
  const [authType, setAuthType] = useState<AuthType>(() =>
    jsonData.basicAuthUser || secureJsonFields?.basicAuthPassword
      ? 'basic'
      : secureJsonFields?.bearerToken
        ? 'bearer'
        : 'none'
  );
  const [urlTouched, setUrlTouched] = useState(false);

  const setJson = (patch: Partial<CoreScopeOptions>) =>
    onOptionsChange({ ...options, jsonData: { ...jsonData, ...patch } });

  const urlError = validateUrl(jsonData.url);
  const urlMissing = urlTouched && !jsonData.url;
  const timeoutError = validateTimeout(jsonData.timeoutSeconds);

  const clearSecret = (name: keyof CoreScopeSecureJsonData) => ({
    secureJsonFields: { ...secureJsonFields, [name]: false },
    secureJsonData: { ...secureJsonData, [name]: '' },
  });

  const onAuthTypeChange = (next: AuthType) => {
    if (next === authType) {
      return;
    }
    setAuthType(next);
    // Switching clears the other kind so a stale credential is never sent.
    const cleared =
      authType === 'basic'
        ? { jsonData: { ...jsonData, basicAuthUser: undefined }, ...clearSecret('basicAuthPassword') }
        : authType === 'bearer'
          ? { jsonData, ...clearSecret('bearerToken') }
          : { jsonData };
    onOptionsChange({ ...options, ...cleared });
  };

  const setSecret = (name: keyof CoreScopeSecureJsonData, value: string) =>
    onOptionsChange({ ...options, secureJsonData: { ...secureJsonData, [name]: value } });

  const onTimeoutChange = (e: ChangeEvent<HTMLInputElement>) =>
    setJson({ timeoutSeconds: e.target.value === '' ? undefined : Math.round(Number(e.target.value)) });

  return (
    <>
      <FieldSet label="Connection">
        <InlineField
          label="URL"
          labelWidth={20}
          required
          interactive
          tooltip="Base URL of the CoreScope server."
          invalid={!!urlError || urlMissing}
          error={urlError ?? 'A URL is required.'}
        >
          <Input
            id="config-editor-url"
            value={jsonData.url ?? ''}
            placeholder="https://analyzer.example.net"
            width={40}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setJson({ url: e.target.value.trim() })}
            onBlur={() => setUrlTouched(true)}
          />
        </InlineField>
        <InlineField
          label="Timeout (seconds)"
          labelWidth={20}
          tooltip="How long to wait for CoreScope to answer. 1 to 60 seconds."
          invalid={!!timeoutError}
          error={timeoutError}
        >
          <Input
            id="config-editor-timeout"
            type="number"
            min={1}
            max={60}
            step={1}
            value={jsonData.timeoutSeconds ?? ''}
            placeholder={String(DEFAULT_TIMEOUT_SECONDS)}
            width={12}
            onChange={onTimeoutChange}
          />
        </InlineField>
      </FieldSet>

      <FieldSet label="Authentication">
        <InlineField label="Authentication" labelWidth={20}>
          <RadioButtonGroup options={AUTH_OPTIONS} value={authType} onChange={onAuthTypeChange} />
        </InlineField>
        {authType === 'basic' && (
          <>
            <InlineField label="User" labelWidth={20}>
              <Input
                id="config-editor-basic-user"
                value={jsonData.basicAuthUser ?? ''}
                placeholder="User name"
                width={40}
                autoComplete="off"
                onChange={(e: ChangeEvent<HTMLInputElement>) => setJson({ basicAuthUser: e.target.value })}
              />
            </InlineField>
            <InlineField label="Password" labelWidth={20}>
              <SecretInput
                id="config-editor-basic-password"
                isConfigured={!!secureJsonFields?.basicAuthPassword}
                value={secureJsonData?.basicAuthPassword ?? ''}
                placeholder="Password"
                width={40}
                autoComplete="new-password"
                onReset={() => onOptionsChange({ ...options, ...clearSecret('basicAuthPassword') })}
                onChange={(e: ChangeEvent<HTMLInputElement>) => setSecret('basicAuthPassword', e.target.value)}
              />
            </InlineField>
          </>
        )}
        {authType === 'bearer' && (
          <InlineField label="Token" labelWidth={20}>
            <SecretInput
              id="config-editor-bearer-token"
              isConfigured={!!secureJsonFields?.bearerToken}
              value={secureJsonData?.bearerToken ?? ''}
              placeholder="Bearer token"
              width={40}
              autoComplete="new-password"
              onReset={() => onOptionsChange({ ...options, ...clearSecret('bearerToken') })}
              onChange={(e: ChangeEvent<HTMLInputElement>) => setSecret('bearerToken', e.target.value)}
            />
          </InlineField>
        )}
      </FieldSet>

      <FieldSet label="Public dashboards">
        <InlineField
          label="Mesh data only"
          labelWidth={28}
          tooltip="Refuses the server health, endpoint and MQTT query types and drops memory, version and commit from stats. Anyone who can view a dashboard can send any query to its datasource, so hiding panels is not enough."
        >
          <InlineSwitch
            id="config-editor-mesh-only"
            value={!!jsonData.meshOnly}
            onChange={(e) => setJson({ meshOnly: e.currentTarget.checked })}
          />
        </InlineField>
      </FieldSet>

      <FieldSet label="Advanced">
        <InlineField label="Skip TLS verification" labelWidth={28}>
          <InlineSwitch
            id="config-editor-tls-skip"
            value={!!jsonData.tlsSkipVerify}
            onChange={(e) => setJson({ tlsSkipVerify: e.currentTarget.checked })}
          />
        </InlineField>
        {jsonData.tlsSkipVerify && (
          <Alert severity="warning" title="Certificates are not checked">
            Anyone on the network path can impersonate the server and read credentials. Use only for testing.
          </Alert>
        )}
        <InlineField label="Allow credentials over plain HTTP" labelWidth={28}>
          <InlineSwitch
            id="config-editor-insecure-credentials"
            value={!!jsonData.allowInsecureCredentials}
            onChange={(e) => setJson({ allowInsecureCredentials: e.currentTarget.checked })}
          />
        </InlineField>
        {jsonData.allowInsecureCredentials && (
          <Alert severity="warning" title="Credentials are sent unencrypted">
            Passwords and tokens sent to an http:// URL can be read by anyone on the network.
          </Alert>
        )}
      </FieldSet>
    </>
  );
}
