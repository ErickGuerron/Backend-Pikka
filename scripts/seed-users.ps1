# Crea usuarios de prueba, uno por rol, a través de la API del Gateway.
#
# Solo para desarrollo local. Las contraseñas son de prueba y no deben usarse
# en ningún otro entorno.
#
# Requisitos: el stack levantado (docker compose up) y BOOTSTRAP_ADMIN_* en .env,
# porque el administrador inicial es quien crea a los demás usuarios.
#
# Uso:
#   powershell -ExecutionPolicy Bypass -File scripts/seed-users.ps1
#
# Es idempotente: si un usuario ya existe (409), se omite.

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $root '.env'
if (-not (Test-Path -LiteralPath $envFile)) { throw "No existe $envFile. Copia .env.example y ajusta las variables." }

$config = @{}
foreach ($line in Get-Content -LiteralPath $envFile -Encoding UTF8) {
    if ($line -match '^\s*([A-Z_]+)=(.*)$') { $config[$matches[1]] = $matches[2].Trim('"') }
}

$adminEmail = $config['BOOTSTRAP_ADMIN_EMAIL']
$adminPassword = $config['BOOTSTRAP_ADMIN_PASSWORD']
if (-not $adminEmail -or -not $adminPassword) { throw 'BOOTSTRAP_ADMIN_EMAIL y BOOTSTRAP_ADMIN_PASSWORD deben estar en .env.' }

$port = if ($config['GATEWAY_PORT']) { $config['GATEWAY_PORT'] } else { '8080' }
$baseUrl = if ($env:GATEWAY_URL) { $env:GATEWAY_URL } else { "http://localhost:$port" }

function Invoke-Api {
    param([string]$Method, [string]$Path, [object]$Body, [string]$Token)
    $headers = @{ Accept = 'application/json' }
    if ($Token) { $headers.Authorization = "Bearer $Token" }
    $params = @{ Method = $Method; Uri = "$baseUrl$Path"; Headers = $headers; UseBasicParsing = $true }
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Compress)
        $params.ContentType = 'application/json'
    }
    try {
        $response = Invoke-WebRequest @params
        return [pscustomobject]@{ Status = [int]$response.StatusCode; Body = $response.Content }
    } catch {
        $resp = $_.Exception.Response
        if ($null -eq $resp) { throw "Sin respuesta de $baseUrl. ¿Está levantado el stack?" }
        $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
        return [pscustomobject]@{ Status = [int]$resp.StatusCode; Body = $reader.ReadToEnd() }
    }
}

Write-Host "Gateway: $baseUrl"

$login = Invoke-Api -Method 'POST' -Path '/api/v1/auth/login' -Body @{ email = $adminEmail; password = $adminPassword }
if ($login.Status -ne 200) { throw "No se pudo iniciar sesión como administrador inicial (HTTP $($login.Status))." }
$token = ($login.Body | ConvertFrom-Json).accessToken

# Un usuario por rol implementado. CUSTOMER aún no existe en Auth Service (ADR 0007).
$users = @(
    @{ email = 'operador@pikka.test'; password = 'Operador-dev-2026'; fullName = 'Olivia Operadora'; role = 'OPERATOR' },
    @{ email = 'repartidor@pikka.test'; password = 'Repartidor-dev-2026'; fullName = 'Diego Repartidor'; role = 'DRIVER' }
)

Write-Host ''
foreach ($user in $users) {
    $result = Invoke-Api -Method 'POST' -Path '/api/v1/users' -Body $user -Token $token
    switch ($result.Status) {
        201 { Write-Host "creado    $($user.role.PadRight(9)) $($user.email)  contraseña: $($user.password)" }
        409 { Write-Host "ya existe $($user.role.PadRight(9)) $($user.email)" }
        default { throw "Error creando $($user.email): HTTP $($result.Status) $($result.Body)" }
    }
}

Write-Host ''
Write-Host "Administrador inicial: $adminEmail (contraseña en BOOTSTRAP_ADMIN_PASSWORD de .env)"
Write-Host 'CUSTOMER: pendiente, aún no está implementado en Auth Service.'
