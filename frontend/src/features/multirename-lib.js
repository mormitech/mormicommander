// The pure parts of the Multi-Rename tool's page that tests can check without a window: the items of the "[=?] Field…"
// menu. Built-in fields (size, week day, ...) are inserted as [=tc.name], as before; the media fields read from the
// files themselves (EXIF date, image size, audio tags; board issue #7) follow after a separator and are inserted by
// their own name, such as [=img.exifdate] or [=audio.artist], like the content-plugin fields of Total Commander.

const MEDIA_HELP = {
  'img.exifdate': 'Date and time the photo was taken (EXIF)',
  'img.exifyear': 'Year the photo was taken (EXIF)',
  'img.exifmonth': 'Month the photo was taken (EXIF)',
  'img.exifday': 'Day the photo was taken (EXIF)',
  'img.exifhour': 'Hour the photo was taken (EXIF)',
  'img.exifminute': 'Minute the photo was taken (EXIF)',
  'img.exifsecond': 'Second the photo was taken (EXIF)',
  'img.width': 'Image width in pixels',
  'img.height': 'Image height in pixels',
  'img.size': 'Image size as width x height',
  'audio.artist': 'Artist (ID3 tag)',
  'audio.title': 'Title (ID3 tag)',
  'audio.album': 'Album (ID3 tag)',
  'audio.track': 'Track number, two digits (ID3 tag)',
  'audio.year': 'Year (ID3 tag)',
  'audio.genre': 'Genre (ID3 tag)',
};

// fieldMenuItems gives the menu as a list of { label, text, detail } (text is what gets inserted) and { sep: true }.
export function fieldMenuItems(fields = [], media = []) {
  const out = fields.map((f) => ({ label: `[=tc.${f}]`, text: `[=tc.${f}]`, detail: f }));
  if (media.length) {
    if (out.length) out.push({ sep: true });
    for (const f of media) out.push({ label: `[=${f}]`, text: `[=${f}]`, detail: MEDIA_HELP[f] || f });
  }
  return out;
}

// The AI rename plan (issue #33): the fields a local model may set, by the names of the form (readForm/writeForm) and
// of the plan (internal/ai.RenamePlan, whose `set` lists the ones it gave).
const PLAN_FIELDS = ['nameMask', 'extMask', 'search', 'replace', 'regex', 'caseSensitive', 'onlyFirst', 'includeExt', 'case'];

// applyRenamePlan gives the form with the plan's fields filled in: only the ones the plan set, so the counter, the
// accents box and the rest stay as the user has them. A plan that sets a search without saying regex turns RegEx off
// (its search is literal text), and Subst. goes off with RegEx. Nothing is renamed: the page only refreshes the
// preview.
export function applyRenamePlan(form, plan) {
  const out = { ...form };
  const set = new Set((plan?.set || []).filter((k) => PLAN_FIELDS.includes(k)));
  for (const k of set) out[k] = plan[k];
  if (set.has('search') && !set.has('regex')) out.regex = false;
  if (!out.regex) out.subst = false;
  return out;
}

// renamePlanEnabled says whether the AI plan button is shown: only when a local model address is set in Options > AI
// (empty by default, so the feature is hidden and nothing is ever contacted).
export function renamePlanEnabled(values) {
  return !!String(values?.['AI.LocalRenameURL'] ?? '').trim();
}
