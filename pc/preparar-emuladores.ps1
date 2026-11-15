# Descarga y coloca los emuladores que lleva dentro Pokemulador para PC.
# Uso (desde la carpeta pc):  powershell -ExecutionPolicy Bypass -File .\preparar-emuladores.ps1
# Resultado: pc\emuladores\RetroArch (con 4 núcleos) y pc\emuladores\Azahar.

$ErrorActionPreference = 'Stop'
$RetroArchVersion = '1.22.2'
$AzaharVersion = '2126.2'
$Cores = @('gambatte', 'mgba', 'melondsds', 'mupen64plus_next')

$pc = $PSScriptRoot
$descargas = Join-Path $pc 'descargas'
$emuladores = Join-Path $pc 'emuladores'
New-Item -ItemType Directory -Force (Join-Path $descargas 'retroarch'), (Join-Path $descargas 'cores'), (Join-Path $descargas 'azahar') | Out-Null
New-Item -ItemType Directory -Force $emuladores | Out-Null

function Descargar($url, $destino) {
    if (-not (Test-Path $destino)) {
        Write-Host "Descargando $url"
        Invoke-WebRequest -Uri $url -OutFile $destino
    }
}

# RetroArch
$ra7z = Join-Path $descargas 'retroarch\RetroArch.7z'
Descargar "https://buildbot.libretro.com/stable/$RetroArchVersion/windows/x86_64/RetroArch.7z" $ra7z
$raDestino = Join-Path $emuladores 'RetroArch'
if (-not (Test-Path $raDestino)) {
    $tmp = Join-Path $emuladores 'retroarch-tmp'
    New-Item -ItemType Directory -Force $tmp | Out-Null
    & "$env:SystemRoot\System32\tar.exe" -xf $ra7z -C $tmp
    Move-Item (Join-Path $tmp 'RetroArch-Win64') $raDestino
    Remove-Item $tmp
    # Quitamos lo que no usamos para que el instalador pese menos.
    foreach ($sobra in 'assets\xmb', 'assets\glui', 'assets\rgui', 'shaders', 'overlays', 'database', 'autoconfig\hid') {
        $ruta = Join-Path $raDestino $sobra
        if (Test-Path $ruta) { Remove-Item -Recurse -Force $ruta }
    }
    Get-ChildItem (Join-Path $raDestino 'info') -Filter *.info |
        Where-Object { $Cores -notcontains ($_.BaseName -replace '_libretro$', '') } |
        Remove-Item -Force
}

# Núcleos
New-Item -ItemType Directory -Force (Join-Path $raDestino 'cores') | Out-Null
foreach ($core in $Cores) {
    $zip = Join-Path $descargas "cores\${core}_libretro.dll.zip"
    Descargar "https://buildbot.libretro.com/nightly/windows/x86_64/latest/${core}_libretro.dll.zip" $zip
    Expand-Archive -Force $zip (Join-Path $raDestino 'cores')
}
Copy-Item -Force (Join-Path $pc 'config\pokemulador.cfg') (Join-Path $raDestino 'pokemulador.cfg')

# mGBA de Pokemulador (con scripts de Lua, para la partida compartida de GBA): lo compila GitHub
# Actions (artefacto "nucleo-mgba" de la última ejecución correcta) y sustituye al de libretro.
$gh = Get-Command gh -ErrorAction SilentlyContinue
if ($gh) {
    $run = gh run list -R PantherCode57/emulador-salas -w 'Compilar APK' -s success -L 1 --json databaseId --jq '.[0].databaseId'
    $tmpMgba = Join-Path $descargas 'nucleo-mgba'
    if (Test-Path $tmpMgba) { Remove-Item -Recurse -Force $tmpMgba }
    gh run download $run -R PantherCode57/emulador-salas -n nucleo-mgba -D $tmpMgba
    Copy-Item -Force (Join-Path $tmpMgba 'mgba_libretro.dll') (Join-Path $raDestino 'cores\mgba_libretro.dll')
} else {
    Write-Warning 'Sin gh: se queda el mGBA normal (no habrá partida compartida en GBA).'
}

# GBA-PK Multiplayer (partida compartida de GBA), con su licencia.
$gbapk = Join-Path $emuladores 'GBA-PK'
New-Item -ItemType Directory -Force $gbapk | Out-Null
Copy-Item -Force (Join-Path (Split-Path $pc -Parent) 'terceros\gba-pk\*') $gbapk

# Azahar (3DS). La carpeta "user" vacía hace que guarde su configuración a su lado (modo portátil).
$azZip = Join-Path $descargas "azahar\azahar-windows-msvc-$AzaharVersion.zip"
Descargar "https://github.com/azahar-emu/azahar/releases/download/$AzaharVersion/azahar-windows-msvc-$AzaharVersion.zip" $azZip
$azDestino = Join-Path $emuladores 'Azahar'
if (-not (Test-Path (Join-Path $azDestino 'azahar.exe'))) {
    $tmp = Join-Path $emuladores 'azahar-tmp'
    Expand-Archive -Force $azZip $tmp
    if (Test-Path $azDestino) { Remove-Item -Recurse -Force $azDestino }
    Move-Item (Join-Path $tmp "azahar-windows-msvc-$AzaharVersion") $azDestino
    Remove-Item -Recurse $tmp
}
New-Item -ItemType Directory -Force (Join-Path $azDestino 'user') | Out-Null
# El instalador no copia carpetas vacías: dejamos un archivo para que "user" exista.
Set-Content -Path (Join-Path $azDestino 'user\LEEME.txt') -Value 'Configuración de Azahar dentro de Pokemulador.' -Encoding UTF8

# Enlace online (Go + Tailscale). Necesita Go: herramientas\go o el que haya instalado.
$go = Join-Path (Split-Path $pc -Parent) 'herramientas\go\bin\go.exe'
if (-not (Test-Path $go)) { $go = 'go' }
$enlaceDestino = Join-Path $emuladores 'Enlace'
New-Item -ItemType Directory -Force $enlaceDestino | Out-Null
Push-Location (Join-Path $pc 'enlace')
& $go build -p 2 -trimpath -ldflags '-s -w' -o (Join-Path $enlaceDestino 'enlace.exe') .
Pop-Location

Write-Host 'Emuladores listos en' $emuladores
