// The admin page's script. It checks the staff role with /me, then
// loads the admin screens, a separate bundle, only for staff.

import './boot.css';
import { checkAccess } from './access';

const root = document.querySelector<HTMLElement>('#admin');
const status = document.querySelector<HTMLElement>('#admin-status');

/** Shows a line, with a link when url is set. */
function say(text: string, link?: { url: string; label: string }): void {
  if (status === null) {
    return;
  }
  status.textContent = text;
  if (link !== undefined) {
    const a = document.createElement('a');
    a.href = link.url;
    a.textContent = link.label;
    status.append(' ', a);
  }
}

async function boot(): Promise<void> {
  const access = await checkAccess();
  switch (access.kind) {
    case 'login':
      say('Log in first.', { url: access.url, label: 'Log in' });
      return;
    case 'two_factor':
      say(
        `Your account has the ${access.role} role, which needs two-factor login. Turn on an authenticator app or a passkey, then log in with its code, or with a passkey that checks your PIN or fingerprint.`,
        { url: access.url, label: 'Account settings' },
      );
      return;
    case 'none':
      say('This page is for staff.');
      return;
    case 'error':
      say(access.message);
      return;
    case 'staff': {
      say('Loading the admin tools.');
      const { start } = await import('./app');
      if (root !== null) {
        start(root, access.me);
      }
    }
  }
}

boot().catch(() => {
  say('The admin tools could not load. Reload the page.');
});
