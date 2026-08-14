; NexusDesktop Windows 安装器（Inno Setup 7）
; AppId 是覆盖升级的唯一依据，发版后不得更改。
#define AppIdGuid "{B8E4D6A2-3C71-4F9E-A5B0-1D7C8E9F2A34}"
#define MyAppName "NexusDesktop"
#define MyAppPublisher "byteyang"
#define MyAppURL "https://github.com/bytepine/NexusDesktop"
#define MyAppExeName "NexusDesktop.exe"

#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#ifndef MyAppExePath
  #define MyAppExePath "../release/NexusDesktop.exe"
#endif
#ifndef MyOutputDir
  #define MyOutputDir "../release"
#endif
#ifndef MyOutputBaseFilename
  #define MyOutputBaseFilename "NexusDesktop-setup"
#endif

[Setup]
AppId={#AppIdGuid}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
AppUpdatesURL={#MyAppURL}/releases
DefaultDirName={autopf}\{#MyAppName}
DisableProgramGroupPage=yes
LicenseFile=../LICENSE
OutputDir={#MyOutputDir}
OutputBaseFilename={#MyOutputBaseFilename}
#ifdef MySetupIcon
SetupIconFile={#MySetupIcon}
#endif
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
SetupArchitecture=x64
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
UsePreviousAppDir=yes
UsePreviousPrivileges=yes
UsePreviousTasks=yes
CloseApplications=no
RestartIfNeededByRun=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"

[CustomMessages]
english.AutostartTask=Start NexusDesktop when I log on
chinesesimplified.AutostartTask=开机时启动 NexusDesktop
english.DeleteUserData=Also delete settings and logs?%n(%%APPDATA%%\NexusDesktop)
chinesesimplified.DeleteUserData=是否同时删除配置与日志？%n（%%APPDATA%%\NexusDesktop）
english.UpgradeNotice=This will update NexusDesktop %1 → %2. Settings and logs will be kept.
chinesesimplified.UpgradeNotice=将更新 NexusDesktop %1 → %2。配置与日志会保留。
english.NewerAlreadyInstalled=A newer version of NexusDesktop (%1) is already installed. This package is %2.
chinesesimplified.NewerAlreadyInstalled=已安装更新的 NexusDesktop（%1），本安装包为 %2，已取消安装。
english.SwitchScopeHint=To switch between current-user and all-users install, uninstall first.
chinesesimplified.SwitchScopeHint=若要在「当前用户」与「全部用户」之间切换范围，请先卸载再安装。

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "autostart"; Description: "{cm:AutostartTask}"; Flags: unchecked

[Files]
Source: "{#MyAppExePath}"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Registry]
; 开机自启只写 HKCU，全部用户安装也不写 HKLM
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "NexusDesktop"; ValueData: """{app}\{#MyAppExeName}"""; Tasks: autostart

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[Code]
const
  UninstSubKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{#AppIdGuid}_is1';
  RunKey = 'Software\Microsoft\Windows\CurrentVersion\Run';
  RunValueName = 'NexusDesktop';

function GetUninstallString(): String;
begin
  Result := '';
  if not RegQueryStringValue(HKCU, UninstSubKey, 'UninstallString', Result) then
    RegQueryStringValue(HKLM, UninstSubKey, 'UninstallString', Result);
end;

function GetInstalledVersion(): String;
begin
  Result := '';
  if not RegQueryStringValue(HKCU, UninstSubKey, 'DisplayVersion', Result) then
    RegQueryStringValue(HKLM, UninstSubKey, 'DisplayVersion', Result);
end;

function IsUpgrade(): Boolean;
begin
  Result := GetUninstallString() <> '';
end;

procedure KillApp();
var
  ResultCode: Integer;
begin
  Exec('taskkill.exe', '/F /IM {#MyAppExeName} /T', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function InstalledExeNewerThanSetup(): Boolean;
var
  Location, ExePath, InstalledVerStr: String;
  InstalledVer, SetupVer: Int64;
begin
  Result := False;
  Location := '';
  if not RegQueryStringValue(HKCU, UninstSubKey, 'InstallLocation', Location) then
    RegQueryStringValue(HKLM, UninstSubKey, 'InstallLocation', Location);
  if Location = '' then
    Exit;
  ExePath := AddBackslash(Location) + '{#MyAppExeName}';
  if not FileExists(ExePath) then
    Exit;
  if not GetPackedVersion(ExePath, InstalledVer) then
    Exit;
  if not StrToVersion('{#MyAppVersion}', SetupVer) then
    Exit;
  if ComparePackedVersion(InstalledVer, SetupVer) > 0 then
  begin
    Result := True;
    InstalledVerStr := '';
    GetVersionNumbersString(ExePath, InstalledVerStr);
    if InstalledVerStr = '' then
      InstalledVerStr := '?';
    MsgBox(FmtMessage(CustomMessage('NewerAlreadyInstalled'), [InstalledVerStr, '{#MyAppVersion}']), mbInformation, MB_OK);
  end;
end;

function InitializeSetup(): Boolean;
begin
  Result := True;
  if InstalledExeNewerThanSetup() then
  begin
    Result := False;
    Exit;
  end;
  KillApp();
end;

function InitializeUninstall(): Boolean;
begin
  KillApp();
  Result := True;
end;

procedure InitializeWizard();
var
  PrevVer: String;
begin
  PrevVer := GetInstalledVersion();
  if PrevVer <> '' then
  begin
    WizardForm.WelcomeLabel2.Caption :=
      FmtMessage(CustomMessage('UpgradeNotice'), [PrevVer, '{#MyAppVersion}']) +
      Chr(13) + Chr(10) + Chr(13) + Chr(10) +
      CustomMessage('SwitchScopeHint') +
      Chr(13) + Chr(10) + Chr(13) + Chr(10) +
      WizardForm.WelcomeLabel2.Caption;
  end;
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := False;
  if IsUpgrade() then
  begin
    if PageID = wpSelectDir then
      Result := True;
    if PageID = wpSelectProgramGroup then
      Result := True;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  AppDataDir, FyneDir: String;
begin
  if CurUninstallStep <> usUninstall then
    Exit;
  { 升级时 Inno 会 /SILENT 跑旧卸载器：保留开机自启与用户配置 }
  if UninstallSilent then
    Exit;
  RegDeleteValue(HKCU, RunKey, RunValueName);
  if MsgBox(CustomMessage('DeleteUserData'), mbConfirmation, MB_YESNO) = IDYES then
  begin
    AppDataDir := ExpandConstant('{userappdata}\NexusDesktop');
    FyneDir := ExpandConstant('{userappdata}\fyne\com.bytepine.nexusdesktop');
    DelTree(AppDataDir, True, True, True);
    DelTree(FyneDir, True, True, True);
  end;
end;
