param(
    [string]$TargetHost = $(if ($env:SMOKE_SSH_HOST) { $env:SMOKE_SSH_HOST } else { '45.192.197.184' }),
    [int]$Port = $(if ($env:SMOKE_SSH_PORT) { [int]$env:SMOKE_SSH_PORT } else { 22 }),
    [string]$User = $(if ($env:SMOKE_SSH_USER) { $env:SMOKE_SSH_USER } else { 'root' }),
    [string]$RemotePath = $(if ($env:SMOKE_REMOTE_PATH) { $env:SMOKE_REMOTE_PATH } else { '/opt/' }),
    [string]$Password = $env:SMOKE_SSH_PASSWORD
)

$scriptPath = Join-Path $PSScriptRoot 'test_smoke_connection.py'

if (-not (Test-Path -LiteralPath $scriptPath)) {
    throw "找不到脚本: $scriptPath"
}

$args = @(
    $scriptPath
    '--host', $TargetHost
    '--port', $Port
    '--user', $User
    '--remote-path', $RemotePath
)

if ($Password) {
    $args += @('--password', $Password)
}

python @args
exit $LASTEXITCODE
