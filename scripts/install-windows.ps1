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

function Test-WezTermInstalled {
    if ((Get-Command wezterm.exe -ErrorAction SilentlyContinue) -or
        (Get-Command wezterm-gui.exe -ErrorAction SilentlyContinue)) {
        return $true
    }

    $executables = @(
        (Join-Path $env:ProgramFiles "WezTerm\wezterm-gui.exe"),
        (Join-Path $env:ProgramFiles "WezTerm\wezterm.exe"),
        (Join-Path $env:LOCALAPPDATA "Programs\WezTerm\wezterm-gui.exe"),
        (Join-Path $env:LOCALAPPDATA "Programs\WezTerm\wezterm.exe")
    )
    if ($executables | Where-Object { Test-Path -LiteralPath $_ }) {
        return $true
    }

    $uninstallKeys = @(
        "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*",
        "HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*",
        "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*"
    )
    $installed = Get-ItemProperty -Path $uninstallKeys -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -like "WezTerm*" } |
        Select-Object -First 1
    return $null -ne $installed
}

function Install-WezTermFromRelease {
    Write-Host "Installing WezTerm from its verified official release..." -ForegroundColor Cyan

    [Net.ServicePointManager]::SecurityProtocol = `
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $headers = @{
        "Accept" = "application/vnd.github+json"
        "User-Agent" = "TermiReels-Windows-Installer"
    }
    $release = Invoke-RestMethod `
        -Uri "https://api.github.com/repos/wezterm/wezterm/releases/latest" `
        -Headers $headers
    $installerAsset = $release.assets |
        Where-Object { $_.name -match "^WezTerm-.+-setup\.exe$" } |
        Select-Object -First 1
    if (-not $installerAsset) {
        throw "The official WezTerm release does not contain a Windows setup executable."
    }
    $checksumName = "$($installerAsset.name).sha256"
    $checksumAsset = $release.assets |
        Where-Object { $_.name -eq $checksumName } |
        Select-Object -First 1
    if (-not $checksumAsset) {
        throw "The official WezTerm release does not contain $checksumName."
    }

    $tempRoot = Join-Path ([IO.Path]::GetTempPath()) `
        ("termireels-wezterm-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $tempRoot | Out-Null
    try {
        $installerPath = Join-Path $tempRoot $installerAsset.name
        $checksumPath = Join-Path $tempRoot $checksumAsset.name
        Invoke-WebRequest -UseBasicParsing `
            -Uri $installerAsset.browser_download_url -OutFile $installerPath
        Invoke-WebRequest -UseBasicParsing `
            -Uri $checksumAsset.browser_download_url -OutFile $checksumPath

        $checksumMatch = [Regex]::Match(
            (Get-Content -LiteralPath $checksumPath -Raw),
            "(?i)\b[0-9a-f]{64}\b"
        )
        if (-not $checksumMatch.Success) {
            throw "The official WezTerm checksum file is invalid."
        }
        $expectedHash = $checksumMatch.Value.ToLowerInvariant()
        $actualHash = (
            Get-FileHash -LiteralPath $installerPath -Algorithm SHA256
        ).Hash.ToLowerInvariant()
        if ($actualHash -ne $expectedHash) {
            throw "The downloaded WezTerm installer failed SHA-256 verification."
        }

        $process = Start-Process -FilePath $installerPath -ArgumentList @(
            "/VERYSILENT",
            "/SUPPRESSMSGBOXES",
            "/NORESTART",
            "/SP-"
        ) -Wait -PassThru
        if ($process.ExitCode -notin @(0, 3010)) {
            throw "The verified WezTerm installer failed with exit code $($process.ExitCode)."
        }
    }
    finally {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
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

if (Test-WezTermInstalled) {
    Write-Host "WezTerm is already installed." -ForegroundColor Green
}
else {
    $installedWithWinget = $false
    if (Get-Command winget.exe -ErrorAction SilentlyContinue) {
        Write-Host "Refreshing the WinGet community source..." -ForegroundColor Cyan
        & winget.exe source update --name winget
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "WinGet source update failed; resetting the default source."
            & winget.exe source reset --name winget --force
            if ($LASTEXITCODE -eq 0) {
                & winget.exe source update --name winget
            }
        }

        & winget.exe show --id wez.wezterm --exact --source winget `
            --accept-source-agreements
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Installing WezTerm with WinGet..." -ForegroundColor Cyan
            & winget.exe install --id wez.wezterm --exact --source winget --silent `
                --accept-package-agreements --accept-source-agreements
            $installedWithWinget = $LASTEXITCODE -eq 0
            if (-not $installedWithWinget) {
                Write-Warning `
                    "WinGet installation failed with exit code $LASTEXITCODE; using the verified fallback."
            }
        }
        else {
            Write-Warning `
                "WinGet could not resolve wez.wezterm; using the verified fallback."
        }
    }
    else {
        Write-Warning "WinGet is unavailable; using the verified fallback."
    }

    if (-not $installedWithWinget) {
        Install-WezTermFromRelease
    }
    if (-not (Test-WezTermInstalled)) {
        throw "WezTerm installation completed but the application could not be detected."
    }
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
