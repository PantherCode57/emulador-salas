package com.panthercode.panther;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.res.ColorStateList;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.RippleDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.text.TextUtils;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.io.File;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.List;

/** La pantalla de inicio: tus juegos, y traer más del móvil o de Google Drive. */
public class Inicio extends Activity {
    private static final int PEDIR_JUEGOS = 1, PEDIR_PARTIDA = 2, GUARDAR_PARTIDA = 3;
    static final int FONDO = 0xFF0E0B16, TARJETA = 0xFF1A1528, VIOLETA = 0xFF7C4DFF;
    static final int BLANCO = 0xFFF5F3FA, SUAVE = 0xFFA49EBA, APAGADO = 0xFF6E6886, VIOLETA_CLARO = 0xFFA78BFA;

    private Typeface letra, negrita;
    private LinearLayout lista;
    private TextView aviso;
    private SharedPreferences ajustes;
    private File elegido; // el juego al que se le trae o se le saca la partida

    @Override
    protected void onCreate(Bundle guardado) {
        super.onCreate(guardado);
        Juegos.prepararArchivos(this);
        ajustes = getSharedPreferences("panther", MODE_PRIVATE);
        letra = Juegos.letra(this, false);
        negrita = Juegos.letra(this, true);

        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(FONDO);
        scroll.setFillViewport(true);
        LinearLayout raiz = new LinearLayout(this);
        raiz.setOrientation(LinearLayout.VERTICAL);
        int m = dp(20);
        raiz.setPadding(m, m, m, m);
        scroll.addView(raiz);

        raiz.addView(texto("Panther", 36, BLANCO, negrita));
        TextView sub = texto("Game Boy · Game Boy Color · Game Boy Advance", 15, SUAVE, letra);
        raiz.addView(sub, margenes(0, dp(2), 0, dp(22)));

        TextView anadir = texto("+  Añadir juegos", 18, BLANCO, negrita);
        anadir.setGravity(Gravity.CENTER);
        anadir.setPadding(dp(16), dp(16), dp(16), dp(16));
        anadir.setBackground(pulsable(VIOLETA, dp(16)));
        anadir.setOnClickListener(v -> pedirJuegos());
        raiz.addView(anadir);
        TextView ayuda = texto("Del móvil o de tu Google Drive: .gba, .gbc, .gb o .zip. Si eliges también "
                + "su partida (.sav), se le pone sola.", 13, SUAVE, letra);
        ayuda.setGravity(Gravity.CENTER);
        raiz.addView(ayuda, margenes(dp(8), dp(10), dp(8), dp(8)));

        aviso = texto("", 14, VIOLETA_CLARO, negrita);
        aviso.setGravity(Gravity.CENTER);
        aviso.setVisibility(View.GONE);
        raiz.addView(aviso, margenes(0, dp(6), 0, dp(6)));

        TextView titulo = texto("Tus juegos", 20, BLANCO, negrita);
        raiz.addView(titulo, margenes(dp(2), dp(20), 0, dp(10)));
        lista = new LinearLayout(this);
        lista.setOrientation(LinearLayout.VERTICAL);
        raiz.addView(lista);

        TextView pie = texto("En el juego: «Menú» (o la tecla atrás) para guardar, cargar, la velocidad y salir. "
                + "«Rápido» va a ×3. Al salir se guarda dónde ibas.", 13, APAGADO, letra);
        raiz.addView(pie, margenes(dp(4), dp(24), dp(4), 0));

        setContentView(scroll);
        scroll.setOnApplyWindowInsetsListener((v, ins) -> {
            raiz.setPadding(m, m + ins.getSystemWindowInsetTop(), m, m + ins.getSystemWindowInsetBottom());
            return ins;
        });
    }


    @Override
    protected void onResume() {
        super.onResume();
        refrescar();
    }

    private void refrescar() {
        lista.removeAllViews();
        List<File> juegos = Juegos.lista(this);
        if (juegos.isEmpty()) {
            TextView v = texto("Aún no hay juegos. Pulsa «Añadir juegos» y elígelos en el móvil o en Drive.",
                    15, SUAVE, letra);
            v.setPadding(dp(18), dp(22), dp(18), dp(22));
            v.setGravity(Gravity.CENTER);
            v.setBackground(redondo(TARJETA, dp(16)));
            lista.addView(v);
            return;
        }
        for (File f : juegos) lista.addView(tarjeta(f), margenes(0, 0, 0, dp(10)));
    }

