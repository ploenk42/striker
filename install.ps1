# Windows: junction each mod into Herdr's mods/, copy the LCARS layout, build Striker and start it at logon.
$ErrorActionPreference = 'Stop'
$repo = $PSScriptRoot
$herdr = if ($env:HERDR_APP_HOME) { $env:HERDR_APP_HOME } else { Join-Path $env:USERPROFILE '.config\herdr-app' }

foreach ($mod in 'lcars', 'blueprint') {
    $target = Join-Path $herdr "mods\$mod"
    $source = Join-Path $repo $mod
    $item = Get-Item $target -Force -ErrorAction SilentlyContinue
    if ($item -and -not $item.LinkType) {
        Write-Warning "skip ${mod}: $target exists and is not a link"
        continue
    }
    if ($item) { $item.Delete() }
    New-Item -ItemType Junction -Path $target -Target $source | Out-Null
    Write-Host "linked $target -> $source"
}
Copy-Item (Join-Path $repo 'layouts\lcars.json') (Join-Path $herdr 'layouts\lcars.json') -Force
Write-Host 'copied layouts\lcars.json'

# a shell opened before Go was installed lacks it in PATH: try Go's default location
$goDefault = Join-Path $env:ProgramFiles 'Go\bin'
if (-not (Get-Command go -ErrorAction SilentlyContinue) -and (Test-Path (Join-Path $goDefault 'go.exe'))) {
    $env:Path = "$env:Path;$goDefault"
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'go not found in PATH; install it from https://go.dev/dl/ and rerun'
}
$exe = Join-Path $repo 'striker\bin\striker.exe'
# a running striker.exe cannot be overwritten: stop it before building
Stop-ScheduledTask -TaskName 'Striker' -ErrorAction SilentlyContinue
Stop-Process -Name striker -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force (Split-Path $exe) | Out-Null
Push-Location (Join-Path $repo 'striker')
try { go build -o $exe .\cmd\striker; if ($LASTEXITCODE) { throw 'go build failed' } } finally { Pop-Location }
Write-Host "built $exe"

# fleet.json is the person's own and untracked: start from the demo fleet when there is none
$fleet = Join-Path $repo 'striker\fleet.json'
if (-not (Test-Path $fleet)) {
    Copy-Item (Join-Path $repo 'striker\fleet.example.json') $fleet
    Write-Host "copied striker\fleet.example.json to $fleet; enter your repos there"
}

# striker on the user PATH, and STRIKER_HOME so the task and the CLI find fleet.json and briefs
$bin = Split-Path $exe
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | Where-Object { $_ })
if (-not ($entries -contains $bin)) {
    $entries += $bin
    [Environment]::SetEnvironmentVariable('Path', ($entries -join ';'), 'User')
    Write-Host "added $bin to the user PATH (new shells pick it up)"
}
[Environment]::SetEnvironmentVariable('STRIKER_HOME', (Join-Path $repo 'striker'), 'User')
Write-Host "set user environment variable STRIKER_HOME"

# Claude's status line stores model and tokens per terminal for Striker's crew (only if none is set yet)
& $exe install-statusline
if ($LASTEXITCODE) { Write-Warning 'striker install-statusline failed: the crew shows no model and tokens for local agents' }

# conhost --headless: a console without a window (closing a visible one would kill striker serve); hc children inherit it
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\conhost.exe" -Argument "--headless `"$exe`" serve" `
    -WorkingDirectory (Join-Path $repo 'striker')
$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$settings = New-ScheduledTaskSettingsSet -Hidden -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName 'Striker' -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
Stop-ScheduledTask -TaskName 'Striker' -ErrorAction SilentlyContinue
Start-ScheduledTask -TaskName 'Striker'
Write-Host 'registered and started scheduled task Striker'
