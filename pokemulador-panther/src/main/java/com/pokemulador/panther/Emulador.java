package com.pokemulador.panther;

/** Lo que la app le dice al emulador Panther (libpanther.so, que carga SDLActivity). Ver poke_ordenes.c. */
public final class Emulador {
    /** Guarda dónde ibas y sale del juego. */
    public static final String SALIR = "QUIT";
    /** Guarda una captura del juego en la carpeta de capturas. */
    public static final String CAPTURA = "SCREENSHOT";

    private Emulador() {
    }

    public static native void orden(String texto);

    /** -1 si el juego sigue abierto, 0 si ha acabado bien, otra cosa si no se ha podido abrir. */
    public static native int resultado();
}
