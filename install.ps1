param(
    [string]$BinDir = (Join-Path $HOME '.local/bin'),
    [switch]$NoProfile
)
$ErrorActionPreference = 'Stop'
if (-not [System.IO.Path]::IsPathRooted($BinDir)) { throw 'BinDir must be absolute' }
$extension = if ($IsWindows -or $PSVersionTable.PSEdition -eq 'Desktop') { '.exe' } else { '' }
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Push-Location $PSScriptRoot
try {
    if (Get-Command mise -ErrorAction SilentlyContinue) {
        mise exec -- go build -trimpath -o (Join-Path $BinDir "easyvenv$extension") ./cmd/easyvenv
    } else {
        go build -trimpath -o (Join-Path $BinDir "easyvenv$extension") ./cmd/easyvenv
    }
    if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
} finally { Pop-Location }
Copy-Item (Join-Path $BinDir "easyvenv$extension") (Join-Path $BinDir "venv$extension") -Force
if (-not $NoProfile) {
    $binary = (Join-Path $BinDir "easyvenv$extension").Replace("'", "''")
    $line = "(& '$binary' init pwsh) | Out-String | Invoke-Expression # easyvenv shell integration"
    New-Item -ItemType Directory -Force -Path (Split-Path $PROFILE -Parent) | Out-Null
    if (-not (Test-Path $PROFILE) -or -not ((Get-Content $PROFILE) -contains $line)) {
        Add-Content -Path $PROFILE -Value "`n$line"
    }
}
Write-Output "Installed easyvenv and venv in $BinDir. Add this directory to PATH for scripts; restart PowerShell to load shell integration."
