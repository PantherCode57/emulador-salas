@echo off
rem Compila el núcleo libretro de mGBA de Pokemulador (con scripts de Lua) para Windows x64 con
rem Visual Studio. Uso: compilar-windows.bat <carpeta de mgba ya preparada> <carpeta de Lua src> <salida>
rem Tiene que ser con Visual Studio: con MinGW los scripts no funcionan.
setlocal
set "MGBA=%~1"
set "LUASRC=%~2"
set "SALIDA=%~3"
set "VSWHERE=%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe"
if defined VCINSTALLDIR goto vs_listo
for /f "usebackq delims=" %%i in (`"%VSWHERE%" -latest -products * -property installationPath`) do set "VS=%%i"
call "%VS%\VC\Auxiliary\Build\vcvars64.bat" >nul || exit /b 1
:vs_listo
rem CMake y Ninja de Visual Studio, por si no están en el PATH.
set "PATH=%PATH%;%VS%\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin;%VS%\Common7\IDE\CommonExtensions\Microsoft\CMake\Ninja"
mkdir "%SALIDA%\lua-win" 2>nul
pushd "%SALIDA%\lua-win"
for %%f in ("%LUASRC%\*.c") do (
  if /I not "%%~nf"=="lua" if /I not "%%~nf"=="luac" cl /nologo /O2 /MD /c "%%f" || exit /b 1
)
lib /nologo /OUT:liblua.lib *.obj || exit /b 1
popd
cmake -G Ninja -S "%MGBA%" -B "%SALIDA%\build-win" -DCMAKE_BUILD_TYPE=Release -DBUILD_LIBRETRO=ON -DSKIP_LIBRARY=ON -DBUILD_QT=OFF -DBUILD_SDL=OFF -DUSE_DEBUGGERS=OFF -DUSE_GDB_STUB=OFF -DUSE_EDITLINE=OFF -DUSE_FFMPEG=OFF -DUSE_ZLIB=OFF -DUSE_MINIZIP=OFF -DUSE_PNG=OFF -DUSE_LIBZIP=OFF -DUSE_SQLITE3=OFF -DUSE_ELF=OFF -DUSE_LZMA=OFF -DUSE_EPOXY=OFF -DUSE_DISCORD_RPC=OFF -DBUILD_GL=OFF -DBUILD_GLES2=OFF -DBUILD_GLES3=OFF -DENABLE_SCRIPTING=ON -DUSE_LUA=5.4 "-DLUA_INCLUDE_DIR=%LUASRC%" "-DLUA_LIBRARY=%SALIDA%\lua-win\liblua.lib" || exit /b 1
cmake --build "%SALIDA%\build-win" --target mgba_libretro || exit /b 1
copy /y "%SALIDA%\build-win\mgba_libretro.dll" "%SALIDA%\mgba_libretro.dll" >nul || exit /b 1
echo Nucleo de Windows listo: %SALIDA%\mgba_libretro.dll
