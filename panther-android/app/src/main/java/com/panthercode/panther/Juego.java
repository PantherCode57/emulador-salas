package com.panthercode.panther;

import android.os.Build;
import android.os.Bundle;
import android.view.Display;
import android.view.DisplayCutout;
import android.view.WindowManager;

import org.libsdl.app.SDLActivity;

import java.io.File;
import java.util.ArrayList;
import java.util.List;

/**
 * El juego: el emulador de Panther OS (consola/emulador, en C, con SDL) a pantalla completa. Va en su
 * propio proceso (":juego") y lo cierra al salir, para que cada partida empiece de cero.
 */
public class Juego extends SDLActivity {
    static final String JUEGO = "juego", CARAMELOS = "caramelos";
    private static volatile int recorte;

    @Override
    protected void onCreate(Bundle guardado) {
        super.onCreate(guardado);
        Juegos.prepararArchivos(this);
        pedir60Hz();
        // Lo que tapa la cámara: el emulador no pone botones debajo.
        getWindow().getDecorView().setOnApplyWindowInsetsListener((v, ins) -> {
            if (Build.VERSION.SDK_INT >= 28) {
                DisplayCutout c = ins.getDisplayCutout();
                if (c != null)
                    recorte = Math.max(Math.max(c.getSafeInsetTop(), c.getSafeInsetBottom()),
                            Math.max(c.getSafeInsetLeft(), c.getSafeInsetRight()));
            }
            return v.onApplyWindowInsets(ins);
        });
        if (Build.VERSION.SDK_INT >= 28) {
            WindowManager.LayoutParams lp = getWindow().getAttributes();
            lp.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
            getWindow().setAttributes(lp);
        }
    }

    /** Las GB y GBA van a 60 imágenes por segundo: si la pantalla puede ponerse a 60 Hz, mejor. */
    private void pedir60Hz() {
        Display d = getWindowManager().getDefaultDisplay();
        Display.Mode actual = d.getMode(), mejor = null;
        for (Display.Mode m : d.getSupportedModes()) {
            if (m.getPhysicalWidth() != actual.getPhysicalWidth() || m.getPhysicalHeight() != actual.getPhysicalHeight())
                continue;
            if (mejor == null || Math.abs(m.getRefreshRate() - 60) < Math.abs(mejor.getRefreshRate() - 60)) mejor = m;
        }
        if (mejor != null && mejor.getModeId() != actual.getModeId()) {
            WindowManager.LayoutParams lp = getWindow().getAttributes();
            lp.preferredDisplayModeId = mejor.getModeId();
            getWindow().setAttributes(lp);
        }
    }

    @Override
    protected String[] getLibraries() {
        return new String[]{"SDL2", "main"};
    }

    @Override
    protected String[] getArguments() {
        List<String> a = new ArrayList<>();
        a.add("--casa");
        a.add(getFilesDir().getAbsolutePath());
        a.add("--recorte");
        a.add(String.valueOf(recorte));
        if (getIntent().getBooleanExtra(CARAMELOS, false)) {
            a.add("--script");
            a.add(new File(getFilesDir(), "caramelos-frlg.lua").getAbsolutePath());
        }
        String juego = getIntent().getStringExtra(JUEGO);
        if (juego != null) a.add(juego);
        return a.toArray(new String[0]);
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        // El emulador ya ha guardado (al salir del juego o al cerrar la pantalla): se cierra el proceso.
        System.exit(0);
    }
}
