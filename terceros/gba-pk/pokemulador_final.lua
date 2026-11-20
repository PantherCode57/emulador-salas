-- ====================================================================================================
-- Pokemulador: la partida compartida va por la sala de Pokemulador, así que la dirección siempre es
-- este mismo dispositivo (127.0.0.1). Si tienes la sala abierta, abres la partida; si no, entras en
-- la de tu amigo. Si se corta (o tu amigo aún no ha abierto el juego), se vuelve a intentar cada 5 s.
-- Pokemulador cambia __MODO__ por "h" (abrir) o "c" (entrar) al preparar este archivo.
--
-- Combate a medias: si tu amigo se va (o se le corta) en mitad de un combate, GBA-PK lo quita de la
-- lista y deja de llevar el combate, y el juego se queda en «Link standby...» para siempre. Aquí el
-- combate sigue con lo último que se sabía de tu amigo, como si él ya lo hubiera terminado
-- (Text_Stage 0), y GBA-PK lo acaba por su camino de siempre («in case player is stuck»: el mando
-- MPCtrlEnd, que termina un combate por cable): vuelves al mapa con el equipo como antes del combate.
--   * Si la sala es tuya, MainLogic sigue en marcha y llama a Battlescript: lo hace el Battlescript de abajo.
--   * Si es de tu amigo, Disconnect() pone LockFromScript a 0 y MainLogic ya no corre sin conexión:
--     lo hace el "frame" de abajo, que vuelve a poner LockFromScript a 21 hasta que el combate acaba.
-- Solo cuando el combate ya iba (Text_Stage 7 o más). Si se va mientras empieza, queda como antes.
local PokemuladorSolo = false -- tu amigo se ha ido y el combate se está acabando sin él
if Battlescript and FindPlayerByID and SendSpecialData then
	local CombateGBAPK, BuscarGBAPK, EnviarGBAPK = Battlescript, FindPlayerByID, SendSpecialData
	local yo, rival, idRival = nil, nil, nil -- los del combate que va, tal como los dejó GBA-PK
	local function nada() end

	local function enCombate()
		local g = gAddress[GameID]
		return g and g.gMainCallback and g.BattleMainCB2 and emu:read32(g.gMainCallback) == g.BattleMainCB2 + 1
	end

	local function olvidar()
		yo, rival, idRival, PokemuladorSolo = nil, nil, nil, false
	end

	-- Una vuelta del combate de GBA-PK sin tu amigo.
	local function vueltaSolo()
		local b = rival:GetBattleVar()
		b.Text_Stage, b.Waiting_Status, b.Buffer_ID, b.Transfer_Stage = 0, 0, 0, 0
		b.Battleflags = {0, 0, 0, 0}
		rival:GetBattleVar2().Transfer_Stage = 0
		local mio = yo:GetBattleVar()
		mio.Waiting_Status = (mio.Waiting_Status or 0) & ~65 -- UPDATE_PKMN y SENDING: ya nadie manda el equipo
		local idYo = yo:GetID()
		FindPlayerByID = function(id)
			if id == PlayerID or id == idYo then return yo end
			if id == idRival then return rival end
			return BuscarGBAPK(id)
		end
		SendSpecialData = nada -- a tu amigo ya no le llega nada
		local bien, fallo = pcall(CombateGBAPK)
		FindPlayerByID, SendSpecialData = BuscarGBAPK, EnviarGBAPK
		if not bien then
			console:error("Pokemulador: no se ha podido acabar el combate: " .. tostring(fallo))
			LockFromScript = 0
		elseif LockFromScript == 21 and (yo:GetBattleVar().Text_Stage or 0) >= 8 and enCombate() then
			-- GBA-PK solo manda MPCtrlEnd en el Text_Stage 7: si tú estabas eligiendo (menú de LUCHAR), al
			-- elegir el juego lo borra. Se repite hasta que el juego sale de la pantalla del combate.
			local g = gAddress[GameID]
			emu:write8(g.gBattleBufferA, MPCtrlEnd)
			emu:write8(g.gBattleControllerExecFlags, (emu:read8(g.gBattleControllerExecFlags) | 1) & ~0x30)
		end
		if LockFromScript ~= 21 then
			console:log("Pokemulador: combate acabado, de vuelta al mapa")
			olvidar()
		end
	end

	local function empezarSolo()
		PokemuladorSolo = true
		console:log("Pokemulador: tu amigo se ha ido en mitad del combate; se acaba solo")
	end

	Battlescript = function()
		if not PokemuladorSolo then
			local p = FindPlayerByID(PlayerID)
			local r = p and FindPlayerByID(p:GetTalking())
			if p and r then
				yo, rival, idRival = p, r, r:GetID()
			elseif yo and rival and (yo:GetBattleVar().Text_Stage or 0) >= 7 and (not p or p == yo) then
				empezarSolo()
			end
		end
		if PokemuladorSolo then
			vueltaSolo()
			return
		end
		CombateGBAPK()
		if LockFromScript ~= 21 then olvidar() end
	end

	-- La sala era de tu amigo: sin conexión, MainLogic (y con él Battlescript) ya no corre.
	if callbacks then
		callbacks:add("frame", function()
			if Connected or Hosting or not yo or not rival then return end
			if not PokemuladorSolo then
				-- (en el 8 y el 9 el juego ya puede estar fuera de la pantalla del combate: falta devolverte el equipo)
				local etapa = yo:GetBattleVar().Text_Stage or 0
				if etapa < 7 or (etapa == 7 and not enCombate()) then return end
				empezarSolo()
			end
			LockFromScript = 21
			vueltaSolo()
		end)
	end
end

if callbacks then
	PokemuladorModo = "__MODO__"
	PokemuladorEspera = 240
	callbacks:add("frame", function()
		PokemuladorEspera = PokemuladorEspera + 1
		if PokemuladorEspera < 300 then return end
		PokemuladorEspera = 0
		-- (en mitad de un combate no: primero se acaba)
		if not EnableScript or not MPMenuMode() or Hosting or Connected or PokemuladorSolo or LockFromScript == 21 then return end
		if PokemuladorModo == "h" then
			pcall(MPHost)
		else
			pcall(MPConnect, "127.0.0.1", Port)
		end
	end)
end
