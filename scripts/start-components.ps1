param(
    [string]$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$ControllerListen = ":8090",
    [string]$ApiListen = ":8080",
    [string]$ControllerUrl = "localhost:8090",
    [string]$ApiEndpoint = "localhost:8080",
    [string]$ImageRoot = "images",
    [string]$WorkerToken = "worker-secret-token",
    [string]$SchedulerPoll = "250ms",
    [switch]$Hidden,
    [switch]$Stop,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

$LogRoot = Join-Path $ProjectRoot "logs"
$PidFile = Join-Path $LogRoot "components-pids.json"

function Write-Step {
    param([string]$Message)
    Write-Host "[components] $Message"
}

function Quote-PS {
    param([string]$Value)
    return "'" + ($Value -replace "'", "''") + "'"
}

function Get-Port {
    param([string]$Address)
    if ($Address -match ":(\d+)$") {
        return [int]$Matches[1]
    }
    throw "Cannot find port in address '$Address'"
}

function Wait-Port {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 30
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = New-Object Net.Sockets.TcpClient
        try {
            $client.Connect($HostName, $Port)
            $client.Close()
            return
        } catch {
            $client.Close()
            Start-Sleep -Milliseconds 250
        }
    }
    throw "Timed out waiting for $HostName`:$Port"
}

function Stop-StartedComponents {
    if (-not (Test-Path $PidFile)) {
        Write-Step "No pid file found at $PidFile"
        return
    }

    $entries = Get-Content $PidFile -Raw | ConvertFrom-Json
    foreach ($entry in @($entries) | Sort-Object order -Descending) {
        $process = Get-Process -Id $entry.pid -ErrorAction SilentlyContinue
        if ($process) {
            Write-Step "Stopping $($entry.name) pid=$($entry.pid)"
            & taskkill.exe /PID $entry.pid /T /F | Out-Null
        }
    }
}

function Start-Component {
    param(
        [int]$Order,
        [string]$Name,
        [string]$Command,
        [string]$WorkingDirectory
    )

    $stdout = Join-Path $LogRoot "$Name.out.log"
    $stderr = Join-Path $LogRoot "$Name.err.log"
    $fullCommand = "Set-Location -LiteralPath $(Quote-PS $WorkingDirectory); $Command"

    if ($Hidden) {
        $args = @("-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", $fullCommand)
        $process = Start-Process -FilePath "powershell.exe" -ArgumentList $args -WorkingDirectory $WorkingDirectory -RedirectStandardOutput $stdout -RedirectStandardError $stderr -WindowStyle Hidden -PassThru
    } else {
        $args = @("-NoLogo", "-NoExit", "-ExecutionPolicy", "Bypass", "-Command", $fullCommand)
        $process = Start-Process -FilePath "powershell.exe" -ArgumentList $args -WorkingDirectory $WorkingDirectory -PassThru
    }

    Write-Step "Started $Name pid=$($process.Id)"
    return [pscustomobject]@{
        order = $Order
        name = $Name
        pid = $process.Id
        stdout = $stdout
        stderr = $stderr
    }
}

if ($Stop) {
    Stop-StartedComponents
    return
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found on PATH."
}

New-Item -ItemType Directory -Force -Path $LogRoot | Out-Null

if ((Test-Path $PidFile) -and -not $Force) {
    $oldEntries = Get-Content $PidFile -Raw | ConvertFrom-Json
    $active = @($oldEntries | Where-Object { Get-Process -Id $_.pid -ErrorAction SilentlyContinue })
    if ($active.Count -gt 0) {
        throw "Some components from $PidFile are still running. Use -Stop first, or pass -Force."
    }
}

$imageRootFull = Join-Path $ProjectRoot $ImageRoot
New-Item -ItemType Directory -Force -Path $imageRootFull | Out-Null

Write-Step "Project root: $ProjectRoot"
Write-Step "Logs: $LogRoot"

$entries = @()
$entries += Start-Component -Order 1 -Name "controller" -WorkingDirectory $ProjectRoot -Command "go run ./controller --listen $ControllerListen --image-root $(Quote-PS $imageRootFull) --api-endpoint $ApiEndpoint --worker-api-token $WorkerToken"
Wait-Port -HostName "127.0.0.1" -Port (Get-Port $ControllerListen)

$entries += Start-Component -Order 2 -Name "scheduler" -WorkingDirectory $ProjectRoot -Command "go run ./scheduler --controller $ControllerUrl --poll $SchedulerPoll"

$workerDir = Join-Path $ProjectRoot "worker"
$entries += Start-Component -Order 3 -Name "worker-1" -WorkingDirectory $workerDir -Command "go run main.go --controller $ControllerUrl --worker-name worker-1 --tags cpu,fast"
$entries += Start-Component -Order 4 -Name "worker-2" -WorkingDirectory $workerDir -Command "go run main.go --controller $ControllerUrl --worker-name worker-2 --tags cpu,default"

$entries += Start-Component -Order 5 -Name "api" -WorkingDirectory $ProjectRoot -Command "go run ./api --listen $ApiListen --controller $ControllerUrl --image-root $(Quote-PS $imageRootFull) --worker-token $WorkerToken"
Wait-Port -HostName "127.0.0.1" -Port (Get-Port $ApiListen)

$entries | ConvertTo-Json -Depth 4 | Set-Content -Path $PidFile -Encoding UTF8

Write-Step "All components started."
Write-Step "PID file: $PidFile"
Write-Step "Stop them with: .\scripts\start-components.ps1 -Stop"
if ($Hidden) {
    Write-Step "Follow API logs with: Get-Content .\logs\api.out.log -Wait"
} else {
    Write-Step "Each component is running in its own PowerShell terminal window."
}
