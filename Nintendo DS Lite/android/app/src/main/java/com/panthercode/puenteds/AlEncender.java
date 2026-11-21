package com.panthercode.puenteds;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

// Al encender el móvil, el puente vuelve a ponerse en marcha si estaba encendido.
public class AlEncender extends BroadcastReceiver {
    @Override
    public void onReceive(Context c, Intent i) {
        if (Intent.ACTION_BOOT_COMPLETED.equals(i.getAction()) && Principal.encendido(c)) {
            Servicio.encender(c, null);
        }
    }
}