    private View tarjeta(File f) {
        LinearLayout t = new LinearLayout(this);
        t.setOrientation(LinearLayout.HORIZONTAL);
        t.setGravity(Gravity.CENTER_VERTICAL);
        t.setPadding(dp(14), dp(14), dp(6), dp(14));
        t.setBackground(pulsable(TARJETA, dp(16)));

        String sis = Juegos.sistema(f);
        TextView chip = texto(sis, 13, BLANCO, negrita);
        chip.setGravity(Gravity.CENTER);
        chip.setMinWidth(dp(50));
        chip.setPadding(dp(6), dp(6), dp(6), dp(6));
        int color = sis.equals("GBA") ? VIOLETA : sis.equals("GBC") ? 0xFF2E9E8F : sis.equals("GB") ? 0xFF6F7F3A : 0xFF55506A;
        chip.setBackground(redondo(color, dp(10)));
        t.addView(chip);

        LinearLayout textos = new LinearLayout(this);
        textos.setOrientation(LinearLayout.VERTICAL);
        TextView nombre = texto(Juegos.bonito(f.getName()), 17, BLANCO, negrita);
        nombre.setSingleLine(true);
        nombre.setEllipsize(TextUtils.TruncateAt.END);
        textos.addView(nombre);
        String estado = Juegos.dondeIbas(f).exists() ? "Sigues donde lo dejaste"
                : Juegos.partida(f).exists() ? "Con partida guardada" : "Sin empezar";
        if (caramelos(f)) estado += " · Caramelos Raros";
        textos.addView(texto(estado, 13, SUAVE, letra));
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        lp.setMargins(dp(14), 0, dp(4), 0);
        t.addView(textos, lp);

        TextView mas = texto("⋯", 26, SUAVE, negrita);
        mas.setGravity(Gravity.CENTER);
        mas.setPadding(dp(12), dp(2), dp(12), dp(6));
        mas.setBackground(pulsable(TARJETA, dp(12)));
        mas.setOnClickListener(v -> opciones(f));
        t.addView(mas);

        t.setOnClickListener(v -> jugar(f));
        t.setOnLongClickListener(v -> {
            opciones(f);
            return true;
        });
        return t;
    }

    private boolean caramelos(File f) { return ajustes.getBoolean("caramelos:" + f.getName(), false); }

    private void jugar(File f) {
        Intent i = new Intent(this, Juego.class);
        i.putExtra(Juego.JUEGO, f.getAbsolutePath());
        i.putExtra(Juego.CARAMELOS, caramelos(f) && Juegos.tieneCaramelos(f));
        startActivity(i);
    }

    private void opciones(File f) {
        List<String> nombres = new ArrayList<>();
        List<Runnable> que = new ArrayList<>();
        nombres.add("Jugar");
        que.add(() -> jugar(f));
        nombres.add("Traer una partida (.sav)");
        que.add(() -> {
            elegido = f;
            Intent i = new Intent(Intent.ACTION_OPEN_DOCUMENT);
            i.addCategory(Intent.CATEGORY_OPENABLE);
            i.setType("*/*");
            startActivityForResult(i, PEDIR_PARTIDA);
        });
        if (Juegos.partida(f).exists()) {
            nombres.add("Guardar la partida en el móvil o en Drive");
            que.add(() -> {
                elegido = f;
                Intent i = new Intent(Intent.ACTION_CREATE_DOCUMENT);
                i.addCategory(Intent.CATEGORY_OPENABLE);
                i.setType("application/octet-stream");
                i.putExtra(Intent.EXTRA_TITLE, Juegos.base(f.getName()) + ".sav");
                startActivityForResult(i, GUARDAR_PARTIDA);
            });
        }
        if (Juegos.tieneCaramelos(f)) {
            boolean si = caramelos(f);
            nombres.add(si ? "Quitar los Caramelos Raros infinitos" : "Caramelos Raros infinitos");
            que.add(() -> {
                ajustes.edit().putBoolean("caramelos:" + f.getName(), !si).apply();
                mostrar(si ? "Caramelos Raros quitados" : "Caramelos Raros: tendrás 999 en la mochila");
                refrescar();
            });
        }
        nombres.add("Quitar el juego");
        que.add(() -> quitar(f));
        new AlertDialog.Builder(this, android.R.style.Theme_Material_Dialog_Alert)
                .setTitle(Juegos.bonito(f.getName()))
                .setItems(nombres.toArray(new String[0]), (d, i) -> que.get(i).run())
                .show();
    }

    private void quitar(File f) {
        new AlertDialog.Builder(this, android.R.style.Theme_Material_Dialog_Alert)
                .setTitle("¿Quitar «" + Juegos.bonito(f.getName()) + "»?")
                .setMessage("Se borra el juego de la app. Su partida se queda guardada, por si lo vuelves a añadir.")
                .setPositiveButton("Quitar", (d, i) -> {
                    f.delete();
                    refrescar();
                })
                .setNegativeButton("Cancelar", null)
                .show();
    }

