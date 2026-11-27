import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { ConfigEditor, validateUrl } from './ConfigEditor';

function setup(over: { jsonData?: object; secureJsonFields?: object; secureJsonData?: object } = {}) {
  const onOptionsChange = jest.fn();
  const options = {
    id: 1,
    uid: 'u',
    orgId: 1,
    name: 'n',
    typeLogoUrl: '',
    type: 't',
    typeName: 't',
    access: 'proxy',
    url: '',
    user: '',
    database: '',
    basicAuth: false,
    basicAuthUser: '',
    withCredentials: false,
    isDefault: false,
    readOnly: false,
    jsonData: {},
    secureJsonFields: {},
    ...over,
  };
  render(<ConfigEditor options={options as never} onOptionsChange={onOptionsChange} />);
  return { onOptionsChange, last: () => onOptionsChange.mock.calls[onOptionsChange.mock.calls.length - 1][0] };
}

describe('validateUrl', () => {
  it.each([
    ['', undefined],
    ['https://a.example', undefined],
    ['http://a.example:3000/x', undefined],
    ['ftp://a.example', 'must start'],
    ['analyzer.example', 'must start'],
    ['https://user:pw@a.example', 'user name'],
    ['https://user@a.example', 'user name'],
  ])('%s', (url, msg) => {
    const r = validateUrl(url);
    msg ? expect(r).toContain(msg) : expect(r).toBeUndefined();
  });
});

describe('ConfigEditor', () => {
  it('shows a URL error inline', () => {
    setup({ jsonData: { url: 'ftp://x' } });
    expect(screen.getByText(/must start with http/i)).toBeInTheDocument();
  });

  it('shows a required message after the URL is left empty', () => {
    setup();
    fireEvent.blur(screen.getByPlaceholderText('https://analyzer.example.net'));
    expect(screen.getByText('A URL is required.')).toBeInTheDocument();
  });

  it('rejects an out of range timeout', () => {
    setup({ jsonData: { url: 'https://a.b', timeoutSeconds: 99 } });
    expect(screen.getByText(/from 1 to 60/)).toBeInTheDocument();
  });

  it('starts on Basic when a user is set and clears it and the password on switching to Bearer', () => {
    const { last } = setup({
      jsonData: { url: 'https://a.b', basicAuthUser: 'bob' },
      secureJsonFields: { basicAuthPassword: true },
    });
    expect(screen.getByDisplayValue('bob')).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('Bearer token'));
    expect(last().jsonData.basicAuthUser).toBeUndefined();
    expect(last().secureJsonFields.basicAuthPassword).toBe(false);
    expect(last().secureJsonData.basicAuthPassword).toBe('');
    expect(screen.getByPlaceholderText('Bearer token')).toBeInTheDocument();
  });

  it('clears the bearer token when switching to None', () => {
    const { last } = setup({ jsonData: { url: 'https://a.b' }, secureJsonFields: { bearerToken: true } });
    fireEvent.click(screen.getByLabelText('None'));
    expect(last().secureJsonFields.bearerToken).toBe(false);
    expect(last().secureJsonData.bearerToken).toBe('');
  });

  it('resets a configured secret and never echoes it', () => {
    const { last } = setup({ jsonData: { url: 'https://a.b' }, secureJsonFields: { bearerToken: true } });
    expect(screen.getByDisplayValue('configured')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /reset/i }));
    expect(last().secureJsonFields.bearerToken).toBe(false);
    expect(last().secureJsonData.bearerToken).toBe('');
  });

  it('writes a typed password to secureJsonData only', () => {
    const { last } = setup({ jsonData: { url: 'https://a.b', basicAuthUser: 'bob' } });
    fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'x' } });
    expect(last().secureJsonData.basicAuthPassword).toBe('x');
    expect(JSON.stringify(last().jsonData)).not.toContain('x"');
  });

  it('warns when TLS verification is skipped', () => {
    setup({ jsonData: { url: 'https://a.b', tlsSkipVerify: true, allowInsecureCredentials: true } });
    expect(screen.getByText('Certificates are not checked')).toBeInTheDocument();
    expect(screen.getByText('Credentials are sent unencrypted')).toBeInTheDocument();
  });
});

it('stores the timeout as a whole number', () => {
  const { last } = setup({ jsonData: { url: 'https://a.b' } });
  fireEvent.change(screen.getByPlaceholderText('15'), { target: { value: '1.4' } });
  expect(last().jsonData.timeoutSeconds).toBe(1);
});

it('stores the mesh-only switch in jsonData', () => {
  const { last } = setup({ jsonData: { url: 'https://a.b' } });
  fireEvent.click(document.getElementById('config-editor-mesh-only')!);
  expect(last().jsonData.meshOnly).toBe(true);
});
