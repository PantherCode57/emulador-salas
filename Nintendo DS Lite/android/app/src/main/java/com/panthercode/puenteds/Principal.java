package com.panthercode.puenteds;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.PowerManager;
import android.provider.Settings;
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import java.io.File;
import java.io.RandomAccessFile;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.NetworkInterface;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

// La pantalla de Puente DS: si está en marcha, el código de 6 cifras para escribirlo en la DS, la
// IP del móvil y los pasos. Desde aquí se enciende y se apaga.
public class Principal extends Activity {
    private TextView estado, codigo, ips, registro, instagram;
    private Button salirIG;
    private Button boton;
    private final Handler reloj = new Handler(Looper.getMainLooper());

    static boolean encendido(Context c) {
        return c.getSharedPreferences("puente", MODE_PRIVATE).getBoolean("encendido", true);
    }

    private static void guardarEncendido(Context c, boolean si) {
        SharedPreferences.Editor e = c.getSharedPreferences("puente", MODE_PRIVATE).edit();
        e.putBoolean("encendido", si);
        e.apply();
    }

    private int dp(int v) {
        return (int) (v * getResources().getDisplayMetrics().density + 0.5f);
    }

    private TextView texto(LinearLayout l, String s, int tam, int color, boolean negrita) {
        TextView t = new TextView(this);
        t.setText(s);
        t.setTextSize(tam);
        t.setTextColor(color);
        if (negrita) {
            t.setTypeface(Typeface.DEFAULT_BOLD);
        }
        t.setPadding(0, dp(6), 0, dp(6));
        l.addView(t);
        return t;
    }

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        ScrollView sv = new ScrollView(this);
        sv.setBackgroundColor(Color.rgb(11, 18, 32));
        LinearLayout l = new LinearLayout(this);
        l.setOrientation(LinearLayout.VERTICAL);
        l.setPadding(dp(20), dp(28), dp(20), dp(20));
        sv.addView(l);

        texto(l, "Puente DS", 28, Color.WHITE, true);
        estado = texto(l, "", 18, Color.rgb(30, 215, 96), false);
        texto(l, "Código para la DS (se escribe una vez en la DS, en WhatsApp, Instagram o YouTube):", 15, Color.rgb(200, 200, 212), false);
        codigo = texto(l, "…", 48, Color.rgb(196, 170, 255), true);
        codigo.setGravity(Gravity.CENTER);
        ips = texto(l, "", 15, Color.rgb(200, 200, 212), false);
        texto(l, "1. La DS y este móvil tienen que estar en el mismo wifi, y para la DS tiene que ser sin "
                + "contraseña (o con WEP): una red de invitados del router de casa, o la zona wifi de este "
                + "móvil sin contraseña.\n"
                + "2. En la DS abre WhatsApp y escribe el código de arriba.\n"
                + "3. Vincula tu WhatsApp desde la DS: escribe tu número y mete en tu móvil el código que "
                + "te da (WhatsApp → Dispositivos vinculados → Vincular con el número de teléfono).\n"
                + "4. Para Instagram, entra aquí abajo con tu cuenta (una sola vez).\n"
                + "5. YouTube no necesita nada: busca desde la DS.", 14,
                Color.rgb(150, 150, 165), false);

        texto(l, "Instagram", 20, Color.WHITE, true);
        instagram = texto(l, "", 15, Color.rgb(200, 200, 212), false);
        Button entrarIG = new Button(this);
        entrarIG.setText("Entrar en Instagram");
        entrarIG.setOnClickListener(v -> startActivity(new Intent(this, EntrarInstagram.class)));
        l.addView(entrarIG);
        salirIG = new Button(this);
        salirIG.setText("Salir de Instagram en la DS");
        salirIG.setOnClickListener(v -> {
            File d = EntrarInstagram.carpeta(this);
            d.mkdirs();
            try {
                new File(d, "salir").createNewFile();
            } catch (Exception ignorado) {
            }
            reloj.postDelayed(this::refrescar, 3000);
        });
        l.addView(salirIG);

        boton = new Button(this);
        boton.setOnClickListener(v -> {
            boolean ahora = !Servicio.enMarcha;
            guardarEncendido(this, ahora);
            if (ahora) {
                Servicio.encender(this, getIntent());
            } else {
                Servicio.apagar(this);
            }
            reloj.postDelayed(this::refrescar, 500);
        });
        l.addView(boton);