    private void pedirJuegos() {
        Intent i = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        i.addCategory(Intent.CATEGORY_OPENABLE);
        i.setType("*/*");
        i.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        startActivityForResult(i, PEDIR_JUEGOS);
    }

    @Override
    protected void onActivityResult(int pedido, int resultado, Intent datos) {
        super.onActivityResult(pedido, resultado, datos);
        if (resultado != RESULT_OK || datos == null) return;
        if (pedido == PEDIR_JUEGOS) {
            List<Uri> uris = new ArrayList<>();
            ClipData varios = datos.getClipData();
            if (varios != null)
                for (int i = 0; i < varios.getItemCount(); i++) uris.add(varios.getItemAt(i).getUri());
            else if (datos.getData() != null) uris.add(datos.getData());
            traer(uris);
        } else if (pedido == PEDIR_PARTIDA && elegido != null && datos.getData() != null) {
            File juego = elegido;
            Uri u = datos.getData();
            mostrar("Trayendo la partida…");
            new Thread(() -> {
                String r;
                try (InputStream in = getContentResolver().openInputStream(u)) {
                    if (in == null) throw new java.io.IOException("no se puede abrir");
                    Juegos.ponerPartida(in, juego);
                    r = "Partida puesta en " + Juegos.bonito(juego.getName());
                } catch (Exception e) {
                    r = "No se ha podido traer la partida: " + e.getMessage();
                }
                String fin = r;
                runOnUiThread(() -> {
                    mostrar(fin);
                    refrescar();
                });
            }).start();
        } else if (pedido == GUARDAR_PARTIDA && elegido != null && datos.getData() != null) {
            try (OutputStream out = getContentResolver().openOutputStream(datos.getData())) {
                if (out == null) throw new java.io.IOException("no se puede escribir");
                Juegos.copiar(Juegos.partida(elegido), out);
                mostrar("Partida guardada");
            } catch (Exception e) {
                mostrar("No se ha podido guardar la partida: " + e.getMessage());
            }
        }
    }

    /** Copia lo elegido (desde Drive, se descarga en este momento) sin parar la pantalla. */
    private void traer(List<Uri> uris) {
        if (uris.isEmpty()) return;
        mostrar(uris.size() == 1 ? "Trayendo el juego…" : "Trayendo " + uris.size() + " archivos…");
        new Thread(() -> {
            int juegos = 0, partidas = 0;
            List<String> problemas = new ArrayList<>();
            for (Uri u : uris) {
                Juegos.Traido t = Juegos.traer(this, u);
                juegos += t.juegos;
                partidas += t.partidas;
                if (t.problema != null) problemas.add(t.problema);
            }
            StringBuilder s = new StringBuilder();
            if (juegos > 0) s.append(juegos == 1 ? "1 juego añadido" : juegos + " juegos añadidos");
            if (partidas > 0) s.append(s.length() > 0 ? " y " : "").append(partidas == 1 ? "1 partida" : partidas + " partidas");
            String resumen = s.length() > 0 ? s.toString() : "No se ha añadido nada";
            runOnUiThread(() -> {
                mostrar(resumen);
                refrescar();
                if (!problemas.isEmpty())
                    new AlertDialog.Builder(this, android.R.style.Theme_Material_Dialog_Alert)
                            .setTitle("Algunos no se han podido añadir")
                            .setMessage(TextUtils.join("\n", problemas))
                            .setPositiveButton("Vale", null)
                            .show();
            });
        }).start();
    }

    private void mostrar(String s) {
        aviso.setText(s);
        aviso.setVisibility(View.VISIBLE);
        Toast.makeText(this, s, Toast.LENGTH_SHORT).show();
    }

    // ---- Piezas de la pantalla ----

    private TextView texto(String s, int sp, int color, Typeface t) {
        TextView v = new TextView(this);
        v.setText(s);
        v.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        v.setTextColor(color);
        v.setTypeface(t);
        return v;
    }

    private LinearLayout.LayoutParams margenes(int l, int t, int r, int b) {
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT);
        p.setMargins(l, t, r, b);
        return p;
    }

    private static GradientDrawable redondo(int color, float radio) {
        GradientDrawable g = new GradientDrawable();
        g.setColor(color);
        g.setCornerRadius(radio);
        return g;
    }

    private static RippleDrawable pulsable(int color, float radio) {
        return new RippleDrawable(ColorStateList.valueOf(0x40FFFFFF), redondo(color, radio), redondo(0xFFFFFFFF, radio));
    }

    private int dp(int v) { return Math.round(v * getResources().getDisplayMetrics().density); }
}
