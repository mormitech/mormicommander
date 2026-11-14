// The pure part of the first-run setup page's "Coming from Marta" box (board issue #22, F-NEW-22): whether the box is
// shown, what it says before the import and what it says after. It has no DOM and no Go, so the tests check it alone;
// features/setup.js draws the box and calls Setup.Marta / Setup.ImportMarta (feature_setup_marta.go).

// martaOffer says what the box shows for the answer of Setup.Marta: { show, text, canImport }. The box is shown only
// when Marta's settings folder was found.
export function martaOffer(info) {
  if (!info?.found) return { show: false, text: '', canImport: false };
  const n = info.favorites?.length || 0;
  if (info.error) return { show: true, text: info.error, canImport: false };
  if (!n) return { show: true, text: 'Marta is on this computer, but it has no favourite folders to bring over. The Marta-like keyboard preset above may still help.', canImport: false };
  const names = info.favorites.slice(0, 4).map((f) => f.name || f.path.split('/').filter(Boolean).pop() || f.path);
  const more = n > names.length ? `, and ${n - names.length} more` : '';
  return {
    show: true,
    canImport: true,
    text: `Marta is on this computer with ${n} favourite folder${n === 1 ? '' : 's'} (${names.join(', ')}${more}). Import them into the folder hotlist (Ctrl+D), in a "Marta" submenu. Marta's own files are only read, never changed.`,
  };
}

// martaDone is the sentence after an import: added is how many folders were added, total how many Marta had.
export function martaDone(added, total) {
  if (!added) return total ? 'Every Marta favourite is already in the folder hotlist.' : 'Nothing to import.';
  const skipped = total - added;
  return `Added ${added} Marta favourite${added === 1 ? '' : 's'} to the folder hotlist (Ctrl+D > Marta)${skipped > 0 ? `; ${skipped} ${skipped === 1 ? 'was' : 'were'} already there` : ''}.`;
}
