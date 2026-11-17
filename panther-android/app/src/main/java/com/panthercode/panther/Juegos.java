package com.panthercode.panther;

import android.content.Context;
import android.content.pm.PackageInfo;
import android.database.Cursor;
import android.graphics.Typeface;
import android.net.Uri;
import android.provider.OpenableColumns;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.RandomAccessFile;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

/** Los juegos de la app: dónde están, cómo se traen del móvil o de Drive, y sus partidas. */
final class Juegos {
    private Juegos() {}

    static final List<String> EXTENSIONES = Arrays.asList("gba", "gbc", "gb", "7z");

    /** Android/data/com.panthercode.panther/files/juegos: los juegos y, junto a cada uno, su partida (.srm). */
    static File carpeta(Context c) {
        File base = c.getExternalFilesDir(null);
        if (base == null) base = c.getFilesDir();
        File d = new File(base, "juegos");
        d.mkdirs();
        return d;
    }

    static List<File> lista(Context c) {
        List<File> r = new ArrayList<>();
        File[] todos = carpeta(c).listFiles();
        if (todos != null)
            for (File f : todos)
                if (f.isFile() && EXTENSIONES.contains(extension(f.getName()))) r.add(f);
        r.sort((a, b) -> bonito(a.getName()).compareToIgnoreCase(bonito(b.getName())));
        return r;
    }

    static String extension(String n) {
        int p = n.lastIndexOf('.');
        return p < 0 ? "" : n.substring(p + 1).toLowerCase(Locale.ROOT);
    }

    static String base(String n) {
        int p = n.lastIndexOf('.');
        return p <= 0 ? n : n.substring(0, p);
    }

    /** «Pokemon - FireRed Version (USA, Europe).gba» → «Pokemon - FireRed Version». */
    static String bonito(String n) {
        String b = base(n).replace('_', ' ').trim();
        int p = b.indexOf(" (");
        if (p > 0) b = b.substring(0, p);
        p = b.indexOf(" [");
        if (p > 0) b = b.substring(0, p);
        return b.trim().isEmpty() ? n : b.trim();
    }

    static String sistema(File f) {
        switch (extension(f.getName())) {
            case "gba": return "GBA";
            case "gbc": return "GBC";
            case "gb": return "GB";
            default: return "7Z";
        }
    }

    static File partida(File juego) { return new File(juego.getParentFile(), base(juego.getName()) + ".srm"); }
    static File dondeIbas(File juego) { return new File(juego.getParentFile(), base(juego.getName()) + ".estado-auto"); }

    /** Pokémon Rojo Fuego o Verde Hoja de EE. UU. (BPRE, BPGE): los que tienen Caramelos Raros. */
    static boolean tieneCaramelos(File juego) {
        if (!"gba".equals(extension(juego.getName()))) return false;
        try (RandomAccessFile r = new RandomAccessFile(juego, "r")) {
            byte[] codigo = new byte[4];
            r.seek(0xAC);
            r.readFully(codigo);
            String s = new String(codigo, "US-ASCII");
            return s.equals("BPRE") || s.equals("BPGE");
        } catch (IOException e) {
            return false;
        }
    }

    static String nombre(Context c, Uri u) {
        String n = null;
        try (Cursor k = c.getContentResolver().query(u, new String[]{OpenableColumns.DISPLAY_NAME}, null, null, null)) {
            if (k != null && k.moveToFirst()) n = k.getString(0);
        } catch (RuntimeException ignorado) {
        }
        if (n == null) n = u.getLastPathSegment();
        if (n == null) n = "juego";
        int barra = n.lastIndexOf('/');
        if (barra >= 0) n = n.substring(barra + 1);
        return n.replaceAll("[\\\\/:*?\"<>|]", "_");
    }

    /** Lo que se ha traído de un archivo. */
    static final class Traido {
        int juegos, partidas;
        String problema;
    }

