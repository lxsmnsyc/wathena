// Passkey buttons for the account pages. It is the only script of the
// account service, and only the login, reauth and settings pages load
// it. The pages work without it, except for passkeys.
//
// It turns the server's WebAuthn options into what
// navigator.credentials wants, and the browser's answer back into JSON.
// The server keeps the challenge, so this file holds no state.
'use strict';

(() => {
  const box = document.querySelector('[data-passkey]');
  if (box === null || typeof window.PublicKeyCredential === 'undefined') {
    return;
  }
  const kind = box.dataset.passkey;
  const button = box.querySelector('button');
  const error = box.querySelector('.passkey-error');
  const nameInput = box.querySelector('input[name=passkey_name]');
  box.hidden = false;

  const fromB64 = (s) => {
    const b64 = s.replace(/-/g, '+').replace(/_/g, '/');
    const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4));
    return Uint8Array.from(bin, (c) => c.charCodeAt(0)).buffer;
  };
  const toB64 = (buf) => {
    if (buf === null || buf === undefined) {
      return undefined;
    }
    let bin = '';
    for (const b of new Uint8Array(buf)) {
      bin += String.fromCharCode(b);
    }
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  };

  const show = (text) => {
    error.textContent = text;
    error.hidden = false;
  };

  // post sends JSON with the page's CSRF token, and returns the answer.
  const post = async (path, body, query) => {
    const url = new URL(path, location.href);
    for (const [k, v] of Object.entries(query ?? {})) {
      if (v) {
        url.searchParams.set(k, v);
      }
    }
    const res = await fetch(url, {
      method: 'POST',
      credentials: 'same-origin',
      cache: 'no-store',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': box.dataset.csrf },
      body: JSON.stringify(body),
    });
    const json = await res.json().catch(() => ({}));
    if (json.error === 'reauth' && typeof json.next === 'string') {
      location.assign(json.next);
      throw new Error('');
    }
    if (!res.ok) {
      throw new Error(json.message || 'The server cannot do this right now. Try again later.');
    }
    return json;
  };

  const register = async () => {
    const options = await post('/account/passkeys/register/begin', { name: nameInput?.value ?? '' });
    const pk = options.publicKey;
    pk.challenge = fromB64(pk.challenge);
    pk.user.id = fromB64(pk.user.id);
    for (const c of pk.excludeCredentials ?? []) {
      c.id = fromB64(c.id);
    }
    const cred = await navigator.credentials.create({ publicKey: pk });
    const r = cred.response;
    const done = await post('/account/passkeys/register/finish', {
      id: cred.id,
      rawId: toB64(cred.rawId),
      type: cred.type,
      authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
      clientExtensionResults: cred.getClientExtensionResults(),
      response: {
        clientDataJSON: toB64(r.clientDataJSON),
        attestationObject: toB64(r.attestationObject),
        transports: typeof r.getTransports === 'function' ? r.getTransports() : [],
      },
    });
    location.assign(done.next);
  };

  const login = async () => {
    const options = await post('/account/passkeys/login/begin', { purpose: kind });
    const pk = options.publicKey;
    pk.challenge = fromB64(pk.challenge);
    for (const c of pk.allowCredentials ?? []) {
      c.id = fromB64(c.id);
    }
    const cred = await navigator.credentials.get({ publicKey: pk });
    const r = cred.response;
    const done = await post(
      '/account/passkeys/login/finish',
      {
        id: cred.id,
        rawId: toB64(cred.rawId),
        type: cred.type,
        authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
        clientExtensionResults: cred.getClientExtensionResults(),
        response: {
          clientDataJSON: toB64(r.clientDataJSON),
          authenticatorData: toB64(r.authenticatorData),
          signature: toB64(r.signature),
          userHandle: toB64(r.userHandle),
        },
      },
      { return: box.dataset.return, next: box.dataset.next },
    );
    location.assign(done.next);
  };

  button.addEventListener('click', async () => {
    button.disabled = true;
    error.hidden = true;
    try {
      await (kind === 'register' ? register() : login());
    } catch (e) {
      if (e instanceof DOMException && e.name === 'NotAllowedError') {
        show('The passkey prompt was closed or timed out.');
      } else if (e instanceof DOMException && e.name === 'InvalidStateError') {
        show('This passkey is already on this account.');
      } else if (e instanceof Error && e.message !== '') {
        show(e.message);
      }
    } finally {
      button.disabled = false;
    }
  });
})();
