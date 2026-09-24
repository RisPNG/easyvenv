param(
    [string]$BinDir = (Join-Path $HOME '.local/bin'),
    [switch]$NoProfile
)
$ErrorActionPreference = 'Stop'
if (-not [System.IO.Path]::IsPathRooted($BinDir)) { throw 'BinDir must be absolute' }
if (-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    throw 'Use install.sh on Linux or macOS.'
}
$architecture = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
    'X64' { 'amd64' }
    'Arm64' { 'arm64' }
    default { throw 'easyvenv supports x64 and arm64 Windows.' }
}
$asset = "easyvenv-windows-$architecture.zip"
$releaseUrl = 'https://github.com/RisPNG/easyvenv/releases/latest/download'
$temporaryDir = Join-Path ([System.IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $temporaryDir | Out-Null
try {
    $archive = Join-Path $temporaryDir $asset
    $checksums = Join-Path $temporaryDir 'checksums.txt'
    Invoke-WebRequest -Uri "$releaseUrl/$asset" -OutFile $archive
    Invoke-WebRequest -Uri "$releaseUrl/checksums.txt" -OutFile $checksums
    $entry = Get-Content $checksums | Where-Object { ($_ -split '\s+')[1] -eq $asset } | Select-Object -First 1
    if (-not $entry) { throw "No checksum was published for $asset." }
    $expected = ($entry -split '\s+')[0]
    $actual = (Get-FileHash -Algorithm SHA256 $archive).Hash
    if ($actual -ne $expected) { throw "Checksum verification failed for $asset." }
    Expand-Archive -Path $archive -DestinationPath $temporaryDir
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    Copy-Item (Join-Path $temporaryDir 'easyvenv.exe') (Join-Path $BinDir 'easyvenv.exe') -Force
    Copy-Item (Join-Path $temporaryDir 'venv.exe') (Join-Path $BinDir 'venv.exe') -Force
} finally {
    Remove-Item $temporaryDir -Recurse -Force
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $BinDir) {
    [Environment]::SetEnvironmentVariable('Path', (($BinDir, $userPath) -join ';').TrimEnd(';'), 'User')
}
if (($env:PATH -split ';') -notcontains $BinDir) { $env:PATH = "$BinDir;$env:PATH" }

if (-not $NoProfile) {
    $binary = (Join-Path $BinDir 'easyvenv.exe').Replace("'", "''")
    $line = "(& '$binary' init pwsh) | Out-String | Invoke-Expression # easyvenv shell integration"
    New-Item -ItemType Directory -Force -Path (Split-Path $PROFILE -Parent) | Out-Null
    if (-not (Test-Path $PROFILE) -or -not ((Get-Content $PROFILE) -contains $line)) {
        Add-Content -Path $PROFILE -Value "`n$line"
    }
    Invoke-Expression ((& (Join-Path $BinDir 'easyvenv.exe') init pwsh) | Out-String)
    if ((Get-ExecutionPolicy) -in @('Restricted', 'AllSigned')) {
        Write-Warning "Your execution policy may block $PROFILE in new sessions. Run Set-ExecutionPolicy -Scope CurrentUser RemoteSigned to allow shell integration, or review Get-ExecutionPolicy -List if policy is managed."
    }
    Write-Output "Installed easyvenv and venv in $BinDir. Shell integration is loaded now and added to $PROFILE."
} else {
    Write-Output "Installed easyvenv and venv in $BinDir. Configure shell integration: https://github.com/RisPNG/easyvenv#manual-shell-setup"
}
