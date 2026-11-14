// The connection manager's pure helpers (src/features/net.js, F-634, board issue #10): what the manager and the
// connection dialog say about where saved passwords are kept, the master password or the system keychain ("Keep saved
// passwords in the system keychain", off by default), and the message after the option is switched. No DOM here, so
// vitest checks it alone.

import { plural } from '../core/format.js';

// savePasswordLabel is the label of the "save the password" box of the connection dialog.
export function savePasswordLabel(list) {
  return list?.keychain ? 'Save the password (in the system keychain)' : 'Save the password (encrypted with the master password)';
}

// passwordStoreNote is the manager's line when no connection is selected: where saved passwords are.
export function passwordStoreNote(list) {
  if (list?.keychain) return 'Saved passwords are kept in the system keychain.';
  return list?.hasMaster ? 'Saved passwords are protected by the master password.' : '';
}

// sitePasswordNote is the part of a connection's line about its password ('' when none is saved).
export function sitePasswordNote(s) {
  if (!s?.hasPassword) return '';
  return s.inKeychain ? ' · password in the system keychain' : ' · password saved';
}

// keychainTitle is the tooltip of the keychain box: what it does, or why it can't be used here.
export function keychainTitle(list) {
  if (!list?.keychainAvailable) return list?.keychainNote || 'There is no system keychain mormiCommander can use here.';
  return 'Saved passwords go to the system keychain (unlocked by your login) instead of being encrypted with the master password. Switching moves the passwords already saved.';
}

// keychainResult is the message after the option was switched (r is Go's NetKeychainResult).
export function keychainResult(r) {
  const moved = r?.moved ? ` ${plural(r.moved, 'saved password')} ${r.moved === 1 ? 'was' : 'were'} moved.` : '';
  const missing = r?.missing ? ` ${plural(r.missing, 'password')} ${r.missing === 1 ? 'was' : 'were'} not in the keychain any more and will be asked when connecting.` : '';
  const head = r?.keychain ? 'Saved passwords are now kept in the system keychain.' : 'Saved passwords are now encrypted with the master password.';
  return head + moved + missing;
}
