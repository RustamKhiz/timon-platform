param(
    [string]$OutputPath = ".local\keepgo-sandbox-catalog.json"
)

$ErrorActionPreference = "Stop"
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$resolvedOutput = Join-Path $repositoryRoot $OutputPath
$outputDirectory = Split-Path -Parent $resolvedOutput

if (-not (Test-Path -LiteralPath $outputDirectory)) {
    New-Item -ItemType Directory -Path $outputDirectory | Out-Null
}

$apiKeySecure = Read-Host "Keepgo sandbox API key" -AsSecureString
$accessTokenSecure = Read-Host "Keepgo sandbox access token" -AsSecureString

try {
    $env:KEEPGO_BASE_URL = "https://myaccount.sandbox.keepgo.com/api/v2"
    $env:KEEPGO_API_KEY = [System.Net.NetworkCredential]::new("", $apiKeySecure).Password
    $env:KEEPGO_ACCESS_TOKEN = [System.Net.NetworkCredential]::new("", $accessTokenSecure).Password

    Push-Location $repositoryRoot
    try {
        & go run ./cmd/keepgo-catalog | Set-Content -LiteralPath $resolvedOutput -Encoding UTF8
        if ($LASTEXITCODE -ne 0) {
            throw "keepgo-catalog failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    Write-Host "Sandbox catalogue saved to $resolvedOutput"
}
finally {
    Remove-Item Env:KEEPGO_API_KEY -ErrorAction SilentlyContinue
    Remove-Item Env:KEEPGO_ACCESS_TOKEN -ErrorAction SilentlyContinue
    Remove-Item Env:KEEPGO_BASE_URL -ErrorAction SilentlyContinue
    $apiKeySecure = $null
    $accessTokenSecure = $null
}
