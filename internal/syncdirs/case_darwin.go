// On macOS the default APFS and HFS+ volumes ignore letter case, so CaseAuto
// pairs "Report.TXT" with "report.txt". Users syncing with a case-sensitive
// volume can pick CaseSensitive explicitly.
package syncdirs

const defaultCaseInsensitive = true
