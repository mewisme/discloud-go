# discloud-cli installer for Windows (PowerShell)
#
# irm https://get.mewis.me/discloud.ps1 | iex
# irm https://raw.githubusercontent.com/mewisme/discloud-go/main/install.ps1 | iex
#
# Environment:
#   DISCLOUD_VERSION      release tag (default: latest)
#   DISCLOUD_INSTALL_DIR  install location (default: %LOCALAPPDATA%\discloud)
#   DISCLOUD_BASE         API origin (default: http://localhost:8080)
#   DISCLOUD_ORIGIN       web origin / CORS (default: http://localhost:3000)

$ErrorActionPreference = 'Stop'
$repo = 'mewisme/discloud-go'
$installDir = if ($env:DISCLOUD_INSTALL_DIR) { $env:DISCLOUD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'discloud' }

$DefaultBase = 'http://localhost:8080'
$DefaultOrigin = 'http://localhost:3000'

$arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
  'Arm64' { 'arm64' }
  'X64' { 'amd64' }
  default { throw "discloud-cli: unsupported architecture '$_'." }
}

$version = $env:DISCLOUD_VERSION
if (-not $version) {
  $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
if (-not $version) { throw "discloud-cli: could not resolve latest version; set DISCLOUD_VERSION." }
if ($version -notmatch '^v') { $version = "v$version" }
$ver = $version.TrimStart('v')

$url = "https://github.com/$repo/releases/download/$version/discloud-cli_${ver}_windows_${arch}.zip"
Write-Host "Installing discloud-cli $version (windows/$arch)..."

$tmp = Join-Path $env:TEMP ("discloud-cli-" + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$zip = Join-Path $tmp 'discloud-cli.zip'
Invoke-WebRequest -Uri $url -OutFile $zip

$dest = Join-Path $installDir 'current'
if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Expand-Archive -Path $zip -DestinationPath $dest -Force
Remove-Item -Recurse -Force $tmp

$exe = Join-Path $dest 'discloud-cli.exe'
if (-not (Test-Path $exe)) { throw "discloud-cli: discloud-cli.exe missing from archive." }

# Alias shim: discloud.cmd → discloud-cli.exe
$shim = Join-Path $dest 'discloud.cmd'
@"
@echo off
"%~dp0discloud-cli.exe" %*
"@ | Set-Content -Path $shim -Encoding ASCII

# Persist API defaults for the CLI (env still wins when set).
$cfgDir = Join-Path $env:APPDATA 'discloud'
New-Item -ItemType Directory -Force -Path $cfgDir | Out-Null
$baseOut = if ($env:DISCLOUD_BASE) { $env:DISCLOUD_BASE } else { $DefaultBase }
$originOut = if ($env:DISCLOUD_ORIGIN) { $env:DISCLOUD_ORIGIN } else { $DefaultOrigin }
$cfgPath = Join-Path $cfgDir 'config.yaml'
@"
base: $baseOut
origin: $originOut
"@ | Set-Content -Path $cfgPath -Encoding UTF8
Write-Host "Wrote $cfgPath (base=$baseOut origin=$originOut)"

$defaultInstall = Join-Path $env:LOCALAPPDATA 'discloud'
# Only mutate user PATH for the default install location.
if ($installDir -eq $defaultInstall) {
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($userPath -split ';') -notcontains $dest) {
    [Environment]::SetEnvironmentVariable('Path', "$dest;$userPath", 'User')
    $env:Path = "$dest;$env:Path"
    Write-Host "Added $dest to your PATH (restart your terminal if needed)."
  }
}

Write-Host "Installed to $dest"
Write-Host "Run: discloud version   # or: discloud-cli version"
