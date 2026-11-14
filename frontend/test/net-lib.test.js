// The connection manager's pure helpers (src/features/net-lib.js, F-634, board issue #10): the texts about where saved
// passwords are kept (master password or system keychain) and the message after the keychain option is switched.
import { expect, test } from 'vitest';
import { savePasswordLabel, passwordStoreNote, sitePasswordNote, keychainTitle, keychainResult } from '../src/features/net-lib.js';

test('the save box says where the password goes', () => {
  expect(savePasswordLabel({ keychain: false })).toBe('Save the password (encrypted with the master password)');
  expect(savePasswordLabel(undefined)).toBe('Save the password (encrypted with the master password)');
  expect(savePasswordLabel({ keychain: true })).toBe('Save the password (in the system keychain)');
});

test('the manager line names the store', () => {
  expect(passwordStoreNote({ hasMaster: false })).toBe('');
  expect(passwordStoreNote({ hasMaster: true })).toBe('Saved passwords are protected by the master password.');
  expect(passwordStoreNote({ hasMaster: true, keychain: true })).toBe('Saved passwords are kept in the system keychain.');
  expect(sitePasswordNote({ hasPassword: false })).toBe('');
  expect(sitePasswordNote({ hasPassword: true })).toBe(' · password saved');
  expect(sitePasswordNote({ hasPassword: true, inKeychain: true })).toBe(' · password in the system keychain');
});

test('the keychain box explains itself, or why it is off', () => {
  expect(keychainTitle({ keychainAvailable: false, keychainNote: 'Install secret-tool.' })).toBe('Install secret-tool.');
  expect(keychainTitle({ keychainAvailable: false })).toMatch(/no system keychain/);
  expect(keychainTitle({ keychainAvailable: true })).toMatch(/Switching moves/);
});

test('the message after switching counts what moved and what was missing', () => {
  expect(keychainResult({ keychain: true, moved: 0 })).toBe('Saved passwords are now kept in the system keychain.');
  expect(keychainResult({ keychain: true, moved: 2 })).toBe('Saved passwords are now kept in the system keychain. 2 saved passwords were moved.');
  expect(keychainResult({ keychain: false, moved: 1, missing: 1 })).toBe(
    'Saved passwords are now encrypted with the master password. 1 saved password was moved. 1 password was not in the keychain any more and will be asked when connecting.');
});