    /**
     * Trae un archivo elegido en el selector (del móvil o de Google Drive, que lo descarga al abrirlo):
     * un juego (.gba, .gbc, .gb, .7z), un .zip con juegos dentro, o una partida (.sav, .srm) de un juego
     * que ya está, que se pone como su partida.
     */
    static Traido traer(Context c, Uri u) {
        Traido t = new Traido();
        String n = nombre(c, u);
        String ext = extension(n);
        File dir = carpeta(c);
        try (InputStream in = c.getContentResolver().openInputStream(u)) {
            if (in == null) throw new IOException("no se puede abrir");
            if (EXTENSIONES.contains(ext)) {
                copiar(in, new File(dir, n));
                t.juegos++;
            } else if (ext.equals("zip")) {
                ZipInputStream z = new ZipInputStream(in);
                for (ZipEntry e; (e = z.getNextEntry()) != null; ) {
                    String dentro = new File(e.getName()).getName().replaceAll("[\\\\/:*?\"<>|]", "_");
                    String x = extension(dentro);
                    if (e.isDirectory() || !(x.equals("gba") || x.equals("gbc") || x.equals("gb"))) continue;
                    copiar(z, new File(dir, dentro));
                    t.juegos++;
                }
                if (t.juegos == 0) t.problema = n + ": no tiene juegos de GB, GBC ni GBA";
            } else if (ext.equals("sav") || ext.equals("srm")) {
                File juego = juegoLlamado(c, base(n));
                if (juego == null) {
                    t.problema = n + ": no hay ningún juego que se llame igual";
                } else {
                    ponerPartida(in, juego);
                    t.partidas++;
                }
            } else {
                t.problema = n + ": no es un juego de GB, GBC ni GBA";
            }
        } catch (IOException | SecurityException e) {
            t.problema = n + ": " + e.getMessage();
        }
        return t;
    }

    static File juegoLlamado(Context c, String base) {
        for (File f : lista(c))
            if (base(f.getName()).equalsIgnoreCase(base)) return f;
        return null;
    }

    /** Pone una partida (.sav de otro emulador, de linkcable…) como la del juego, guardando la de antes. */
    static void ponerPartida(InputStream in, File juego) throws IOException {
        File srm = partida(juego);
        File nueva = new File(srm.getPath() + ".nueva");
        copiar(in, nueva);
        if (srm.exists()) {
            File antes = new File(srm.getPath() + ".antes");
            antes.delete();
            srm.renameTo(antes);
        }
        if (!nueva.renameTo(srm)) throw new IOException("no se ha podido guardar");
        // Si no, al abrirlo seguirías donde lo dejaste con la partida de antes.
        dondeIbas(juego).delete();
    }

    static void copiar(InputStream in, File destino) throws IOException {
        File tmp = new File(destino.getPath() + ".tmp");
        try (OutputStream out = new FileOutputStream(tmp)) {
            byte[] b = new byte[1 << 16];
            for (int n; (n = in.read(b)) > 0; ) out.write(b, 0, n);
        }
        destino.delete();
        if (!tmp.renameTo(destino)) throw new IOException("no se ha podido guardar " + destino.getName());
    }

    static void copiar(File origen, OutputStream out) throws IOException {
        try (InputStream in = new FileInputStream(origen)) {
            byte[] b = new byte[1 << 16];
            for (int n; (n = in.read(b)) > 0; ) out.write(b, 0, n);
        }
    }

    /** Las letras del menú (Selawik) y el script de los Caramelos Raros, del APK a los archivos de la app. */
    static void prepararArchivos(Context c) {
        long instalada = 0;
        try {
            PackageInfo p = c.getPackageManager().getPackageInfo(c.getPackageName(), 0);
            instalada = p.lastUpdateTime;
        } catch (Exception ignorado) {
        }
        for (String n : new String[]{"selawk.ttf", "selawksb.ttf", "caramelos-frlg.lua"}) {
            File f = new File(c.getFilesDir(), n);
            if (f.exists() && f.lastModified() >= instalada) continue;
            try (InputStream in = c.getAssets().open(n)) {
                copiar(in, f);
            } catch (IOException ignorado) {
            }
        }
    }

    private static Typeface normal, negrita;

    static Typeface letra(Context c, boolean gruesa) {
        try {
            if (normal == null) normal = Typeface.createFromAsset(c.getAssets(), "selawk.ttf");
            if (negrita == null) negrita = Typeface.createFromAsset(c.getAssets(), "selawksb.ttf");
            return gruesa ? negrita : normal;
        } catch (RuntimeException e) {
            return gruesa ? Typeface.DEFAULT_BOLD : Typeface.DEFAULT;
        }
    }
}
