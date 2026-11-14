// Which names a copy or move unselects in the source panel when it ends. Total Commander takes the selection off
// every file it copied and leaves only the skipped or failed ones selected, so you can see at once what did not go
// across. The engine (internal/fileops) reports those sources in the final progress's "left" list; jobs that don't
// report it (servers, archives) keep the older rule: everything is unselected only when nothing was skipped or failed.

// copiedNames returns the names to unselect after a job. names[i] is the panel name of sources[i] (the full path sent
// to the engine); final is the job's final progress.
export function copiedNames(final, sources, names) {
  if (!final || (final.state !== 'done' && final.state !== 'failed')) return [];
  if (Array.isArray(final.left)) {
    const left = new Set(final.left);
    return names.filter((_, i) => !left.has(sources[i]));
  }
  if (final.state === 'done' && !final.skipped && !(final.errors || []).length) return names.slice();
  return [];
}
