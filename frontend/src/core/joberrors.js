// joberrors.js holds the logic of the transfer error list (board issue #15), kept apart from jobs.js so it can be
// tested without the page: which files failed (from the engine's Progress.failed, or the plain sentences of job engines
// that have no paths), the text that "Copy list" and "Save log" produce, the new jobs that "Retry failed" starts, and
// the rules of the "Skip errors in background" setting (fileops setting skipErrorsBackground, off by default, so a job
// keeps asking Retry / Skip as it always did unless the user turns it on).

// failuresOf is the list of failures of a finished job: [{source, target, reason, message}]. Job engines that only
// give sentences (archives, servers) yield entries with just the message.
export function failuresOf(p) {
  if (Array.isArray(p?.failed) && p.failed.length) return p.failed.map((f) => ({ source: f.source || '', target: f.target || '', reason: f.reason || '', message: f.message || '' }));
  return (p?.errors || []).map((m) => ({ source: '', target: '', reason: '', message: m }));
}

// errorCount is how many errors a job has had so far (the engine's uncapped count when it gives one).
export function errorCount(p) {
  return Math.max(p?.errorCount || 0, p?.errors?.length || 0);
}

// dirOf and baseOf split a path at its last / or \ (both kinds, since a job may run on Windows paths).
export function dirOf(p) {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'));
  if (i < 0) return '';
  if (i === 0) return p.slice(0, 1);
  if (i === 2 && /^[A-Za-z]:/.test(p)) return p.slice(0, 3); // "C:\x" -> "C:\"
  return p.slice(0, i);
}
export function baseOf(p) {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'));
  return p.slice(i + 1);
}
// inside says whether path p is under folder dir (not dir itself).
function inside(p, dir) {
  const d = dir.replace(/[\\/]+$/, '');
  return p.length > d.length + 1 && p.startsWith(d) && /[\\/]/.test(p[d.length]);
}

// line is one failure as a line of text: "path: reason", or the sentence when there is no path.
function line(f) {
  if (!f.source && !f.target) return f.message;
  return `${f.source || f.target}: ${f.reason || f.message}`;
}

// listText is what "Copy list" puts on the clipboard: one failed path per line with its reason.
export function listText(failures) {
  return failures.map(line).join('\n');
}

const KINDS = { copy: 'Copy', move: 'Move', delete: 'Delete', trash: 'Move to the trash', wipe: 'Wipe' };

// logText is what "Save log" writes: a heading with the job and when it ended, the summary, and every failure with
// its full sentence and paths.
export function logText(p, failures, when = new Date()) {
  const pad = (n) => String(n).padStart(2, '0');
  const stamp = `${when.getFullYear()}-${pad(when.getMonth() + 1)}-${pad(when.getDate())} ${pad(when.getHours())}:${pad(when.getMinutes())}:${pad(when.getSeconds())}`;
  const out = [`${KINDS[p?.kind] || 'Job'}${p?.target ? ` to ${p.target}` : ''}, ended ${stamp}`];
  if (p?.summary) out.push(p.summary);
  out.push('');
  for (const f of failures) {
    out.push(f.message || line(f));
    if (f.source) out.push(`  source: ${f.source}`);
    if (f.target) out.push(`  target: ${f.target}`);
  }
  const more = errorCount(p) - failures.length;
  if (more > 0) out.push('', `…and ${more} more errors that were not listed.`);
  return out.join('\n');
}

// logName is the file name "Save log" suggests: "copy-errors-2026-11-14.txt".
export function logName(p, when = new Date()) {
  const pad = (n) => String(n).padStart(2, '0');
  return `${p?.kind || 'job'}-errors-${when.getFullYear()}-${pad(when.getMonth() + 1)}-${pad(when.getDate())}.txt`;
}

// retryRequests turns the failures of a job started with req into the requests "Retry failed" starts, with all of
// req's options kept. Copies and moves go to where each failed file was going: the files bound for one folder under
// their own names make one job, a file that was being renamed gets a job of its own. A failure inside a failed folder
// is left out (the folder brings it along). Failures that can't be retried by themselves (a moved file whose copy
// worked but whose original could not be removed, or sentences without paths) are counted in skipped. A failure of
// the whole job (no source: the target folder could not be made) retries the whole request.
export function retryRequests(req, failures) {
  if (!req) return { requests: [], skipped: failures.length };
  const transfer = req.kind === 'copy' || req.kind === 'move';
  let skipped = 0;
  const usable = [];
  for (const f of failures) {
    if (!f.source) {
      if (f.target && transfer) return { requests: [{ ...req }], skipped: 0 };
      skipped++;
    } else if (transfer && !f.target) skipped++;
    else usable.push(f);
  }
  const kept = usable.filter((f, i) => !usable.some((g, k) => k !== i && (inside(f.source, g.source) || (g.source === f.source && k < i))));
  if (!transfer) return kept.length ? { requests: [{ ...req, sources: kept.map((f) => f.source) }], skipped } : { requests: [], skipped };
  const groups = new Map(); // target folder -> sources keeping their names
  const requests = [];
  for (const f of kept) {
    const dir = dirOf(f.target), name = baseOf(f.target);
    if (!dir) { skipped++; continue; }
    if (name === baseOf(f.source)) {
      if (!groups.has(dir)) groups.set(dir, []);
      groups.get(dir).push(f.source);
    } else {
      requests.push({ ...req, sources: [f.source], target: dir, rename: name });
    }
  }
  const grouped = [...groups].map(([dir, sources]) => ({ ...req, sources, target: dir, rename: '' }));
  return { requests: [...grouped, ...requests], skipped };
}

// onErrorFor is the error rule a new copy or move starts with: the copy dialog's "Don't stop on files that can't
// be read" as before, and with the "Skip errors in background" setting on, every background job skips too.
export function onErrorFor({ ignoreErrors = false, background = false, skipErrorsBackground = false } = {}) {
  return ignoreErrors || (background && skipErrorsBackground) ? '' : 'ask';
}

// listAtEnd says whether the error list opens by itself when a job ends: only with the setting on, only for a job
// that ran in the background or skipped errors without asking, and only when something failed.
export function listAtEnd({ skipErrorsBackground = false, background = false, onError = 'ask', final = null } = {}) {
  if (!skipErrorsBackground || !final || final.state === 'cancelled') return false;
  if (!(background || onError === '')) return false;
  return errorCount(final) > 0;
}
