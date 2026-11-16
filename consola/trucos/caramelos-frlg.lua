-- Caramelos Raros infinitos en Pokémon Rojo Fuego y Verde Hoja (versión de EE. UU.).
--
-- Lo carga el mGBA de Pokemulador (como el script de GBA-PK) cuando se activa la opción en la app
-- Pokémon de la consola. Cada medio segundo busca los Caramelos Raros en el bolsillo de objetos de
-- la mochila y los deja en 999; si no hay ninguno, los pone en el primer hueco libre. No cambia la
-- ROM ni nada más de la partida.
--
-- Dónde está cada cosa (del proyecto pret/pokefirered, que descompiló el juego):
--   gSaveBlock1Ptr = 0x03005008, gSaveBlock2Ptr = 0x0300500C (los dos se mueven por la memoria)
--   SaveBlock1 + 0x310: bolsillo de objetos, 42 huecos de {id u16, cantidad u16}
--   SaveBlock2 + 0xF20: clave con la que va cifrada la cantidad (cantidad XOR clave & 0xFFFF)
--   Caramelo Raro = objeto 68

local CARAMELO = 68
local CUANTOS = 999
local HUECOS = 42

local function enRam(p)
	return p >= 0x02000000 and p < 0x02040000
end

local vueltas = 0
local function caramelos()
	vueltas = vueltas + 1
	if vueltas % 30 ~= 0 then
		return
	end
	local sb1 = emu:read32(0x03005008)
	local sb2 = emu:read32(0x0300500C)
	if not enRam(sb1) or not enRam(sb2) then
		return
	end
	local clave = emu:read32(sb2 + 0xF20) & 0xFFFF
	local cantidad = CUANTOS ~ clave
	local bolsillo = sb1 + 0x310
	local libre = nil
	for i = 0, HUECOS - 1 do
		local hueco = bolsillo + i * 4
		local id = emu:read16(hueco)
		if id == CARAMELO then
			if emu:read16(hueco + 2) ~= cantidad then
				emu:write16(hueco + 2, cantidad)
			end
			return
		end
		if id == 0 and libre == nil then
			libre = hueco
		end
	end
	if libre ~= nil then
		emu:write16(libre, CARAMELO)
		emu:write16(libre + 2, cantidad)
	end
end

local juego = emu:getGameCode()
if juego == "AGB-BPRE" or juego == "AGB-BPGE" then
	callbacks:add("frame", caramelos)
end
