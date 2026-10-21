# Prepara los datos de OSRM para el mapa de Ecuador (MVP).
#
# 1. Descarga el extracto de OpenStreetMap de Geofabrik (si no existe).
# 2. Ejecuta osrm-extract, osrm-partition y osrm-customize (MLD) como servicio del compose (queda dentro del grupo).
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

Write-Host "== osrm-extract + osrm-partition + osrm-customize (MLD) como servicio del compose; puede tardar varios minutos"
Push-Location $root
try {
    docker compose --profile osrm-prepare up --abort-on-container-exit --exit-code-from osrm-prepare osrm-prepare
    $code = $LASTEXITCODE
    docker compose --profile osrm-prepare rm -f osrm-prepare | Out-Null
} finally { Pop-Location }
if ($code -ne 0) { throw "la preparacion de OSRM fallo con codigo $code" }
if (-not (Test-Path (Join-Path $dataDir "$baseName.cell_metrics"))) { throw "no se genero $baseName.cell_metrics" }

Write-Host "Listo. Levanta el servidor con: make osrm-up"

