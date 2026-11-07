param(
    [string]$Distro = 'Ubuntu-22.04',
    [string[]]$Packages = @('./...'),
    [ValidateRange(1,100)][int]$Repeat = 1,
    [string]$Run = '.'
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$tags = 'sing xray hysteria2 with_quic with_grpc with_utls with_wireguard with_acme with_gvisor'
$oldGOOS, $oldGOARCH, $oldCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
Push-Location $repo
try {
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $output = Join-Path $repo 'dist/tests'
    New-Item -ItemType Directory -Force $output | Out-Null
    $testPackages = go list -tags $tags -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' @Packages
    if ($LASTEXITCODE -ne 0) { throw 'go list failed' }
    foreach ($package in $testPackages) {
        if (-not $package.Trim()) { continue }
        $relative = $package -replace '^github.com/InazumaV/V2bX/', ''
        $binary = Join-Path $output (($relative -replace '/', '_') + '.test')
        go test -c -tags $tags -o $binary $package
        if ($LASTEXITCODE -ne 0) { throw "Test compilation failed: $package" }
        $linuxBinary = (wsl.exe -d $Distro --exec wslpath -u $binary.Replace('\','/') | Select-Object -Last 1).Trim()
        $linuxCwd = (wsl.exe -d $Distro --exec wslpath -u (Join-Path $repo $relative).Replace('\','/') | Select-Object -Last 1).Trim()
        wsl.exe -d $Distro --cd $linuxCwd --exec $linuxBinary '-test.v' '-test.timeout=90s' "-test.count=$Repeat" "-test.run=$Run"
        if ($LASTEXITCODE -ne 0) { throw "Tests failed: $package" }
    }
    $linuxRepo = (wsl.exe -d $Distro --exec wslpath -u $repo.Replace('\','/') | Select-Object -Last 1).Trim()
    wsl.exe -d $Distro --cd $linuxRepo -- bash script/tests/wizard.sh
    if ($LASTEXITCODE -ne 0) { throw 'Wizard regression tests failed' }
    wsl.exe -d $Distro --cd $linuxRepo --exec bash -c 'export PATH="$PWD/dist/test-tools/runtime/usr/bin:$PATH"; bash script/tests/installer.sh'
    if ($LASTEXITCODE -ne 0) { throw 'Installer regression tests failed' }
} finally {
    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $oldGOOS, $oldGOARCH, $oldCGO
    Pop-Location
}
