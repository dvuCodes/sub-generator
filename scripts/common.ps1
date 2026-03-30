function Get-NormalizedRepoRoot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ScriptRoot
    )

    $repoRoot = [System.IO.Path]::GetFullPath((Join-Path $ScriptRoot ".."))
    if ($repoRoot.StartsWith("\\?\")) {
        return $repoRoot.Substring(4)
    }

    return $repoRoot
}

function Invoke-FromRepoRoot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ScriptRoot,

        [Parameter(Mandatory = $true)]
        [scriptblock]$Action
    )

    $repoRoot = Get-NormalizedRepoRoot -ScriptRoot $ScriptRoot
    Push-Location -LiteralPath $repoRoot
    try {
        & $Action
        exit $LASTEXITCODE
    }
    finally {
        Pop-Location
    }
}
