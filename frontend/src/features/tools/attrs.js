// Change attributes (cm_SetAttrib, F-194, F-315, F-316, F-338, F-340, F-683, F-684, F-685): Total Commander's dialog with
// three-state boxes (checked = set, unchecked = clear, grey = leave alone), "Process subfolders", date and time with a
// "Now" button, over internal/attrs (Tools.AttrInfo / Tools.SetAttrs). On Windows the box group is the attributes
// (read only, archive, hidden, system); on macOS and Linux it is the rwx grid with the octal field and the special
// bits, the Finder hidden flag on macOS, and owner and group (spec area-3 3.1, area-6 2.3).
//
// Only boxes the user changed are sent, so OK without touching a box never rewrites it, also deep in subfolders.
// Every three-state box has a hidden twin "<id>-state" holding "on", "off" or "mixed", so control mode can read and
// set the grey state too (answering the box itself with true/false sets on/off).

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { formDialog } from '../../core/dialogs.js';
import { plural } from '../../core/format.js';
import { esc, runTool, needLocal, targets, showError, failureList, reloadAll, deselect, pref, setPref, dateInput, parseDateInput } from './common.js';

const UNIX_ROWS = [['Owner', [0o400, 0o200, 0o100]], ['Group', [0o40, 0o20, 0o10]], ['Others', [0o4, 0o2, 0o1]]];
const SPECIAL = [[0o4000, 'Set user ID'], [0o2000, 'Set group ID'], [0o1000, 'Sticky']];
const HIDDEN_BIT = 0x2; // the Windows hidden attribute; on macOS the engine maps it to the Finder hidden flag

function tri(id, label, state, { disabled = false, aria = '' } = {}) {
  return `<label class="mc-check mc-tools-tri"><input type="checkbox" id="${id}" data-tri="${state}" data-init="${state}"${disabled ? ' disabled' : ''}${aria ? ` aria-label="${esc(aria)}"` : ''}>${label ? ` ${esc(label)}` : ''}</label><input type="hidden" id="${id}-state" value="${state}">`;
}

// setupTri makes the boxes cycle off → on (→ grey, for boxes that started grey) → off, and keeps the hidden twins.
function setupTri(box) {
  for (const el of box.querySelectorAll('input[data-tri]')) {
    const twin = box.querySelector(`#${CSS.escape(el.id + '-state')}`);
    const allowMixed = el.dataset.init === 'mixed';
    const show = (st, notify = true) => {
      el.dataset.tri = st;
      el.checked = st === 'on';
      el.indeterminate = st === 'mixed';
      twin.value = st;
      if (notify) el.dispatchEvent(new CustomEvent('tri', { bubbles: true }));
    };
    el.setTri = show;
    show(el.dataset.tri, false);
    let fromClick = false;
    el.addEventListener('click', () => {
      fromClick = true;
      const cur = el.dataset.tri;
      show(cur === 'off' ? 'on' : cur === 'on' ? (allowMixed ? 'mixed' : 'off') : 'off');
    });
    el.addEventListener('change', () => {
      if (fromClick) { fromClick = false; return; }
      show(el.checked ? 'on' : 'off');
    });
    twin.addEventListener('change', () => { if (['on', 'off', 'mixed'].includes(twin.value)) show(twin.value); });
  }
}

const octalOf = (box) => {
  let m = 0;
  for (const [bit] of [...SPECIAL, ...UNIX_ROWS.flatMap(([, bits]) => bits.map((b) => [b]))]) {
    const st = box.querySelector(`#perm-${bit.toString(8)}`)?.dataset.tri;
    if (st === 'mixed') return '';
    if (st === 'on') m |= bit;
  }
  return m.toString(8).padStart(4, '0');
};

function dateRow(id, label, ms, mixed, enabled = true) {
  return `<div class="mc-tools-date"><label class="mc-check"><input type="checkbox" id="set-${id}"${enabled ? '' : ' disabled'}> ${esc(label)}</label>
    <input type="datetime-local" step="1" id="${id}" value="${esc(dateInput(ms))}" disabled${mixed ? ' title="The selected items have different dates"' : ''}>
    <button type="button" class="mc-tools-now" data-for="${id}" disabled>Now</button></div>`;
}

