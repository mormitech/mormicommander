; Inno Setup script for mormiCommander. Per-user install (no admin rights), adds a Start menu entry, no startup entry.
; Silent install, as winget runs it:  mormicommander-setup-<version>-windows-x64.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART
; CI replaces __VERSION__ and compiles it (see .github/workflows/build.yml).
[Setup]
AppId={{3C9B6E1A-7D42-4F58-A0E3-5B1C8D2F6A97}
AppName=mormiCommander
AppVersion=__VERSION__
AppPublisher=mormitech
AppPublisherURL=https://github.com/mormitech/mormicommander
AppSupportURL=https://github.com/mormitech/mormicommander/issues
DefaultDirName={autopf}\mormicommander
DisableDirPage=yes
DisableProgramGroupPage=yes
DisableReadyPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist
OutputBaseFilename=mormicommander-setup-__VERSION__-windows-x64
UninstallDisplayName=mormiCommander
UninstallDisplayIcon={app}\mormicommander.exe
Compression=lzma2
SolidCompression=yes

[Files]
Source: "..\..\build\bin\mormicommander.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\mormiCommander"; Filename: "{app}\mormicommander.exe"

[Run]
Filename: "{app}\mormicommander.exe"; Description: "Start mormiCommander"; Flags: nowait postinstall skipifsilent

[Code]
// taskkill returns before Windows has released the file of the killed process, so wait a moment before the installer
// replaces or deletes it.
procedure StopMormicommander;
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM mormicommander.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(1500);
end;

// An upgrade replaces a file that may be in use, so close the running app first.
procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then StopMormicommander;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then StopMormicommander;
end;
