param(
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$OutDir,
    [Parameter(Mandatory = $true)][string]$Workspace
)

$semverPattern = '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
if ($Version -notmatch $semverPattern) {
    throw "Invalid semver Version '$Version'. Expected strict semver like 0.4.0-rc.1."
}
if ([string]::IsNullOrWhiteSpace($OutDir)) {
    throw "OutDir must not be empty."
}
if ($OutDir -match '[;"`$\r\n\t]') {
    throw "OutDir contains disallowed characters: '$OutDir'"
}

$workspaceFull = [System.IO.Path]::GetFullPath($Workspace).TrimEnd('\', '/')
$candidate = if ([System.IO.Path]::IsPathRooted($OutDir)) {
    [System.IO.Path]::GetFullPath($OutDir)
} else {
    [System.IO.Path]::GetFullPath((Join-Path $workspaceFull $OutDir))
}
$prefix = $workspaceFull + [System.IO.Path]::DirectorySeparatorChar
if (-not $candidate.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "OutDir '$OutDir' resolves to '$candidate', which escapes workspace '$workspaceFull'."
}
$relOut = $candidate.Substring($prefix.Length) -replace '\\', '/'
if ($relOut -eq "" -or $relOut -eq ".git" -or $relOut.StartsWith(".git/")) {
    throw "OutDir '$OutDir' points to restricted workspace path '$relOut'."
}

return [pscustomobject]@{
    Version   = $Version
    OutPath   = $candidate
    RelOutPosix = $relOut
    Workspace = $workspaceFull
}
