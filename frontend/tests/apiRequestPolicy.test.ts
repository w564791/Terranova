import assert from 'node:assert/strict';
import test from 'node:test';
import {
  isOrganizationScopedApiPath,
  isTrustedApiOrigin,
  isTrustedApiRequestUrl,
  resolveApiRequestUrl,
} from '../src/services/apiRequestPolicy.ts';
import { isValidAuthenticatedUserId } from '../src/services/authIdentity.ts';

test('marks dashboard, AI and CMDB routes as organization scoped', () => {
  for (const path of [
    '/dashboard/overview',
    '/api/v1/dashboard/compliance',
    '/ai/form/generate-with-cmdb-skill-sse',
    '/api/v1/ai/manifest/check-sse',
    '/cmdb/resources',
  ]) {
    assert.equal(isOrganizationScopedApiPath(path), true, path);
  }

  assert.equal(isOrganizationScopedApiPath('/auth/me'), false);
});

test('normalizes internal API paths to the configured API base URL', () => {
  const url = resolveApiRequestUrl(
    '/api/v1/ai/cmdb/search-summary-sse?mode=fast',
    'https://api.terranova.example/api/v1',
  );

  assert.equal(
    url.toString(),
    'https://api.terranova.example/api/v1/ai/cmdb/search-summary-sse?mode=fast',
  );
});

test('trusts only the configured API origin for apiFetch and axios bearer forwarding', () => {
  const apiBaseUrl = 'https://api.terranova.example/api/v1';

  const apiFetchExternalUrl = resolveApiRequestUrl(
    'https://untrusted.example/api/v1/ai/generate',
    apiBaseUrl,
  );
  assert.equal(isTrustedApiOrigin(apiFetchExternalUrl, apiBaseUrl), false);

  assert.equal(
    isTrustedApiOrigin(
      new URL('https://api.terranova.example/api/v1/dashboard/overview'),
      apiBaseUrl,
    ),
    true,
  );
  assert.equal(
    isTrustedApiOrigin(new URL('https://untrusted.example/ai/generate'), apiBaseUrl),
    false,
  );
  assert.equal(isTrustedApiRequestUrl('/dashboard/overview', apiBaseUrl), true);
  assert.equal(
    isTrustedApiRequestUrl('https://untrusted.example/api/v1/dashboard/overview', apiBaseUrl),
    false,
  );
});

test('accepts real SSO user identifiers without accepting placeholder values', () => {
  assert.equal(isValidAuthenticatedUserId('user-0001'), true);
  assert.equal(isValidAuthenticatedUserId('  42  '), true);
  assert.equal(isValidAuthenticatedUserId(42), true);
  assert.equal(isValidAuthenticatedUserId(''), false);
  assert.equal(isValidAuthenticatedUserId(0), false);
});
