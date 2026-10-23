# Install from_code toolchains for HackMe Windows desktop (portable under APP_DIR\toolchains).
param(
  [string]$AppDir = $PSScriptRoot,
  [switch]$CheckOnly
)

$ErrorActionPreference = "Stop"
$ZigVersion = if ($env:ZIG_VERSION) { $env:ZIG_VERSION } else { "0.14.0" }
$TinyGoVersion = if ($env:TINYGO_VERSION) { $env:TINYGO_VERSION } else { "0.35.0" }
$Prefix = Join-Path $AppDir "toolchains"
$Bin = Join-Path $Prefix "bin"
New-Item -ItemType Directory -Force -Path $Bin | Out-Null

# Pinned SHA256 for release assets (update when bumping versions).
$ToolchainSha256 = @{
  "zig-windows-x86_64-0.14.0.zip"           = "f53e5f9011ba20bbc3e0e6d0a9441b31eb227a97bac0e7d24172f1b8b27b4371"
  "node-v22.15.1-win-x64.zip"               = "7c7997d6ef2efd9525a54b033fc0be6f22549a5905f5da6573a41e61a436a1b5"
  "tinygo0.35.0.windows-amd64.zip"          = "81c10cdd2ebe8b5e8a74d8011481c0df14507b7d6f1e571fea4fd65b623a9417"
  "wabt-1.0.42-windows-x64.tar.gz"          = "62f1eb2b51aa57cf0f0b8ba333bf2c72a049930aa9c5de01dcad271c4fe48c88"
}

function Log($m) { Write-Host "[toolchains] $m" }
function Warn($m) { Write-Warning "[toolchains] $m" }
function Have($name) { return [bool](Get-Command $name -ErrorAction SilentlyContinue) }

