# Levanta el servidor OSRM de Ecuador con el algoritmo MLD (requerido por /trip, la ruta optima).
# Requiere `make osrm-prepare` (datos generados). Uso: make osrm-up

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$ready = Join-Path $root "infrastructure/osrm/data/ecuador-latest.osrm.cell_metrics"

if (-not (Test-Path $ready)) {
    throw "Faltan los datos de OSRM. Ejecuta primero: make osrm-prepare"
}

Push-Location $root
try {
    docker compose --profile osrm up -d --force-recreate osrm
    if ($LASTEXITCODE -ne 0) { throw "no se pudo levantar el servicio osrm" }
} finally { Pop-Location }
Write-Host "Listo: OSRM en 127.0.0.1:5000. El gateway usa OSRM_URL=http://osrm:5000."


