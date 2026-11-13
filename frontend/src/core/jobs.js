// File jobs from the page's side (copy, move, delete, trash): start one in Go (feature_jobs.go, internal/fileops), show
// its progress in a dialog with a Cancel button, ask the overwrite question when a file is in the way, and resolve
// with the final state. Features for F5/F6/F8 call:
//
//   const p = await runJob({kind: 'copy', sources: [...full paths], target: '/folder', rename: '', overwrite: ''},
//                          {title: 'Copying'});
//   p.state is 'done', 'failed' (p.errors has sentences) or 'cancelled'; both panels are reread afterwards.
//
// The dialogs are page dialogs, so control mode reads and answers them ("dialog" / "dialog_answer"): the progress dialog
// has kind "job", the question kind "overwrite" with the buttons below.

import { go, on, errText } from './go.js';
import { modal, messageDialog, inputDialog, esc } from './dialogs.js';
import { size, date, plural } from './format.js';
import { track } from './busy.js';
import { app } from './app.js';

const jobs = new Map(); // id → {resolve, update(p), close()}
let listening = false;

function listen() {
  if (listening) return;
  listening = true;
  on('job', (p) => jobs.get(p.id)?.update(p));
  on('job-ask', (p) => { jobs.get(p.id)?.update(p); ask(p); });
  on('job-done', (p) => {
    const j = jobs.get(p.id);
    if (!j) return;
    jobs.delete(p.id);
    j.close();
    j.resolve(p);
  });
}

const BUTTONS = { Overwrite: 'overwrite', Skip: 'skip', 'Overwrite all': 'overwrite_all', 'Skip all': 'skip_all', 'Overwrite all older': 'older_all', 'Rename…': 'rename', Cancel: 'cancel' };

async function ask(p) {
  const c = p.ask;
  const name = c.target.split(/[\\/]/).pop();
  const msg = `${name} is already in the target folder.\n\nNew: ${size(c.srcSize)}, ${date(c.srcTime)}\nThere: ${size(c.dstSize)}, ${date(c.dstTime)}`;
  const b = await messageDialog({ Title: 'Overwrite?', Message: msg, Buttons: Object.keys(BUTTONS), DefaultButton: 'Overwrite', CancelButton: 'Cancel', Kind: 'overwrite' });
  const answer = { choice: BUTTONS[b] || 'cancel', newName: '' };
  if (answer.choice === 'rename') {
    const v = await inputDialog({ title: 'Copy under another name', label: 'New name', value: name, kind: 'rename-target' });
    if (v === null) answer.choice = 'skip';
    else answer.newName = v;
  }
  go.Jobs.Answer(p.id, answer).catch(() => {});
}

const VERB = { copy: 'Copying', move: 'Moving', delete: 'Deleting', trash: 'Moving to the trash' };

// runJob starts a job and resolves with its final Progress. Errors starting it reject with a sentence.
export function runJob(req, { title } = {}) {
  listen();
  return track((async () => {
    const id = await go.Jobs.Start(req).catch((e) => { throw new Error(errText(e)); });
    let p = { id, kind: req.kind, state: 'running', files: 0, filesDone: 0, bytes: 0, bytesDone: 0, current: '' };
    let close = () => {};
    let paint = () => {};
    const result = new Promise((resolve) => jobs.set(id, { resolve, update: (np) => { p = np; paint(); }, close: () => close() }));
    // the dialog only shows for jobs that take a moment, so a small copy doesn't flash a box
    const showTimer = setTimeout(() => {
      modal((box, done) => {
        close = () => done();
        box.innerHTML = `<h2>${esc(title || VERB[req.kind] || 'Working')}</h2><p class="mc-msg"></p><div class="mc-progress"><i></i></div><p class="mc-note"></p>
          <div class="mc-btns"><button type="button" class="cancel">Cancel</button></div>`;
        const msg = box.querySelector('.mc-msg'), bar = box.querySelector('.mc-progress i'), note = box.querySelector('.mc-note');
        paint = () => {
          msg.textContent = p.current ? p.current.split(/[\\/]/).pop() : '';
          const frac = p.bytes > 0 ? p.bytesDone / p.bytes : p.files > 0 ? p.filesDone / p.files : 0;
          bar.style.width = `${Math.round(Math.min(1, frac) * 100)}%`;
          note.textContent = `${p.filesDone} of ${plural(p.files, 'file')}${p.bytes ? ` · ${size(p.bytesDone)} of ${size(p.bytes)}` : ''}`;
        };
        paint();
        box.querySelector('.cancel').onclick = () => go.Jobs.Cancel(id);
        box.onsubmit = (ev) => ev.preventDefault();
      }, undefined, { kind: 'job' }).then(() => go.Jobs.Cancel(id).catch(() => {}));
    }, 400);
    const final = await result;
    clearTimeout(showTimer);
    close();
    await Promise.all(app.panels.map((pn) => pn.reload()));
    if (final.state === 'failed') app.flash(final.errors.slice(0, 3).join('\n') + (final.errors.length > 3 ? `\n…and ${final.errors.length - 3} more.` : ''), 'err');
    return final;
  })());
}
