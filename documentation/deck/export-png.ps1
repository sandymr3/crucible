# Renders every slide to PNG for visual QA.
#
# The pptx skill's documented path is LibreOffice -> PDF -> pdftoppm. Neither
# exists on this machine, but PowerPoint COM 16.0 does — and it is strictly
# better for this job: it is the exact renderer the judges will use, so font
# metrics are real and apparent text fit can be trusted.

$ErrorActionPreference = "Stop"

$deck = Join-Path $PSScriptRoot "Crucible-InnovaHack.pptx"
$out  = Join-Path $PSScriptRoot "render"

if (-not (Test-Path $deck)) { throw "deck not found: $deck" }

# Clear stale renders so a deleted slide can't linger and be reviewed as current.
if (Test-Path $out) { Remove-Item $out -Recurse -Force }
New-Item -ItemType Directory -Path $out | Out-Null

# COM attaches to an already-running PowerPoint rather than starting a fresh
# one. If the user has PowerPoint open, Quit() here would try to close THEIR
# session — so only Quit when this script started the instance.
$preExisting = [bool](Get-Process POWERPNT -ErrorAction SilentlyContinue)

$ppt = New-Object -ComObject PowerPoint.Application
try {
    # msoTrue = -1. PowerPoint refuses to open a presentation fully hidden in
    # some builds, so open read-only with the window untitled instead.
    $pres = $ppt.Presentations.Open($deck, $true, $false, $false)
    try {
        $pres.Export($out, "PNG", 1920, 1080)
        Write-Output "exported $($pres.Slides.Count) slides -> $out"
    } finally {
        $pres.Close()
    }
} finally {
    if (-not $preExisting) { $ppt.Quit() }
    [System.Runtime.InteropServices.Marshal]::ReleaseComObject($ppt) | Out-Null
    [GC]::Collect()
    [GC]::WaitForPendingFinalizers()
}

# PowerPoint releases the file handle a moment after Quit() returns. Without
# this wait the next `node build-deck.js` fails with EBUSY on a file nothing
# appears to own. Skip when attached to the user's own instance.
if (-not $preExisting) {
    $deadline = (Get-Date).AddSeconds(20)
    while ((Get-Process POWERPNT -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
    }
}

Get-ChildItem $out -Filter *.PNG | Sort-Object Name | ForEach-Object {
    Write-Output ("  {0}  {1:N0} bytes" -f $_.Name, $_.Length)
}
