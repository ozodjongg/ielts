$ErrorActionPreference = "Stop"
$failed = $false

function Need($cmd) {
  if (Get-Command $cmd -ErrorAction SilentlyContinue) {
    Write-Host "[ok] $cmd" -ForegroundColor Green
  } else {
    Write-Host "[missing] $cmd" -ForegroundColor Red
    $script:failed = $true
  }
}

Need "go"
Need "node"
Need "npm"
Need "python"

if (Get-Command psql -ErrorAction SilentlyContinue) {
  Write-Host "[ok] psql (local database CLI available)" -ForegroundColor Green
} else {
  Write-Host "[optional] psql not found; required only for native local database workflows" -ForegroundColor Yellow
}

python tools/qa_v5.py
if ($LASTEXITCODE -ne 0) { $failed = $true }

if ($failed) {
  Write-Host "Preflight FAILED" -ForegroundColor Red
  exit 1
}

Write-Host "Preflight PASSED (see QA_REPORT.md for build-environment limitations)" -ForegroundColor Green