        if (Build.VERSION.SDK_INT >= 23) {
            Button bateria = new Button(this);
            bateria.setText("Que Android no lo cierre (batería)");
            bateria.setOnClickListener(v -> pedirBateria());
            l.addView(bateria);
        }

        Button verRegistro = new Button(this);
        verRegistro.setText("Ver el registro");
        verRegistro.setOnClickListener(v -> registro.setVisibility(
                registro.getVisibility() == View.VISIBLE ? View.GONE : View.VISIBLE));
        l.addView(verRegistro);
        registro = texto(l, "", 11, Color.rgb(150, 150, 165), false);
        registro.setTypeface(Typeface.MONOSPACE);
        registro.setVisibility(View.GONE);

        setContentView(sv);

        if (Build.VERSION.SDK_INT >= 33) {
            requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"}, 1);
        }
        if (encendido(this) || getIntent().hasExtra("falso")) {
            guardarEncendido(this, true);
            Servicio.encender(this, getIntent());
        }
    }

    private void pedirBateria() {
        PowerManager pm = (PowerManager) getSystemService(POWER_SERVICE);
        if (Build.VERSION.SDK_INT >= 23 && !pm.isIgnoringBatteryOptimizations(getPackageName())) {
            try {
                startActivity(new Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                        Uri.parse("package:" + getPackageName())));
            } catch (Exception e) {
                startActivity(new Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS));
            }
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        refrescar();
    }

    @Override
    protected void onPause() {
        super.onPause();
        reloj.removeCallbacksAndMessages(null);
    }

    private void refrescar() {
        boolean si = Servicio.enMarcha;
        estado.setText(si ? "En marcha: la DS puede conectarse" : "Apagado");
        estado.setTextColor(si ? Color.rgb(30, 215, 96) : Color.rgb(240, 80, 90));
        boton.setText(si ? "Apagar el puente" : "Encender el puente");
        String c = leer(new File(Servicio.datos(this), "codigo"), 32).trim();
        codigo.setText(c.length() == 6 ? c.substring(0, 3) + " " + c.substring(3) : "…");
        List<String> l = direcciones();
        ips.setText(l.isEmpty() ? "Este móvil no está en ningún wifi" : "Este móvil en el wifi: " + android.text.TextUtils.join(", ", l));
        File ig = EntrarInstagram.carpeta(this);
        String usuario = "";
        try {
            usuario = new org.json.JSONObject(leer(new File(ig, "cuenta.json"), 64 * 1024)).optString("usuario", "");
        } catch (Exception ignorado) {
        }
        boolean cuenta = new File(ig, "cuenta.json").exists();
        if (new File(ig, "sesion-nueva").exists()) {
            instagram.setText("Comprobando la sesión con Instagram…");
        } else if (cuenta) {
            instagram.setText(usuario.isEmpty() ? "La DS ya tiene tu Instagram" : "La DS tiene el Instagram de @" + usuario);
        } else {
            instagram.setText("Aún no has entrado: pulsa «Entrar en Instagram» y escribe tu usuario y tu contraseña en la página de Instagram.");
        }
        salirIG.setVisibility(cuenta ? View.VISIBLE : View.GONE);
        if (registro.getVisibility() == View.VISIBLE) {
            registro.setText(leer(Servicio.registro(this), 6000));
        }
        reloj.removeCallbacksAndMessages(null);
        reloj.postDelayed(this::refrescar, 2000);
    }

    // leer: el final de un archivo (como mucho max bytes).
    private static String leer(File f, int max) {
        try (RandomAccessFile r = new RandomAccessFile(f, "r")) {
            long n = r.length(), desde = Math.max(0, n - max);
            byte[] b = new byte[(int) (n - desde)];
            r.seek(desde);
            r.readFully(b);
            return new String(b, StandardCharsets.UTF_8);
        } catch (Exception e) {
            return "";
        }
    }

    private static List<String> direcciones() {
        List<String> r = new ArrayList<>();
        try {
            for (NetworkInterface i : Collections.list(NetworkInterface.getNetworkInterfaces())) {
                if (!i.isUp() || i.isLoopback()) {
                    continue;
                }
                for (InetAddress a : Collections.list(i.getInetAddresses())) {
                    if (a instanceof Inet4Address) {
                        r.add(a.getHostAddress());
                    }
                }
            }
        } catch (Exception ignorado) {
        }
        return r;
    }
}
