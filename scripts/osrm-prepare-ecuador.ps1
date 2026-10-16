# Prepara los datos de OSRM para el mapa de Ecuador (MVP).
#
# 1. Descarga el extracto de OpenStreetMap de Geofabrik (si no existe).
# 2. Ejecuta osrm-extract, osrm-partition y osrm-customize dentro de Docker.
#
# Los datos quedan en infrastructure/osrm/data (ignorado por git). Tarda unos minutos
# y necesita unos 2 GB de RAM para el paso de extracción.
#
# Uso:  pwsh scripts/osrm-prepare-ecuador.ps1 [-Force]
#   -Force  vuelve a descargar y regenerar aunque ya existan los datos.

param(
    [switch]$Force
)

$ErrorActionPreference = "Stop"

$image = "osrm/osrm-backend:v5.25.0"
$pbfUrl = "https://download.geofabrik.de/south-america/ecuador-latest.osm.pbf"
$root = Split-Path -Parent $PSScriptRoot
$dataDir = Join-Path $root "infrastructure/osrm/data"
$pbfName = "ecuador-latest.osm.pbf"
$baseName = "ecuador-latest.osrm"
$pbfPath = Join-Path $dataDir $pbfName

New-Item -ItemType Directory -Force -Path $dataDir | Out-Null

if ($Force -or -not (Test-Path $pbfPath)) {
    Write-Host "Descargando $pbfUrl ..."
    Invoke-WebRequest -Uri $pbfUrl -OutFile $pbfPath -UseBasicParsing
}
Write-Host ("Extracto: {0:N1} MB" -f ((Get-Item $pbfPath).Length / 1MB))

$mount = (Resolve-Path $dataDir).Path

function Invoke-Osrm([string]$step, [string[]]$command) {
    Write-Host "== $step"
    docker run --rm -v "${mount}:/data" $image @command
    if ($LASTEXITCODE -ne 0) {
        throw "$step fallo con codigo $LASTEXITCODE"
    }
}

Invoke-Osrm "osrm-extract" @("osrm-extract", "-p", "/opt/car.lua", "/data/$pbfName")
Invoke-Osrm "osrm-partition" @("osrm-partition", "/data/$baseName")
Invoke-Osrm "osrm-customize" @("osrm-customize", "/data/$baseName")

Write-Host "Listo. Levanta el servidor con: make osrm-up"
