$ErrorActionPreference = 'Stop'

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$sourceDir = Join-Path $scriptDir 'dist'
$destDir = Join-Path $scriptDir '..\backend\internal\web\dist'

if (-not (Test-Path -LiteralPath $sourceDir)) {
  Write-Host "[postbuild] source dist not found: $sourceDir"
  exit 0
}

New-Item -ItemType Directory -Force -Path $destDir | Out-Null

for ($attempt = 1; $attempt -le 8; $attempt++) {
  try {
    Copy-Item -LiteralPath (Join-Path $sourceDir '*') -Destination $destDir -Recurse -Force
    Write-Host "[postbuild] copied build output from $sourceDir to $destDir"
    exit 0
  } catch {
    if ($attempt -eq 8) {
      throw
    }
    Start-Sleep -Milliseconds (250 * $attempt)
  }
}
