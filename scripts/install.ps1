$ErrorActionPreference = "Stop"

$Repo = "struckchure/axel"
$LatestReleaseApi = "https://api.github.com/repos/$Repo/releases/latest"

# ── Version resolution ─────────────────────────────────────────────────────────
$Version = $args[0]
if (-not $Version) {
    # Prefer the releases/latest redirect (not rate-limited): follow it and read
    # the tag from the final URL. Fall back to the GitHub API if that fails.
    try {
        $req = [System.Net.WebRequest]::Create("https://github.com/$Repo/releases/latest")
        $req.Method = "HEAD"
        $req.AllowAutoRedirect = $false
        $req.UserAgent = "axel-installer"
        $resp = $req.GetResponse()
        $location = $resp.Headers["Location"]
        $resp.Close()
        if ($location -match "/tag/(.+)$") { $Version = $Matches[1] }
    } catch { }

    if (-not $Version) {
        try {
            $ReleaseInfo = Invoke-RestMethod -Uri $LatestReleaseApi -Headers @{ Accept = "application/vnd.github+json"; "User-Agent" = "axel-installer" }
            $Version = $ReleaseInfo.tag_name
        } catch { }
    }

    if (-not $Version) {
        Write-Error "Failed to resolve latest release version"
        exit 1
    }
}

Write-Host "Installing axel $Version..."

# ── Platform detection ─────────────────────────────────────────────────────────
$Arch = [System.Environment]::GetEnvironmentVariable("PROCESSOR_ARCHITECTURE")

switch ($Arch) {
    "AMD64" { $File = "axel_Windows_x86_64.zip" }
    "ARM64" { $File = "axel_Windows_arm64.zip"  }
    default {
        Write-Error "Unsupported architecture: $Arch"
        exit 1
    }
}

# ── Download & extract ─────────────────────────────────────────────────────────
$DestDir = "$env:USERPROFILE\.axel\bin"
if (-not (Test-Path $DestDir)) {
    New-Item -Path $DestDir -ItemType Directory | Out-Null
}

$TmpDir = [System.IO.Path]::GetTempPath() + [System.Guid]::NewGuid().ToString()
New-Item -Path $TmpDir -ItemType Directory | Out-Null

$DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$File"
$TmpFile = Join-Path $TmpDir $File

Write-Host "Downloading $File..."
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TmpFile
} catch {
    Write-Error "Failed to download ${DownloadUrl}: $($_.Exception.Message)"
    Remove-Item -Path $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host "Extracting to $DestDir..."
try {
    Expand-Archive -Path $TmpFile -DestinationPath $DestDir -Force
} catch {
    Write-Error "Failed to extract ${File}: $($_.Exception.Message)"
    Remove-Item -Path $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    exit 1
}

Remove-Item -Path $TmpDir -Recurse -Force

# ── PATH update ────────────────────────────────────────────────────────────────
$CurrentPath = [System.Environment]::GetEnvironmentVariable("PATH", [System.EnvironmentVariableTarget]::User)

if ($CurrentPath -notlike "*$DestDir*") {
    [System.Environment]::SetEnvironmentVariable(
        "PATH",
        "$CurrentPath;$DestDir",
        [System.EnvironmentVariableTarget]::User
    )
    Write-Host "Updated user PATH to include $DestDir"
} else {
    Write-Host "$DestDir is already in PATH"
}

# Also update PATH for the current session so the user doesn't need to restart now.
$env:PATH = "$env:PATH;$DestDir"

# ── Done ───────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "axel $Version installed to $DestDir\axel.exe"
Write-Host ""
Write-Host "Verify with:"
Write-Host "  axel version"
