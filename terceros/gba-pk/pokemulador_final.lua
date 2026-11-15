
-- ====================================================================================================
-- Pokemulador: la partida compartida va por la sala de Pokemulador, así que la dirección siempre es
-- este mismo dispositivo (127.0.0.1). Si tienes la sala abierta, abres la partida; si no, entras en
-- la de tu amigo. Si se corta (o tu amigo aún no ha abierto el juego), se vuelve a intentar cada 5 s.
-- Pokemulador cambia __MODO__ por "h" (abrir) o "c" (entrar) al preparar este archivo.
if callbacks then
	PokemuladorModo = "__MODO__"
	PokemuladorEspera = 240
	callbacks:add("frame", function()
		PokemuladorEspera = PokemuladorEspera + 1
		if PokemuladorEspera < 300 then return end
		PokemuladorEspera = 0
		if not EnableScript or not MPMenuMode() or Hosting or Connected then return end
		if PokemuladorModo == "h" then
			pcall(MPHost)
		else
			pcall(MPConnect, "127.0.0.1", Port)
		end
	end)
end