command({
  name: 'cm_SetAttrib', label: 'Change attributes…',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (!paths.length) { app.flash('Nothing is selected.'); return; }
    let st;
    try { st = await go.Tools.AttrInfo(paths); } catch (e) { await showError(String(e?.message ?? e)); return; }
    const accounts = st.ownerChange ? go.Tools.AttrAccounts().catch(() => ({ users: [], groups: [] })) : null;
    const what = paths.length === 1 ? entries[0].name : `${plural(st.count, 'item')} (${plural(st.files + st.links, 'file')}, ${plural(st.folders, 'folder')})`;
    const lastDate = pref(app, 'attrLastDate', 0);
    const dateFor = (ms, mixed) => (mixed ? lastDate || Date.now() : ms || Date.now());

    let html = `<p class="mc-note mc-tools-what">${esc(what)}</p>
      <label class="mc-check"><input type="checkbox" id="recursive"${st.folders ? '' : ' disabled'}> Process subfolders</label>
      <div class="mc-tools-rec" hidden>
        <div class="mc-row"><label for="applyTo">Change</label><select id="applyTo"><option value="">Files and folders</option><option value="files">Files only</option><option value="folders">Folders only</option></select></div>
        <div class="mc-row"><label for="mask">Only names matching (for example *.sh | .git\\)</label><input type="text" id="mask" spellcheck="false" placeholder="* (all)"></div>
      </div>`;
    const bits = new Map((st.perm || []).map((b) => [b.bit, b]));
    if (st.unixPerms) {
      html += `<fieldset class="mc-tools-group"><legend>Permissions</legend><table class="mc-tools-perm"><tr><th></th><th>Read</th><th>Write</th><th>Execute</th></tr>`;
      for (const [who, row] of UNIX_ROWS) {
        html += `<tr><th>${who}</th>${row.map((b) => `<td>${tri(`perm-${b.toString(8)}`, '', bits.get(b)?.state || 'off', { aria: `${who} ${bits.get(b)?.name || ''}` })}</td>`).join('')}</tr>`;
      }
      html += `</table><div class="mc-tools-special">${SPECIAL.map(([b, n]) => tri(`perm-${b.toString(8)}`, n, bits.get(b)?.state || 'off')).join('')}</div>
        <div class="mc-row mc-tools-octal"><label for="octal">Octal</label><input type="text" id="octal" spellcheck="false" maxlength="4" value="${esc(st.octal)}" placeholder="mixed"></div>`;
      if (st.hiddenFlag) html += tri('hidden', 'Hidden (Finder hidden flag)', st.hidden);
      html += `</fieldset>`;
      if (st.ownerChange) {
        html += `<div class="mc-tools-owner"><div class="mc-row"><label for="owner">Owner</label><select id="owner"${st.isRoot ? '' : ' disabled title="Only an administrator can change the owner"'}><option value="">${esc(st.ownerMixed ? '(unchanged – they differ)' : `(unchanged: ${st.owner})`)}</option></select></div>
          <div class="mc-row"><label for="group">Group</label><select id="group"><option value="">${esc(st.groupMixed ? '(unchanged – they differ)' : `(unchanged: ${st.group})`)}</option></select></div></div>`;
      }
    }
    if (st.attrs?.length) {
      html += `<fieldset class="mc-tools-group"><legend>Attributes</legend><div class="mc-tools-attrs">${st.attrs.map((a) => tri(`attr-${a.bit}`, a.name, a.state, { disabled: !a.settable })).join('')}</div></fieldset>`;
    }
    html += `<fieldset class="mc-tools-group"><legend>Change date/time</legend>
      ${dateRow('modified', 'Modified', dateFor(st.modified, st.modifiedMixed), st.modifiedMixed)}
      ${st.createdTime ? dateRow('created', 'Created', dateFor(st.created, st.createdMixed), st.createdMixed) : ''}
      ${dateRow('accessed', 'Accessed', dateFor(st.accessed, st.accessedMixed), st.accessedMixed)}
      <div class="mc-btns mc-tools-left"><button type="button" class="mc-tools-other" title="Copy the date of the file under the cursor in the other panel">Date from other panel</button></div>
      <div class="mc-row"><label for="dateFrom">Modified date of each file from</label><select id="dateFrom"><option value="">(the date above)</option><option value="exif">its EXIF date (photos)</option><option value="name">a date in its name</option></select></div>
      </fieldset>`;

    const req = await formDialog(paths.length === 1 ? `Change attributes of ${entries[0].name}` : 'Change attributes', html, (box) => {
      const q = (s) => box.querySelector(s);
      const r = { paths, recursive: !!q('#recursive')?.checked, applyTo: '', mask: '', permSet: 0, permClear: 0, attrSet: 0, attrClear: 0,
        owner: '', group: '', modified: { mode: '' }, accessed: { mode: '' }, created: { mode: '' }, modifiedFrom: '' };
      if (r.recursive) { r.applyTo = q('#applyTo').value; r.mask = q('#mask').value.trim(); }
      // A box counts when the user changed it. With "Process subfolders", a box clicked back to its start value
      // still counts: it means "make everything inside like this", and files deep inside may differ.
      let changed = false;
      for (const el of box.querySelectorAll('input[data-tri]')) {
        const st = el.dataset.tri;
        if (st === 'mixed' || !el.dataset.touched || (st === el.dataset.init && !r.recursive)) continue;
        const on = st === 'on';
        changed = true;
        if (el.id.startsWith('perm-')) { const b = parseInt(el.id.slice(5), 8); if (on) r.permSet |= b; else r.permClear |= b; }
        else if (el.id.startsWith('attr-')) { const b = Number(el.id.slice(5)); if (on) r.attrSet |= b; else r.attrClear |= b; }
        else if (el.id === 'hidden') { if (on) r.attrSet |= HIDDEN_BIT; else r.attrClear |= HIDDEN_BIT; }
      }
      for (const k of ['owner', 'group']) { const v = q(`#${k}`)?.value || ''; if (v) { r[k] = v; changed = true; } }
      for (const k of ['modified', 'created', 'accessed']) {
        if (!q(`#set-${k}`)?.checked) continue;
        const ms = parseDateInput(q(`#${k}`).value);
        if (Number.isNaN(ms)) { q(`#${k}`).focus(); return 'The date or time is not valid. Use a date like 2026-10-14 and a time like 09:30:00.'; }
        r[k] = { mode: 'set', at: ms };
        changed = true;
      }
      // F-340: each file's own date (EXIF, or a date in its name); files without one keep the Modified setting
      const from = q('#dateFrom')?.value || '';
      if (from) { r.modifiedFrom = from; changed = true; }
      if (!changed) return { nothing: true };
      return r;
    }, (box) => {
      setupTri(box);
      const q = (s) => box.querySelector(s);
      const rec = q('#recursive'), recBox = q('.mc-tools-rec');
      rec.addEventListener('change', () => { recBox.hidden = !rec.checked; });
      box.addEventListener('tri', (ev) => {
        ev.target.dataset.touched = '1';
        if (ev.target.id.startsWith('perm-') && q('#octal')) q('#octal').value = octalOf(box);
      });
      q('#octal')?.addEventListener('input', () => {
        const v = q('#octal').value.trim();
        if (!/^[0-7]{3,4}$/.test(v)) return;
        const m = parseInt(v, 8);
        for (const el of box.querySelectorAll('input[id^="perm-"][data-tri]')) {
          const b = parseInt(el.id.slice(5), 8);
          el.dataset.touched = '1';
          el.setTri(m & b ? 'on' : 'off', false);
        }
      });
      for (const k of ['modified', 'created', 'accessed']) {
        const cb = q(`#set-${k}`);
        if (!cb) continue;
        const sync = () => { q(`#${k}`).disabled = !cb.checked; box.querySelector(`.mc-tools-now[data-for="${k}"]`).disabled = !cb.checked; };
        cb.addEventListener('change', sync);
        q(`#${k}`).addEventListener('input', () => { if (!cb.checked) { cb.checked = true; sync(); } });
        box.querySelector(`.mc-tools-now[data-for="${k}"]`).onclick = () => { q(`#${k}`).value = dateInput(Date.now()); };
      }
      q('.mc-tools-other').onclick = () => {
        const e = app.other.cur;
        if (!e || e.up) { app.flash('The other panel has no file under the cursor.'); return; }
        const cb = q('#set-modified');
        cb.checked = true;
        cb.dispatchEvent(new Event('change'));
        q('#modified').value = dateInput(e.mtime);
      };
      accounts?.then((a) => {
        const fill = (sel, names) => { if (sel) sel.insertAdjacentHTML('beforeend', names.map((n) => `<option value="${esc(n)}">${esc(n)}</option>`).join('')); };
        fill(q('#owner'), a.users || []);
        fill(q('#group'), a.groups || []);
      });
    }, { kind: 'attributes' });
    if (!req || req.nothing) return;
    if (req.modified.mode === 'set') setPref(app, 'attrLastDate', req.modified.at);

    let res;
    try { res = await runTool('Changing attributes', (id) => go.Tools.SetAttrs(id, req)); } catch (e) { await showError(e.message); return; }
    await reloadAll(app);
    const failed = new Set(res.failed.map((f) => f.path));
    deselect(p, entries.filter((e, i) => !failed.has(paths[i])).map((e) => e.name));
    if (res.canceled) app.flash(`Stopped. ${plural(res.changed, 'item')} changed.`);
    else if (res.failed.length) await failureList('Change attributes', res.failed, `${plural(res.changed, 'item')} changed, ${res.failed.length} not:`);
    else app.flash(res.changed ? `Changed ${plural(res.changed, 'item')}.` : 'Nothing needed a change.');
  },
});