function Assert-FileSha256([string]$Path, [string]$ExpectedHex) {
  if (-not $ExpectedHex) {
    throw "no pinned SHA256 for $(Split-Path -Leaf $Path)"
  }
  $actual = (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToLowerInvariant()
  $expect = $ExpectedHex.ToLowerInvariant()
  if ($actual -ne $expect) {
    throw "SHA256 mismatch for $(Split-Path -Leaf $Path): got $actual expected $expect"
  }
  Log "SHA256 OK $(Split-Path -Leaf $Path)"
}

function Add-PathPrefix([string]$dir) {
  if (Test-Path $dir) {
    $env:PATH = "$dir;$env:PATH"
  }
}

function Install-ZigPortable {
  $zigRoot = Join-Path $Prefix "zig-windows-x86_64-$ZigVersion"
  $zigExe = Join-Path $zigRoot "zig.exe"
  if (Test-Path $zigExe) { return }
  $zip = "zig-windows-x86_64-$ZigVersion.zip"
  $url = "https://ziglang.org/download/$ZigVersion/$zip"
  $tmp = Join-Path $env:TEMP "hackme-zig-$ZigVersion"
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  $out = Join-Path $tmp $zip
  Log "downloading Zig $ZigVersion"
  Invoke-WebRequest -Uri $url -OutFile $out
  Assert-FileSha256 $out $ToolchainSha256[$zip]
  Expand-Archive -Path $out -DestinationPath $tmp -Force
  if (Test-Path $zigRoot) { Remove-Item -Recurse -Force $zigRoot }
  Move-Item (Join-Path $tmp "zig-windows-x86_64-$ZigVersion") $zigRoot
  Remove-Item -Recurse -Force $tmp
}

function Install-TinyGoPortable {
  $tinyExe = Join-Path $Prefix "tinygo\bin/tinygo.exe"
  if (Test-Path $tinyExe) { return }
  $zip = "tinygo${TinyGoVersion}.windows-amd64.zip"
  $url = "https://github.com/tinygo-org/tinygo/releases/download/v${TinyGoVersion}/$zip"
  $tmp = Join-Path $env:TEMP "hackme-tinygo"
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  $out = Join-Path $tmp $zip
  Log "downloading TinyGo $TinyGoVersion"
  Invoke-WebRequest -Uri $url -OutFile $out
  Assert-FileSha256 $out $ToolchainSha256[$zip]
  Expand-Archive -Path $out -DestinationPath $tmp -Force
  $dest = Join-Path $Prefix "tinygo"
  if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
  Move-Item (Join-Path $tmp "tinygo") $dest
  Remove-Item -Recurse -Force $tmp
}

function Install-WabtPortable {
  $wat = Join-Path $Prefix "wabt/wat2wasm.exe"
  if (Test-Path $wat) { return }
  # 1.0.36-windows.zip 404s; pin a working windows-x64 tar.gz release asset.
  $ver = if ($env:WABT_VERSION) { $env:WABT_VERSION } else { "1.0.42" }
  $archive = "wabt-$ver-windows-x64.tar.gz"
  $url = "https://github.com/WebAssembly/wabt/releases/download/$ver/$archive"
  $tmp = Join-Path $env:TEMP "hackme-wabt"
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  $out = Join-Path $tmp $archive
  Log "downloading WABT $ver"
  try {
    Invoke-WebRequest -Uri $url -OutFile $out
    $expect = $ToolchainSha256[$archive]
    if (-not $expect) { throw "no pinned SHA256 for $archive (bump ToolchainSha256)" }
    Assert-FileSha256 $out $expect
    # Windows 10+ ships tar.exe; Expand-Archive cannot read .tar.gz.
    & tar -xzf $out -C $tmp
    if ($LASTEXITCODE -ne 0) { throw "tar extract failed for $archive" }
    $found = Get-ChildItem -Path $tmp -Recurse -Filter wat2wasm.exe | Select-Object -First 1
    if ($found) {
      $wdir = Join-Path $Prefix "wabt"
      New-Item -ItemType Directory -Force -Path $wdir | Out-Null
      Copy-Item $found.FullName $wat -Force
    }
  } catch {
    Warn "wabt download failed -- install LLVM/wabt manually or: winget install WebAssembly.wabt ($_)"
  }
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

function Install-RustupUser {
  $cargo = Join-Path $Prefix ".cargo\bin\rustc.exe"
  if (Test-Path $cargo) { return }
  if (-not (Get-Command rustup -ErrorAction SilentlyContinue)) {
    Warn "rustup missing -- install from https://rustup.rs/ or: winget install Rustlang.Rustup"
    return
  }
  $env:CARGO_HOME = Join-Path $Prefix ".cargo"
  $env:RUSTUP_HOME = Join-Path $Prefix ".rustup"
  Log "rustup toolchain minimal"
  rustup toolchain install stable -y --profile minimal 2>$null
}

function Install-NodePortable {
  $nodeExe = Join-Path $Prefix "nodejs\node.exe"
  if (Test-Path $nodeExe) { return }
  if (Get-Command npm -ErrorAction SilentlyContinue) { return }
  $nodeVer = if ($env:NODE_VERSION) { $env:NODE_VERSION } else { "22.15.1" }
  $zip = "node-v${nodeVer}-win-x64.zip"
  $url = "https://nodejs.org/dist/v${nodeVer}/$zip"
  $tmp = Join-Path $env:TEMP "hackme-node"
  New-Item -ItemType Directory -Force -Path $tmp | Out-Null
  $out = Join-Path $tmp $zip
  Log "downloading Node.js $nodeVer (for asc)"
  try {
    Invoke-WebRequest -Uri $url -OutFile $out
    Assert-FileSha256 $out $ToolchainSha256[$zip]
    Expand-Archive -Path $out -DestinationPath $tmp -Force
    $dest = Join-Path $Prefix "nodejs"
    if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
    Move-Item (Join-Path $tmp "node-v${nodeVer}-win-x64") $dest
  } catch {
    Warn "portable Node download failed -- winget install OpenJS.NodeJS.LTS ($_)"
  }
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

function Install-AscNpm {
  $asc = Join-Path $Prefix ".npm-global\asc.cmd"
  if (Test-Path $asc) { return }
  Install-NodePortable
  $npm = Get-Command npm -ErrorAction SilentlyContinue
  if (-not $npm) {
    $portableNpm = Join-Path $Prefix "nodejs\npm.cmd"
    if (Test-Path $portableNpm) { $npm = Get-Command $portableNpm }
  }
  if (-not $npm) {
    Warn "npm missing -- install Node.js LTS (nodejs.org or winget install OpenJS.NodeJS.LTS)"
    return
  }
  $env:NPM_CONFIG_PREFIX = Join-Path $Prefix ".npm-global"
  New-Item -ItemType Directory -Force -Path $env:NPM_CONFIG_PREFIX | Out-Null
  Log "npm install -g assemblyscript"
  & $npm.Source install -g assemblyscript@latest --prefix $env:NPM_CONFIG_PREFIX
}

function Check-All {
  $missing = 0
  foreach ($t in @("wat2wasm", "rustc", "clang", "tinygo", "zig", "asc")) {
    if (Have $t) { Log "OK   $t" }
    else { Warn "MISS $t"; $missing++ }
  }
  return $missing -eq 0
}

# Optional system installs via winget (best-effort, no admin required for some)
function Try-Winget([string]$id) {
  if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { return }
  winget install --id $id -e --accept-source-agreements --accept-package-agreements 2>$null
}

if (-not $CheckOnly) {
  Try-Winget "LLVM.LLVM"
  Try-Winget "Rustlang.Rustup"
  Try-Winget "OpenJS.NodeJS.LTS"
  Install-NodePortable
  Install-ZigPortable
  Install-TinyGoPortable
  Install-WabtPortable
  Install-RustupUser
  Install-AscNpm
}

$zigRoot = Join-Path $Prefix "zig-windows-x86_64-$ZigVersion"
Add-PathPrefix (Join-Path $Prefix "nodejs")
Add-PathPrefix (Join-Path $zigRoot "")
Add-PathPrefix (Join-Path $Prefix "tinygo\bin")
Add-PathPrefix (Join-Path $Prefix "wabt")
Add-PathPrefix (Join-Path $Prefix ".cargo\bin")
Add-PathPrefix (Join-Path $Prefix ".npm-global")
Add-PathPrefix $Bin

$envFile = Join-Path $Prefix ".env.toolchains.windows"
$nodePath = Join-Path $Prefix "nodejs"
@"
# Generated by install_from_code_toolchains.ps1
PATH=$nodePath;$zigRoot;$($Prefix)\tinygo\bin;$($Prefix)\wabt;$($Prefix)\.cargo\bin;$($Prefix)\.npm-global;$Bin
RUSTUP_HOME=$($Prefix)\.rustup
CARGO_HOME=$($Prefix)\.cargo
NPM_CONFIG_PREFIX=$($Prefix)\.npm-global
"@ | Set-Content -Encoding utf8 $envFile
Copy-Item $envFile (Join-Path $AppDir ".hackme-toolchains.env") -Force

Log "toolchain root: $Prefix"
Log "env file: $envFile"
if (-not (Check-All)) { Warn "some toolchains missing -- re-run or install via winget" }
Log "done"
