$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

Invoke-FromRepoRoot -ScriptRoot $PSScriptRoot -Action {
    bun install
}
