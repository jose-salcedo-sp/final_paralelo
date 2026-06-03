param(
    [string]$ProjectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$ApiBaseUrl = "http://localhost:8080",
    [string]$Username = "username",
    [string]$Password = "password",
    [string]$FramesPath = "frames",
    [string]$FilteredPath = "filtered",
    [string]$OutputVideo = "filtered.mp4",
    [string]$VideoUrl = "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_720p_stereo.avi",
    [string]$VideoFile = "big_buck_bunny_720p_stereo.avi",
    [int]$PollTimeoutSeconds = 900,
    [switch]$SkipDownload,
    [switch]$SkipExtract,
    [switch]$SkipVenv,
    [switch]$CleanFiltered
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host "[loco] $Message"
}

function Resolve-ProjectPath {
    param([string]$Path)
    if ([IO.Path]::IsPathRooted($Path)) {
        return $Path
    }
    return Join-Path $ProjectRoot $Path
}

function Assert-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name was not found on PATH."
    }
}

function Invoke-JsonRequest {
    param(
        [string]$Method,
        [string]$Uri,
        [hashtable]$Headers
    )
    return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $Headers
}

Set-Location $ProjectRoot
Assert-Command "curl.exe"

$testsDir = Join-Path $ProjectRoot "Loco's tests"
$requirements = Join-Path $testsDir "requirements.txt"
$stressTest = Join-Path $testsDir "stress_test.py"
$videoUtils = Join-Path $testsDir "video_utils_windows.py"

if (-not (Test-Path $requirements)) { throw "Missing $requirements" }
if (-not (Test-Path $stressTest)) { throw "Missing $stressTest" }
if (-not (Test-Path $videoUtils)) { throw "Missing $videoUtils" }

$venvDir = Join-Path $ProjectRoot ".venv"
$venvPython = Join-Path $venvDir "Scripts\python.exe"

if (-not $SkipVenv) {
    if (-not (Test-Path $venvPython)) {
        Assert-Command "py"
        Write-Step "Creating Python virtual environment at $venvDir"
        & py -m venv $venvDir
    }
    Write-Step "Installing Python requirements"
    & $venvPython -m pip install -r $requirements
} elseif (-not (Test-Path $venvPython)) {
    throw "Virtualenv not found at $venvDir. Run without -SkipVenv first."
}

$framesDir = Resolve-ProjectPath $FramesPath
$filteredDir = Resolve-ProjectPath $FilteredPath
$outputVideoPath = Resolve-ProjectPath $OutputVideo
$videoPath = Resolve-ProjectPath $VideoFile

if (-not $SkipDownload -and -not (Test-Path $videoPath)) {
    Write-Step "Downloading sample video to $videoPath"
    & curl.exe -L -o $videoPath $VideoUrl
}

if (-not $SkipExtract) {
    $existingFrames = @()
    if (Test-Path $framesDir) {
        $existingFrames = @(Get-ChildItem -Path $framesDir -Filter "*.png" -File)
    }
    if ($existingFrames.Count -eq 0) {
        if (-not (Test-Path $videoPath)) {
            throw "Video file not found at $videoPath. Provide -VideoFile or remove -SkipDownload."
        }
        Write-Step "Extracting frames to $framesDir"
        & $venvPython $videoUtils -action extract $videoPath $framesDir
    } else {
        Write-Step "Using existing frames in $framesDir ($($existingFrames.Count) png files)"
    }
}

$frameCount = 0
if (Test-Path $framesDir) {
    $frameCount = @(Get-ChildItem -Path $framesDir -Filter "*.png" -File).Count
}
if ($frameCount -eq 0) {
    throw "No PNG frames found in $framesDir"
}

if ($CleanFiltered -and (Test-Path $filteredDir)) {
    Write-Step "Cleaning filtered output directory $filteredDir"
    Get-ChildItem -Path $filteredDir -Filter "*.png" -File | Remove-Item -Force
}
New-Item -ItemType Directory -Force -Path $filteredDir | Out-Null

Write-Step "Logging in to $ApiBaseUrl as $Username"
$basic = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("${Username}:${Password}"))
$login = Invoke-JsonRequest -Method "Post" -Uri "$ApiBaseUrl/login" -Headers @{ Authorization = "Basic $basic" }
$token = $login.token
if (-not $token) {
    throw "Login did not return a token."
}

try {
    $headers = @{ Authorization = "Bearer $token" }

    Write-Step "Creating workload with Loco no-body request"
    $workload = Invoke-JsonRequest -Method "Post" -Uri "$ApiBaseUrl/workloads" -Headers $headers
    $workloadID = $workload.workload_id
    if (-not $workloadID) {
        throw "Workload creation did not return workload_id."
    }
    Write-Step "Workload created: $workloadID"

    Write-Step "Pushing $frameCount frames"
    & $venvPython $stressTest -action push -workload-id $workloadID -token $token -frames-path $framesDir

    Write-Step "Waiting for workload completion"
    $deadline = (Get-Date).AddSeconds($PollTimeoutSeconds)
    do {
        Start-Sleep -Seconds 2
        $status = Invoke-JsonRequest -Method "Get" -Uri "$ApiBaseUrl/workloads/$workloadID" -Headers $headers
        $filteredCount = @($status.filtered_images).Count
        Write-Step "status=$($status.status) running_jobs=$($status.running_jobs) filtered=$filteredCount/$frameCount"
        if ($status.status -eq "completed" -and $filteredCount -ge $frameCount) {
            break
        }
    } while ((Get-Date) -lt $deadline)

    if ($status.status -ne "completed") {
        throw "Workload did not complete before timeout. Last status: $($status.status)"
    }

    Write-Step "Pulling filtered images to $filteredDir"
    & $venvPython $stressTest -action pull -workload-id $workloadID -image-type filtered -token $token -frames-path $filteredDir

    Write-Step "Joining filtered frames into $outputVideoPath"
    & $venvPython $videoUtils -action join $outputVideoPath $filteredDir

    Write-Step "Loco flow finished successfully."
    Write-Step "Workload: $workloadID"
    Write-Step "Output video: $outputVideoPath"
} finally {
    if ($token) {
        Write-Step "Logging out"
        try {
            Invoke-RestMethod -Method Delete -Uri "$ApiBaseUrl/logout" -Headers @{ Authorization = "Bearer $token" } | Out-Null
        } catch {
            Write-Step "Logout failed: $($_.Exception.Message)"
        }
    }
}
