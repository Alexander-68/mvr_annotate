$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$zip = 'mvr_annotate.zip'
if (Test-Path $zip) { Remove-Item $zip }
Compress-Archive -Path index.html, fv.png, mvr_annotate.json, assets -DestinationPath $zip
Write-Output "built $zip ($((Get-Item $zip).Length) bytes)"
