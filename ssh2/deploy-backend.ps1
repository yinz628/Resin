param(
    [string]$TargetHost = $(if ($env:DEPLOY_SSH_HOST) { $env:DEPLOY_SSH_HOST } else { '107.148.164.86' }),
    [int]$Port = $(if ($env:DEPLOY_SSH_PORT) { [int]$env:DEPLOY_SSH_PORT } else { 22 }),
    [string]$User = $(if ($env:DEPLOY_SSH_USER) { $env:DEPLOY_SSH_USER } else { 'root' }),
    [string]$RemotePath = $(if ($env:DEPLOY_REMOTE_PATH) { $env:DEPLOY_REMOTE_PATH } else { '/opt/Resin' }),
    [string]$Service = $(if ($env:DEPLOY_SERVICE) { $env:DEPLOY_SERVICE } else { 'resin' }),
    [int]$Timeout = $(if ($env:DEPLOY_TIMEOUT) { [int]$env:DEPLOY_TIMEOUT } else { 15 }),
    [int]$HealthTimeout = $(if ($env:DEPLOY_HEALTH_TIMEOUT) { [int]$env:DEPLOY_HEALTH_TIMEOUT } else { 180 }),
    [string]$Password = $env:DEPLOY_SSH_PASSWORD,
    [switch]$KeepArchive
)

$scriptPath = Join-Path $PSScriptRoot 'deploy_backend.py'

if (-not (Test-Path -LiteralPath $scriptPath)) {
    throw "找不到脚本: $scriptPath"
}

$args = @(
    $scriptPath
    '--host', $TargetHost
    '--port', $Port
    '--user', $User
    '--remote-path', $RemotePath
    '--service', $Service
    '--timeout', $Timeout
    '--health-timeout', $HealthTimeout
)

if ($Password) {
    $args += @('--password', $Password)
}

if ($KeepArchive) {
    $args += '--keep-archive'
}

python @args
exit $LASTEXITCODE
