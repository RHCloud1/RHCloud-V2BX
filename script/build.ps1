param(
    [string]$Version = 'local-simulation.20261108',
    [ValidateSet('amd64','arm64','s390x')][string[]]$Architectures = @('amd64','arm64')
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$tags = 'sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor'
$assetNames = @{ amd64 = '64'; arm64 = 'arm64-v8a'; s390x = 's390x' }
$oldGOOS, $oldGOARCH, $oldCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
Push-Location $repo
try {
    $env:GOOS = 'linux'
    $env:CGO_ENABLED = '0'
    $releaseDir = Join-Path $repo "dist/$Version"
    New-Item -ItemType Directory -Force $releaseDir | Out-Null
    foreach ($architecture in $Architectures) {
        $env:GOARCH = $architecture
        $stage = Join-Path $releaseDir ("stage-$architecture-" + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory $stage | Out-Null
        go build -tags $tags -trimpath -o (Join-Path $stage 'V2bX') -ldflags "-X github.com/InazumaV/V2bX/cmd.version=$Version -s -w -buildid=" .
        if ($LASTEXITCODE -ne 0) { throw "Build failed for $architecture" }
        Copy-Item README.md, LICENSE $stage
        Copy-Item example/*.json, example/*.dat $stage
        Copy-Item -Recurse docs (Join-Path $stage 'docs')
        Copy-Item script/*.sh, script/*.service $stage
        $archive = Join-Path $releaseDir "V2bX-linux-$($assetNames[$architecture]).zip"
        # Windows PowerShell 5.1's Compress-Archive writes "docs\file" entry names,
        # which install.sh rejects; write entries with "/" explicitly.
        Add-Type -AssemblyName System.IO.Compression, System.IO.Compression.FileSystem
        if (Test-Path $archive) { Remove-Item $archive }
        $zip = [IO.Compression.ZipFile]::Open($archive, [IO.Compression.ZipArchiveMode]::Create)
        try {
            $root = (Resolve-Path $stage).Path.TrimEnd('\') + '\'
            foreach ($item in Get-ChildItem -Recurse -File $stage) {
                $name = $item.FullName.Substring($root.Length).Replace('\', '/')
                [void][IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $item.FullName, $name, [IO.Compression.CompressionLevel]::Optimal)
            }
        } finally {
            $zip.Dispose()
        }
        $hash = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
        [IO.File]::WriteAllText("$archive.sha256", "$hash  $([IO.Path]::GetFileName($archive))`n", [Text.UTF8Encoding]::new($false))
        Write-Host "Built $archive"
    }
} finally {
    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $oldGOOS, $oldGOARCH, $oldCGO
    Pop-Location
}
