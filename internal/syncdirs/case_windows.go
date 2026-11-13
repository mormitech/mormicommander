// On Windows the usual file systems (NTFS, FAT, exFAT) ignore letter case,
// so "Report.TXT" on one side and "report.txt" on the other are the same file
// and CaseAuto pairs them up, exactly as Total Commander does.
package syncdirs

const defaultCaseInsensitive = true
