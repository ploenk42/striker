<#
.SYNOPSIS
  Sets up or removes the demo fleet for the Striker documentation screenshots.

.DESCRIPTION
  demo.ps1 -Target <folder>   creates <folder>\repos\alpha and <folder>\repos\beta as git repos from the
                              templates in repos\, a demo STRIKER_HOME in <folder>\home (fleet.json with the
                              repo paths, briefs copied from striker\briefs) and copies the lead status files
                              from status\ into the status dir (default ~\.striker\status, where Striker reads
                              the status files of leads on this computer).
  demo.ps1 -Clean <folder>    removes <folder> and the status files the setup wrote (listed in
                              <folder>\demo.manifest).

  Nothing is overwritten: the target folder must not exist yet, and the setup stops if a status file it would
  write is already there.

.EXAMPLE
  .\demo.ps1 -Target C:\Temp\striker-demo
  .\demo.ps1 -Clean C:\Temp\striker-demo
#>
[CmdletBinding(DefaultParameterSetName = 'Up')]
param(
    [Parameter(Mandatory = $true, ParameterSetName = 'Up')]
    [string]$Target,

    [Parameter(Mandatory = $true, ParameterSetName = 'Clean')]
    [string]$Clean,

    [Parameter(ParameterSetName = 'Up')]
    [string]$StatusDir = (Join-Path $HOME '.striker\status')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$demoDir = $PSScriptRoot
$strikerDir = Join-Path (Split-Path (Split-Path $demoDir -Parent) -Parent) 'striker'
$utf8 = New-Object System.Text.UTF8Encoding $false

# The status files a demo writes or produces: the two leads, and the coordinator of alpha/add-greeting
# started while taking the shots.
$demoStatusFiles = @('alpha.json', 'beta.json', 'alpha--add-greeting.json')

function Invoke-DemoGit {
    param([string]$Repo, [string[]]$GitArgs)
    $all = @('-C', $Repo, '-c', 'user.name=Demo', '-c', 'user.email=demo@example.invalid',
             '-c', 'commit.gpgsign=false', '-c', 'core.autocrlf=false') + $GitArgs
    & git @all | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "git $($GitArgs -join ' ') failed in $Repo (exit $LASTEXITCODE)"
    }
}

function Write-DemoFile {
    param([string]$Path, [string]$Text)
    [System.IO.File]::WriteAllText($Path, $Text, $utf8)
}

function New-DemoRepo {
    param([string]$Name, [string]$Dest, [object[]]$Commits)
    Copy-Item -Path (Join-Path $demoDir "repos\$Name") -Destination $Dest -Recurse
    Invoke-DemoGit $Dest @('init', '--quiet', '--initial-branch=main')
    foreach ($c in $Commits) {
        Invoke-DemoGit $Dest (@('add', '--') + $c.Paths)
        Invoke-DemoGit $Dest @('commit', '--quiet', '-m', $c.Message)
    }
}

if ($PSCmdlet.ParameterSetName -eq 'Clean') {
    $root = [System.IO.Path]::GetFullPath($Clean)
    $manifest = Join-Path $root 'demo.manifest'
    if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) {
        throw "$root has no demo.manifest; not a demo folder, nothing removed."
    }
    foreach ($line in [System.IO.File]::ReadAllLines($manifest)) {
        if ($line -like 'status=*') {
            $file = $line.Substring(7)
            if (Test-Path -LiteralPath $file -PathType Leaf) {
                Remove-Item -LiteralPath $file -Force
                Write-Host "removed $file"
            }
        }
    }
    Remove-Item -LiteralPath $root -Recurse -Force
    Write-Host "removed $root"
    return
}

$root = [System.IO.Path]::GetFullPath($Target)
if (Test-Path -LiteralPath $root) {
    throw "$root already exists; choose a new folder or remove it with -Clean."
}
$StatusDir = [System.IO.Path]::GetFullPath($StatusDir)
$statusPaths = @()
foreach ($n in $demoStatusFiles) {
    $p = Join-Path $StatusDir $n
    if (Test-Path -LiteralPath $p) {
        throw "$p already exists; a demo may still be set up (remove it with -Clean) or a real repo uses that name."
    }
    $statusPaths += $p
}
if (-not (Test-Path -LiteralPath (Join-Path $strikerDir 'briefs') -PathType Container)) {
    throw "striker\briefs not found next to docs-site ($strikerDir)."
}

$repos = Join-Path $root 'repos'
$home_ = Join-Path $root 'home'
New-Item -ItemType Directory -Path $repos | Out-Null
New-Item -ItemType Directory -Path $home_ | Out-Null

# The manifest comes first, so -Clean works even if the setup stops halfway.
$lines = @('# striker docs demo fleet, made by demo.ps1') + ($statusPaths | ForEach-Object { "status=$_" })
Write-DemoFile (Join-Path $root 'demo.manifest') (($lines -join "`n") + "`n")

New-DemoRepo 'alpha' (Join-Path $repos 'alpha') @(
    @{ Paths = @('README.md', 'AGENTS.md', 'openspec/config.yaml'); Message = 'alpha: README and working rules' },
    @{ Paths = @('openspec/changes/add-greeting'); Message = 'openspec: add-greeting (proposal, design, tasks, plan)' }
)
New-DemoRepo 'beta' (Join-Path $repos 'beta') @(
    @{ Paths = @('README.md', 'openspec/config.yaml'); Message = 'beta: README' },
    @{ Paths = @('charts.txt', 'openspec/specs', 'openspec/changes/archive'); Message = 'openspec: archive chart-list' }
)

$fwd = { param($p) $p.Replace('\', '/') }
$fleet = [System.IO.File]::ReadAllText((Join-Path $demoDir 'home\fleet.json'))
$fleet = $fleet.Replace('{repos}', (& $fwd $repos)).Replace('{status}', (& $fwd $StatusDir))
Write-DemoFile (Join-Path $home_ 'fleet.json') $fleet
Copy-Item -Path (Join-Path $strikerDir 'briefs') -Destination (Join-Path $home_ 'briefs') -Recurse

if (-not (Test-Path -LiteralPath $StatusDir)) {
    New-Item -ItemType Directory -Path $StatusDir | Out-Null
}
foreach ($n in @('alpha.json', 'beta.json')) {
    Copy-Item -LiteralPath (Join-Path $demoDir "status\$n") -Destination (Join-Path $StatusDir $n)
}

Write-Host "Demo fleet ready in $root"
Write-Host "  repos:       $repos"
Write-Host "  STRIKER_HOME:  $home_"
Write-Host "  status dir:  $StatusDir"
Write-Host "Remove it with: .\demo.ps1 -Clean $root"
