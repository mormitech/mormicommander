// The default Lister settings, kept in a module of their own (no imports) so both the Lister (view.js) and the Options
// dialog's bridge to them (config/optbridge-features.js, board issue #94) use the same values, and the tests can load
// them in Node. The settings themselves are stored in the settings key "lister"; a key missing there means its default
// from this list, with Total Commander's defaults where it has the same setting.
export const LISTER_DEFAULTS = {
  defaultMode: 'auto', // auto, text, binary, hex
  defaultCharset: 'auto', // auto, ansi, oem, utf-8, unicode
  autoHex: true, // binary files open in hex mode (TC opens them as text)
  showMedia: true, // images, audio and video open in mode 4 (TC's BmpStartup / Multimedia)
  renderHTML: true, // HTML and Markdown open rendered
  openTables: false, // CSV, TSV, JSONL, Parquet and SQLite files open as a table (off: they open as before; T shows the table)
  wrap: true,
  tabWidth: 8,
  binaryWidth: 75,
  font: 'fixed', // fixed or variable (V)
  fontSize: 12,
  fitImage: true, // images larger than the window shrink to fit it (F toggles it)
  fitGrow: false, // with fitImage, smaller images are enlarged to the window too (Options > Lister > Image size)
  viewer: '', // external viewer for F3 ('' = Lister); Alt+F3 then opens the other one
  findHistory: [],
};
