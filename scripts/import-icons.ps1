[CmdletBinding()]
param (
    [string]$SourceDir = "E:\New folder\ICONS (2)\ICONS",
    [string]$DestDir = ""
)

$ErrorActionPreference = "Stop"

if (-not $DestDir) {
    $DestDir = Join-Path $PSScriptRoot "..\frontend\public\icons"
}

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "  Veracity Maps -- Icon Asset Importer and Catalog" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

if (-not (Test-Path $SourceDir)) {
    Write-Error "Source directory does not exist: $SourceDir`nPlease check the path or specify -SourceDir."
    exit 1
}

$destFullPath = [System.IO.Path]::GetFullPath($DestDir)
if (-not (Test-Path $destFullPath)) {
    New-Item -ItemType Directory -Path $destFullPath -Force | Out-Null
}
$destResolved = (Resolve-Path -Path $destFullPath).Path

Write-Host "Source:      $SourceDir"
Write-Host "Destination: $destResolved`n"

# Load System.Drawing for dimension reading if available
Add-Type -AssemblyName System.Drawing -ErrorAction SilentlyContinue

$supportedExts = @(".png", ".svg", ".webp", ".jpg", ".jpeg", ".ico")
$allFiles = Get-ChildItem -Path $SourceDir -Recurse -File

$imported = @()
$skipped = @()
$duplicates = @()
$seenHashes = @{}
$seenIds = @{}

$defaultManifestIcons = @(
    "cinema", "fingerprint", "key", "locked-with-key", "locked",
    "person-police-officer", "security-lock", "shield", "unlocked", "video-call"
)

foreach ($file in $allFiles) {
    $ext = $file.Extension.ToLower()

    if ($supportedExts -notcontains $ext) {
        $skipped += [PSCustomObject]@{
            Name   = $file.Name
            Path   = $file.FullName
            Reason = "Unsupported extension '$ext'"
        }
        continue
    }

    # Hash check for exact duplicate content
    $hash = (Get-FileHash -Path $file.FullName -Algorithm SHA256).Hash
    if ($seenHashes.ContainsKey($hash)) {
        $duplicates += [PSCustomObject]@{
            Name         = $file.Name
            Path         = $file.FullName
            OriginalPath = $seenHashes[$hash]
        }
        continue
    }
    $seenHashes[$hash] = $file.FullName

    # Determine Category from immediate or parent folder
    $parentFolder = $file.Directory.Name
    $category = switch -Regex ($parentFolder) {
        "Exp Icon"          { "Exploration and Operations" }
        "Holly Icons"       { "Civil and Public Service" }
        "Icons Format"      { "Standard Symbols" }
        "Map Icon"          { "Map and Navigation" }
        "New Smart ICONS"   { "Smart Infrastructure" }
        "noidapoc_icons"    { "Municipal and Security" }
        "PNG Format"        { "General Symbols" }
        "water icons"       { "Water Utility" }
        "Water Tank Icons"  { "Water Tanks and Storage" }
        default             {
            if ($parentFolder -eq "ICONS") { "General" }
            else { $parentFolder }
        }
    }

    # Clean Slug ID
    $baseName = [System.IO.Path]::GetFileNameWithoutExtension($file.Name)
    $cleanId = $baseName.ToLower() -replace '[^a-z0-9]+', '-'
    $cleanId = $cleanId.Trim('-')
    if (-not $cleanId) { $cleanId = "icon" }

    # Deduplicate ID
    $id = $cleanId
    $counter = 1
    while ($seenIds.ContainsKey($id)) {
        $counter++
        $id = "$cleanId-$counter"
    }
    $seenIds[$id] = $true

    $destFileName = "$id$ext"
    $targetPath = Join-Path $destResolved $destFileName

    # Copy file (preserve original transparency, do not convert)
    Copy-Item -Path $file.FullName -Destination $targetPath -Force

    # Read dimensions if possible
    $width = 0
    $height = 0
    try {
        if ($ext -ne ".svg") {
            $img = [System.Drawing.Image]::FromFile($targetPath)
            $width = $img.Width
            $height = $img.Height
            $img.Dispose()
        }
    } catch {
        $width = 32
        $height = 32
    }

    # Create Display Name
    $displayName = ($baseName -replace '[-_]+', ' ').Trim()
    $textInfo = (Get-Culture).TextInfo
    $displayName = $textInfo.ToTitleCase($displayName.ToLower())

    $imported += [PSCustomObject]@{
        id               = $id
        name             = $displayName
        category         = $category
        filename         = $destFileName
        path             = "/icons/$destFileName"
        file_type        = $ext.TrimStart('.')
        has_transparency = ($ext -in @(".png", ".svg", ".webp"))
        width            = $width
        height           = $height
        file_size        = $file.Length
    }
}

# Also ensure existing seed icons in public/icons are cataloged if present
foreach ($seedId in $defaultManifestIcons) {
    if (-not $seenIds.ContainsKey($seedId)) {
        $seedPath = Join-Path $destResolved "$seedId.png"
        if (Test-Path $seedPath) {
            $seedFile = Get-Item $seedPath
            $width = 32
            $height = 32
            try {
                $img = [System.Drawing.Image]::FromFile($seedPath)
                $width = $img.Width
                $height = $img.Height
                $img.Dispose()
            } catch {}

            $displayName = ($seedId -replace '[-_]+', ' ').Trim()
            $textInfo = (Get-Culture).TextInfo
            $displayName = $textInfo.ToTitleCase($displayName.ToLower())

            $imported += [PSCustomObject]@{
                id               = $seedId
                name             = $displayName
                category         = "Security and Access"
                filename         = "$seedId.png"
                path             = "/icons/$seedId.png"
                file_type        = "png"
                has_transparency = $true
                width            = $width
                height           = $height
                file_size        = $seedFile.Length
            }
            $seenIds[$seedId] = $true
        }
    }
}

$categories = $imported | ForEach-Object { $_.category } | Select-Object -Unique | Sort-Object

$manifest = [ordered]@{
    version      = "1.0"
    generated_at = (Get-Date).ToString("o")
    source_dir   = $SourceDir
    total_icons  = $imported.Count
    categories   = $categories
    icons        = $imported
}

$manifestPath = Join-Path $destResolved "manifest.json"
$manifestJson = $manifest | ConvertTo-Json -Depth 5
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($manifestPath, $manifestJson, $utf8NoBom)

Write-Host "----------------------------------------------------------" -ForegroundColor Green
Write-Host "Import Complete!" -ForegroundColor Green
Write-Host "  Imported Icons:     $($imported.Count)" -ForegroundColor Green
Write-Host "  Categories:         $($categories.Count)" -ForegroundColor Green
Write-Host "  Duplicates Skipped: $($duplicates.Count)" -ForegroundColor Yellow
Write-Host "  Unsupported Files:  $($skipped.Count)" -ForegroundColor Yellow
Write-Host "  Manifest Created:   $manifestPath" -ForegroundColor Green
Write-Host "----------------------------------------------------------" -ForegroundColor Green
