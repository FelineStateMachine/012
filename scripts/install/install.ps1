# install.ps1 - installs a released 012 binary on Windows. Served at the
# docs site's root with the release archives under /releases/; see
# docs/getting-started/install.md.
#
#   irm https://f58b.n.zip/install.ps1 | iex
#
# It picks the archive for this architecture, checks its SHA256 against
# the release's SHA256SUMS, puts 012.exe in
# %LOCALAPPDATA%\Programs\012\bin and adds that folder to the user's
# PATH. It never asks for administrator rights.
#
#   $env:O12_VERSION      a release other than the latest, like v1.2.3
#   $env:O12_INSTALL_DIR  another folder for 012.exe
#   $env:O12_BASE_URL     the site serving /releases/ (https://f58b.n.zip)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Install-012 {
    $base = if ($env:O12_BASE_URL) { $env:O12_BASE_URL } else { 'https://f58b.n.zip' }
    $version = if ($env:O12_VERSION) { $env:O12_VERSION } else { 'latest' }
    if ($version -match '^[0-9]') { $version = "v$version" }
    $dir = if ($env:O12_INSTALL_DIR) { $env:O12_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\012\bin' }

    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { throw "012 install: no release for $arch; build from source: go install github.com/FelineStateMachine/012/cmd/012@latest" }
    }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("012-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $url = "$base/releases/$version"
        $sums = Join-Path $tmp 'SHA256SUMS'
        Invoke-WebRequest -UseBasicParsing -Uri "$url/SHA256SUMS" -OutFile $sums

        # A line of SHA256SUMS: the digest, two spaces, 012_1.2.3_windows_amd64.zip.
        $line = Get-Content $sums | Where-Object { $_ -match "^([0-9a-f]{64})  (012_\S+_windows_$arch\.zip)$" } | Select-Object -First 1
        if (-not $line) { throw "012 install: release $version has no archive for windows/$arch" }
        $null = $line -match "^([0-9a-f]{64})  (012_\S+_windows_$arch\.zip)$"
        $want = $Matches[1]
        $file = $Matches[2]
        $name = $file -replace '\.zip$', ''
        $got_version = 'v' + ($name -replace '^012_', '' -replace "_windows_$arch$", '')

        Write-Host "012 install: downloading $url/$file"
        $zip = Join-Path $tmp $file
        Invoke-WebRequest -UseBasicParsing -Uri "$url/$file" -OutFile $zip
        $got = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLowerInvariant()
        if ($got -ne $want) { throw "012 install: $file has SHA256 $got, SHA256SUMS says $want; nothing installed" }
        Write-Host "012 install: SHA256 checked against SHA256SUMS: $want"

        Expand-Archive -Path $zip -DestinationPath $tmp -Force
        $exe = Join-Path $tmp "$name\012.exe"
        if (-not (Test-Path $exe)) { throw "012 install: $file holds no 012.exe" }
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        Copy-Item -Force $exe (Join-Path $dir '012.exe')
        Write-Host "012 install: installed 012 $got_version (windows/$arch) as $(Join-Path $dir '012.exe')"

        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $parts = @()
        if ($userPath) { $parts = $userPath -split ';' | Where-Object { $_ } }
        if ($parts -notcontains $dir) {
            [Environment]::SetEnvironmentVariable('Path', (($parts + $dir) -join ';'), 'User')
            $env:Path = "$env:Path;$dir"
            Write-Host "012 install: added $dir to your user PATH; new terminals find 012"
        } else {
            Write-Host "012 install: run 012 to start"
        }
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

Install-012
