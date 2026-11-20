-- Prueba de terceros/gba-pk/pokemulador_final.lua: si tu amigo se va en mitad de un combate, el combate
-- se acaba solo. La pasa consola.yml con el emulador y una ROM mínima:
--   ./emulador --medir 300 --script pruebas/combate_solo.lua --script <pokemulador_final.lua> prueba.gba
-- Va antes que el archivo: pone un GBA-PK de mentira (los nombres que usa el archivo) y luego hace
-- de MainLogic y del juego, imagen a imagen. Al final escribe «prueba combate: todo bien» o «MAL».
local RAM = 0x02000000
local CB2_COMBATE, CB2_MAPA = 0x08000100, 0x08000200
local BUFFER_A, EXEC = RAM + 0x100, RAM + 0x200
GameID = "PRUEBA"
gAddress = {PRUEBA = {gMainCallback = RAM, BattleMainCB2 = CB2_COMBATE,
	gBattleBufferA = BUFFER_A, gBattleControllerExecFlags = EXEC}}
MPCtrlEnd = 0x37
EnableScript = false -- así la parte que entra en la sala no hace nada
LockFromScript, PlayerID, Connected, Hosting = 0, 1, false, true

local enviados, fallos = 0, {}
local function mal(que) fallos[#fallos + 1] = que end

local Jugador = {}
Jugador.__index = Jugador
local function jugador(id, habla, etapa)
	return setmetatable({id = id, habla = habla,
		BattleVars = {Text_Stage = etapa, Waiting_Status = 64, Buffer_ID = 0, Transfer_Stage = 7, Battleflags = {0, 0, 0, 0}},
		BattleVars2 = {Transfer_Stage = 1}}, Jugador)
end
function Jugador:GetID() return self.id end
function Jugador:GetTalking() return self.habla end
function Jugador:GetBattleVar() return self.BattleVars end
function Jugador:GetBattleVar2() return self.BattleVars2 end

players = {}
function FindPlayerByID(id)
	for _, p in ipairs(players) do
		if p.id == id then return p end
	end
	return nil
end
function SendSpecialData() enviados = enviados + 1 end
local BuscarDeVerdad, EnviarDeVerdad = FindPlayerByID, SendSpecialData

-- Los pasos del final de un combate del Battlescript de GBA-PK (Text_Stage 7, 8 y 9), en pequeño.
function Battlescript()
	local p = FindPlayerByID(PlayerID)
	if not p then LockFromScript = 0 return end
	local o = FindPlayerByID(p:GetTalking())
	if not o then LockFromScript = 0 return end
	local b, ob = p:GetBattleVar(), o:GetBattleVar()
	if (b.Waiting_Status & 64) ~= 0 then return end -- espera el equipo del otro
	if b.Text_Stage == 7 and (ob.Text_Stage > 7 or ob.Text_Stage == 0) then
		if emu:read8(BUFFER_A) == MPCtrlEnd then
			b.Text_Stage = 8
		else
			emu:write8(BUFFER_A, MPCtrlEnd)
			emu:write8(EXEC, 1)
		end
	elseif b.Text_Stage == 8 and (ob.Text_Stage > 7 or ob.Text_Stage == 0) then
		b.Text_Stage = 9
	elseif b.Text_Stage == 9 and emu:read32(RAM) ~= CB2_COMBATE + 1 then
		b.Text_Stage = 0
		LockFromScript = 0
		SendSpecialData(nil, "BATT", o:GetID())
	end
end

-- El juego: sale del combate cuando le llega MPCtrlEnd con su bit. Con «menu», estabas eligiendo:
-- en el Text_Stage 9 eliges (el juego borra el mando: 0x38) y hace falta que llegue otra vez.
local borrado = false
local function juego(yo, menu)
	if emu:read32(RAM) ~= CB2_COMBATE + 1 then return end
	if menu and yo.BattleVars.Text_Stage == 9 and not borrado then
		borrado = true
		emu:write8(BUFFER_A, 0x38)
		emu:write8(EXEC, 0)
	elseif (borrado or not menu) and emu:read8(BUFFER_A) == MPCtrlEnd and (emu:read8(EXEC) & 1) ~= 0 then
		emu:write32(RAM, CB2_MAPA + 1)
	end
end

-- Y que GBA-PK con este archivo detrás, como lo prepara la app, se puede cargar.
local function leer(ruta)
	local f = io.open(ruta, "rb")
	if not f then return nil end
	local t = f:read("a")
	f:close()
	return t
end
local gbapk = leer("../../terceros/gba-pk/GBA-PK_Multiplayer.lua")
local final = leer("../../terceros/gba-pk/pokemulador_final.lua")
if not (gbapk and final) then
	mal("no encuentro los archivos de GBA-PK")
else
	local junto, err = load(gbapk .. "\n" .. final:gsub("__MODO__", "h"), "=multijugador.lua")
	if not junto then mal("GBA-PK con pokemulador_final.lua no se carga: " .. tostring(err)) end
end

local casos = {}
-- 1. La sala es tuya y tu amigo se va: GBA-PK lo quita de la lista; MainLogic sigue.
casos[1] = {nombre = "sala tuya", empezar = function()
	PlayerID, Hosting, Connected = 1, true, false
	local yo, otro = jugador(1, 2, 7), jugador(2, 1, 7)
	players = {yo, otro}
	return yo
end, irse = function()
	table.remove(players, 2)
end, enviados = 0, menu = true}
-- 2. La sala es de tu amigo y se va: Disconnect() vacía la lista y MPResetGameState() pone LockFromScript a 0.
casos[2] = {nombre = "sala de tu amigo", empezar = function()
	PlayerID, Hosting, Connected = 2, false, true
	local yo, otro = jugador(2, 1, 7), jugador(1, 2, 7)
	players = {otro, yo}
	return yo
end, irse = function()
	players, PlayerID, Connected, LockFromScript = {}, 0, false, 0
end, enviados = 0, menu = true}
-- 3. Combate normal: tu amigo lo termina antes (Text_Stage 0) y sigue en la sala. Como siempre.
casos[3] = {nombre = "combate normal", empezar = function()
	PlayerID, Hosting, Connected = 1, true, false
	local yo, otro = jugador(1, 2, 7), jugador(2, 1, 7)
	players = {yo, otro}
	return yo
end, irse = function()
	players[2].BattleVars.Text_Stage = 0
	players[1].BattleVars.Waiting_Status = 0
end, enviados = 1}
-- 4. Se va mientras el combate empieza (Text_Stage 5): como antes, GBA-PK lo deja.
casos[4] = {nombre = "al empezar", empezar = function()
	PlayerID, Hosting, Connected = 1, true, false
	local yo, otro = jugador(1, 2, 5), jugador(2, 1, 5)
	players = {yo, otro}
	return yo
end, irse = function()
	table.remove(players, 2)
end, enviados = 0, sin_acabar = true}

local n, caso, yo, imagen = 0, 0, nil, 0
callbacks:add("frame", function()
	n = n + 1
	if caso > #casos then return end
	if yo == nil then
		caso = caso + 1
		if caso > #casos then
			if #fallos == 0 then
				console:error("prueba combate: todo bien")
			else
				console:error("prueba combate: MAL: " .. table.concat(fallos, "; "))
			end
			return
		end
		enviados, borrado, imagen = 0, false, 0
		emu:write32(RAM, CB2_COMBATE + 1)
		emu:write8(BUFFER_A, 0x20)
		emu:write8(EXEC, 0)
		yo = casos[caso].empezar()
		LockFromScript = 21
		Battlescript() -- una vuelta con los dos
		casos[caso].irse()
		return
	end
	local c = casos[caso]
	imagen = imagen + 1
	if (Connected or Hosting) and LockFromScript == 21 then Battlescript() end -- MainLogic
	juego(yo, c.menu)
	local acabado = LockFromScript ~= 21 and emu:read32(RAM) == CB2_MAPA + 1 and yo.BattleVars.Text_Stage == 0
	if c.sin_acabar and imagen == 20 then
		if LockFromScript ~= 0 then mal(c.nombre .. ": LockFromScript " .. LockFromScript) end
		acabado = true
	end
	if acabado or imagen > 100 then
		if not acabado then
			mal(c.nombre .. ": no se acaba (Text_Stage " .. tostring(yo.BattleVars.Text_Stage) .. ", LockFromScript " .. LockFromScript .. ")")
		end
		if enviados ~= c.enviados then mal(c.nombre .. ": " .. enviados .. " envíos") end
		if FindPlayerByID ~= BuscarDeVerdad or SendSpecialData ~= EnviarDeVerdad then
			mal(c.nombre .. ": FindPlayerByID o SendSpecialData sin devolver")
		end
		console:error("prueba combate: " .. c.nombre .. " en " .. imagen .. " imágenes")
		yo = nil
	end
end)
