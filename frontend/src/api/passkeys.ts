import { apiClient } from './client';

export interface PasskeyInfo {
  id: number;
  name: string;
  created_at: string;
  last_used?: string;
}

// ─── WebAuthn binary encoding helpers ───────────────────────────────────────

function b64urlToUint8Array(b64: string): Uint8Array {
  const base64 = b64.replace(/-/g, '+').replace(/_/g, '/');
  const pad = '===='.slice(0, (4 - (base64.length % 4)) % 4);
  const binary = atob(base64 + pad);
  return Uint8Array.from(binary, c => c.charCodeAt(0));
}

function uint8ArrayToB64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

// ─── Prepare browser options (decode base64url → ArrayBuffer) ──────────────

function prepareCreationOptions(opts: any): PublicKeyCredentialCreationOptions {
  return {
    ...opts.publicKey,
    challenge: b64urlToUint8Array(opts.publicKey.challenge),
    user: {
      ...opts.publicKey.user,
      id: b64urlToUint8Array(opts.publicKey.user.id),
    },
    excludeCredentials: (opts.publicKey.excludeCredentials ?? []).map((c: any) => ({
      ...c,
      id: b64urlToUint8Array(c.id),
    })),
  };
}

function prepareRequestOptions(opts: any): PublicKeyCredentialRequestOptions {
  return {
    ...opts.publicKey,
    challenge: b64urlToUint8Array(opts.publicKey.challenge),
    allowCredentials: (opts.publicKey.allowCredentials ?? []).map((c: any) => ({
      ...c,
      id: b64urlToUint8Array(c.id),
    })),
  };
}

// ─── Serialize credential responses (ArrayBuffer → base64url) ──────────────

function serializeRegistrationCredential(cred: PublicKeyCredential): object {
  const resp = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    rawId: uint8ArrayToB64url(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: uint8ArrayToB64url(resp.attestationObject),
      clientDataJSON: uint8ArrayToB64url(resp.clientDataJSON),
    },
  };
}

function serializeAuthenticationCredential(cred: PublicKeyCredential): object {
  const resp = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: uint8ArrayToB64url(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: uint8ArrayToB64url(resp.authenticatorData),
      clientDataJSON: uint8ArrayToB64url(resp.clientDataJSON),
      signature: uint8ArrayToB64url(resp.signature),
      ...(resp.userHandle ? { userHandle: uint8ArrayToB64url(resp.userHandle) } : {}),
    },
  };
}

// ─── Registration flow ──────────────────────────────────────────────────────

export async function registerPasskey(name: string): Promise<void> {
  // 1. Get challenge from server
  const beginResp = await apiClient.post<{ options: any; session_id: string }>(
    '/auth/passkey/register/begin',
    {}
  );

  // 2. Call browser WebAuthn API
  const credential = await navigator.credentials.create({
    publicKey: prepareCreationOptions(beginResp.options),
  }) as PublicKeyCredential;

  if (!credential) throw new Error('Passkey creation cancelled');

  // 3. Send credential to server
  await apiClient.post('/auth/passkey/register/finish', {
    session_id: beginResp.session_id,
    name,
    credential: serializeRegistrationCredential(credential),
  });
}

// ─── Login flow ─────────────────────────────────────────────────────────────

export async function loginWithPasskey(): Promise<{ user: any; token: string }> {
  // 1. Get challenge from server
  const beginResp = await fetch('/api/auth/passkey/login/begin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({}),
  });
  if (!beginResp.ok) throw new Error('Failed to start passkey login');
  const beginData = await beginResp.json();

  // 2. Call browser WebAuthn API
  const credential = await navigator.credentials.get({
    publicKey: prepareRequestOptions(beginData.options),
  }) as PublicKeyCredential;

  if (!credential) throw new Error('Passkey authentication cancelled');

  // 3. Send assertion to server
  const finishResp = await fetch('/api/auth/passkey/login/finish', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      session_id: beginData.session_id,
      credential: serializeAuthenticationCredential(credential),
    }),
  });
  const finishData = await finishResp.json().catch(() => ({}));
  if (!finishResp.ok) throw new Error(finishData.message || 'Passkey authentication failed');
  return finishData;
}

// ─── Management ─────────────────────────────────────────────────────────────

export async function listPasskeys(): Promise<PasskeyInfo[]> {
  return apiClient.get<PasskeyInfo[]>('/auth/passkeys');
}

export async function deletePasskey(id: number): Promise<void> {
  return apiClient.delete<void>(`/auth/passkeys/${id}`);
}

export async function renamePasskey(id: number, name: string): Promise<void> {
  return apiClient.put<void>(`/auth/passkeys/${id}`, { name });
}
