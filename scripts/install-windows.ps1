#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Distro = "Ubuntu-24.04"
)

$ErrorActionPreference = "Stop"
$installerUrl = "https://raw.githubusercontent.com/ademiru/TermiReels/main/scripts/install-wsl-release.sh"

function Test-Administrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Administrator)) {
    if (-not $PSCommandPath) {
        throw "Download this installer to a file before running it."
    }
    Write-Host "Requesting administrator permission..." -ForegroundColor Yellow
    $arguments = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", "`"$PSCommandPath`"",
        "-Distro", "`"$Distro`""
    )
    Start-Process powershell.exe -Verb RunAs -ArgumentList $arguments
    exit
}

if (-not (Get-Command winget.exe -ErrorAction SilentlyContinue)) {
    throw "winget is required. Install Microsoft App Installer, then run this installer again."
}

Write-Host "Installing WezTerm..." -ForegroundColor Cyan
& winget.exe install --id Wez.WezTerm --exact --silent `
    --accept-package-agreements --accept-source-agreements
if ($LASTEXITCODE -ne 0) {
    throw "WezTerm installation failed with exit code $LASTEXITCODE."
}

$wsl = Get-Command wsl.exe -ErrorAction SilentlyContinue
if (-not $wsl) {
    throw "This Windows version does not provide wsl.exe. Install current Windows updates first."
}

$wslList = & wsl.exe --list --quiet 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "Installing WSL2 and Ubuntu..." -ForegroundColor Cyan
    & wsl.exe --install -d $Distro
    Write-Host "Restart Windows, then run this same installer once more." -ForegroundColor Yellow
    exit
}
$distros = ($wslList | Out-String) -replace "`0", ""
if ($distros -notmatch "(?m)^$([Regex]::Escape($Distro))\s*$") {
    Write-Host "Installing the $Distro WSL distribution..." -ForegroundColor Cyan
    & wsl.exe --install -d $Distro
    if ($LASTEXITCODE -ne 0) {
        throw "Ubuntu installation failed with exit code $LASTEXITCODE."
    }
}

Write-Host "Ensuring $Distro uses WSL2..." -ForegroundColor Cyan
& wsl.exe --set-version $Distro 2
if ($LASTEXITCODE -ne 0) {
    throw "Could not configure $Distro as WSL2."
}

Write-Host ""
Write-Host "Ubuntu may now ask you to create a Linux username and password." -ForegroundColor Yellow
$linuxCommand = @'
set -euo pipefail
sudo apt-get update
sudo apt-get install -y ca-certificates curl
installer="$(mktemp)"
trap 'rm -f "$installer"' EXIT
curl --fail --location --silent --show-error "__INSTALLER_URL__" --output "$installer"
bash "$installer"
'@.Replace("__INSTALLER_URL__", $installerUrl)

& wsl.exe -d $Distro -- bash -lc $linuxCommand
if ($LASTEXITCODE -ne 0) {
    throw "TermiReels installation inside WSL failed with exit code $LASTEXITCODE."
}

Write-Host ""
Write-Host "Installation complete." -ForegroundColor Green
Write-Host "Open WezTerm and run:"
Write-Host "  wsl -d $Distro"
Write-Host "  termireels --login"
Write-Host "After login:"
Write-Host "  termireels --creator-provider"
