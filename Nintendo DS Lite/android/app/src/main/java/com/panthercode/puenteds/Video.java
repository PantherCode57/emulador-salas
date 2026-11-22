package com.panthercode.puenteds;

import android.graphics.Rect;
import android.media.Image;
import android.media.MediaCodec;
import android.media.MediaCodecInfo;
import android.media.MediaExtractor;
import android.media.MediaFormat;

import org.json.JSONObject;

import java.io.IOException;
import java.io.OutputStream;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.ShortBuffer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;
import java.util.Map;

// Un vídeo para la DS: lo descodifica Android (MediaExtractor y MediaCodec, con lo que traiga el
// móvil) y le pasa al puente las imágenes ya pequeñas (YUV 4:2:0) y el sonido en una sola voz a la
// frecuencia de la DS. El puente las hace JPEG y ADPCM (puente/video.go).
//
// Paquetes: [tipo][largo: 4 bytes][ms: 4 bytes][largo bytes], en little endian, en orden de tiempo:
// 'V' Y, Cb y Cr de ancho×alto; 'A' muestras de 16 bits; 'F' se acabó; 'E' un error (texto).
class Video {
    static class Fuente {
        String video, audio; // direcciones (o rutas de archivo); audio puede ser null o la misma
        Map<String, String> cabeceras;
        long dur; // ms, 0 si no se sabe
    }

    private static class Paquete {
        final byte tipo;
        final long ts;
        final byte[] datos;

        Paquete(byte tipo, long ts, byte[] datos) {
            this.tipo = tipo;
            this.ts = ts;
            this.datos = datos;
        }
    }

    // Cola: unos pocos paquetes de una pista, esperando a que les toque.
    private class Cola {
        private final ArrayDeque<Paquete> q = new ArrayDeque<>();
        private boolean fin;

        synchronized void poner(Paquete p) throws InterruptedException {
            while (q.size() >= 6 && !parar) {
                wait(200);
            }
            q.add(p);
            notifyAll();
        }

        synchronized void acabar() {
            fin = true;
            notifyAll();
        }

        // mirar: el primero (esperando a que haya), o null si se ha acabado.
        synchronized Paquete mirar() throws InterruptedException {
            while (q.isEmpty() && !fin && !parar) {
                wait(200);
            }
            return q.peek();
        }

        synchronized Paquete sacar() {
            Paquete p = q.poll();
            notifyAll();
            return p;
        }
    }

    private volatile boolean parar;
    private volatile String error;
    private final Cola colaV = new Cola(), colaA = new Cola();
    private final Fuente f;
    private final long desde;
    private final int wmax, hmax, area, tasa, fps;
    private int w, h;

    Video(Fuente f, long desde, int wmax, int hmax, int area, int tasa, int fps) {
        this.f = f;
        this.desde = desde;
        this.wmax = wmax;
        this.hmax = hmax;
        this.area = area;
        this.tasa = tasa;
        this.fps = Math.max(1, Math.min(30, fps));
    }

    private static MediaExtractor abrir(String fuente, Map<String, String> cab) throws IOException {
        MediaExtractor e = new MediaExtractor();
        if (fuente.startsWith("http")) {
            e.setDataSource(fuente, cab);
        } else {
            e.setDataSource(fuente);
        }
        return e;
    }

    private static int pista(MediaExtractor e, String tipo) {
        for (int i = 0; i < e.getTrackCount(); i++) {
            String mime = e.getTrackFormat(i).getString(MediaFormat.KEY_MIME);
            if (mime != null && mime.startsWith(tipo)) {
                return i;
            }
        }
        return -1;
    }

    // tamano: el de la DS (que quepa en wmax×hmax, sin pasar de area puntos y de 16 en 16).
    private void tamano(int vw, int vh) {
        double e = Math.min(Math.min((double) wmax / vw, (double) hmax / vh), Math.sqrt((double) area / ((double) vw * vh)));
        w = Math.max(16, (int) (vw * e) / 16 * 16);
        h = Math.max(16, (int) (vh * e) / 16 * 16);
    }

    // enviar: todo el vídeo por out (la primera línea, en JSON, con lo que es).
    void enviar(OutputStream out) throws Exception {
        MediaExtractor ev = abrir(f.video, f.cabeceras);
        int pv = pista(ev, "video/");
        if (pv < 0) {
            ev.release();
            throw new IOException("ese vídeo no tiene imagen");
        }
        MediaExtractor ea = null;
        int pa = -1;
        if (f.audio != null) {
            ea = abrir(f.audio, f.cabeceras);
            pa = pista(ea, "audio/");
            if (pa < 0) {
                ea.release();
                ea = null;
            }
        }
        MediaFormat fv = ev.getTrackFormat(pv);
        int vw = fv.getInteger(MediaFormat.KEY_WIDTH), vh = fv.getInteger(MediaFormat.KEY_HEIGHT);
        tamano(vw, vh);
        long dur = f.dur;
        if (dur <= 0 && fv.containsKey(MediaFormat.KEY_DURATION)) {
            dur = fv.getLong(MediaFormat.KEY_DURATION) / 1000;
        }
        JSONObject j = new JSONObject();
        j.put("dur", dur);
        j.put("w", w);
        j.put("h", h);
        j.put("sonido", ea != null);
        out.write((j.toString() + "\n").getBytes(StandardCharsets.UTF_8));
        out.flush();

        ev.selectTrack(pv);
        ev.seekTo(desde * 1000, MediaExtractor.SEEK_TO_PREVIOUS_SYNC);
        final MediaExtractor eA = ea;
        if (eA != null) {
            eA.selectTrack(pa);
            eA.seekTo(desde * 1000, MediaExtractor.SEEK_TO_PREVIOUS_SYNC);
        } else {
            colaA.acabar();
        }
        Thread tv = new Thread(() -> {
            try {
                imagenes(ev);
            } catch (Exception e) {
                if (!parar) {
                    error = "imagen: " + e;
                }
            } finally {
                ev.release();
                colaV.acabar();
            }
        }, "video");
        Thread ta = new Thread(() -> {
            try {
                if (eA != null) {
                    sonido(eA);
                }
            } catch (Exception e) {
                if (!parar) {
                    error = "sonido: " + e;
                }
            } finally {
                if (eA != null) {
                    eA.release();
                }
                colaA.acabar();
            }
        }, "audio");
        tv.start();
        ta.start();
        try {
            // en orden de tiempo, de las dos pistas
            while (error == null) {
                Paquete a = colaA.mirar(), v = colaV.mirar();
                if (a == null && v == null) {
                    break;
                }
                Paquete p = (v == null || (a != null && a.ts <= v.ts)) ? colaA.sacar() : colaV.sacar();
                if (p != null) {
                    escribir(out, p);
                }
            }
            if (error != null) {
                escribir(out, new Paquete((byte) 'E', 0, error.getBytes(StandardCharsets.UTF_8)));
            } else {
                escribir(out, new Paquete((byte) 'F', dur, new byte[0]));
            }
            out.flush();
        } finally {
            parar = true;
            synchronized (colaV) {
                colaV.notifyAll();
            }
            synchronized (colaA) {
                colaA.notifyAll();
            }
            tv.join(3000);
            ta.join(3000);
        }
    }

    private static void escribir(OutputStream out, Paquete p) throws IOException {
        byte[] c = new byte[9];
        c[0] = p.tipo;
        int n = p.datos.length;
        c[1] = (byte) n;
        c[2] = (byte) (n >> 8);
        c[3] = (byte) (n >> 16);
        c[4] = (byte) (n >> 24);
        long t = Math.max(0, p.ts);
        c[5] = (byte) t;
        c[6] = (byte) (t >> 8);
        c[7] = (byte) (t >> 16);
        c[8] = (byte) (t >> 24);
        out.write(c);
        out.write(p.datos);
    }

    // --- La imagen ---

    private void imagenes(MediaExtractor ev) throws Exception {
        MediaFormat fv = ev.getTrackFormat(ev.getSampleTrackIndex() >= 0 ? ev.getSampleTrackIndex() : 0);
        for (int i = 0; i < ev.getTrackCount(); i++) {
            String m = ev.getTrackFormat(i).getString(MediaFormat.KEY_MIME);
            if (m != null && m.startsWith("video/")) {
                fv = ev.getTrackFormat(i);
            }
        }
        String mime = fv.getString(MediaFormat.KEY_MIME);
        MediaCodec dec = MediaCodec.createDecoderByType(mime);
        fv.setInteger(MediaFormat.KEY_COLOR_FORMAT, MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Flexible);
        dec.configure(fv, null, null, 0);
        dec.start();
        MediaCodec.BufferInfo info = new MediaCodec.BufferInfo();
        boolean finEntrada = false;
        long siguiente = desde, paso = 1000 / fps;
        MediaFormat salida = null;
        try {
            while (!parar) {
                if (!finEntrada) {
                    int i = dec.dequeueInputBuffer(10000);
                    if (i >= 0) {
                        ByteBuffer b = dec.getInputBuffer(i);
                        int n = b == null ? -1 : ev.readSampleData(b, 0);
                        if (n < 0) {
                            dec.queueInputBuffer(i, 0, 0, 0, MediaCodec.BUFFER_FLAG_END_OF_STREAM);
                            finEntrada = true;
                        } else {
                            dec.queueInputBuffer(i, 0, n, ev.getSampleTime(), 0);
                            ev.advance();
                        }
                    }
                }
                int o = dec.dequeueOutputBuffer(info, 10000);
                if (o == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                    salida = dec.getOutputFormat();
                } else if (o >= 0) {
                    long ts = info.presentationTimeUs / 1000;
                    boolean fin = (info.flags & MediaCodec.BUFFER_FLAG_END_OF_STREAM) != 0;
                    if (info.size > 0 && ts >= desde && ts >= siguiente) {
                        byte[] yuv = null;
                        Image im = null;
                        try {
                            im = dec.getOutputImage(o);
                        } catch (Exception ignorado) {
                        }
                        if (im != null) {
                            yuv = deImagen(im);
                            im.close();
                        } else {
                            yuv = deBuffer(dec.getOutputBuffer(o), salida != null ? salida : dec.getOutputFormat(), info);
                        }
                        if (yuv != null) {
                            colaV.poner(new Paquete((byte) 'V', ts, yuv));
                            siguiente = Math.max(siguiente + paso, ts);
                        }
                    }
                    dec.releaseOutputBuffer(o, false);
                    if (fin) {
                        break;
                    }
                }
            }
        } finally {
            try {
                dec.stop();
            } catch (Exception ignorado) {
            }
            dec.release();
        }
    }

    // deImagen: la imagen del descodificador, a w×h (cada punto, la media de 4 de la original).
    private byte[] deImagen(Image im) {
        Rect r = im.getCropRect();
        int cw = r.width(), ch = r.height();
        Image.Plane[] pl = im.getPlanes();
        byte[] out = new byte[w * h + 2 * (w / 2) * (h / 2)];
        byte[][] datos = new byte[3][];
        int[] fila = new int[3], punto = new int[3];
        for (int k = 0; k < 3; k++) {
            ByteBuffer b = pl[k].getBuffer();
            datos[k] = new byte[b.remaining()];
            b.get(datos[k]);
            fila[k] = pl[k].getRowStride();
            punto[k] = pl[k].getPixelStride();
        }
        escalar(datos[0], r.top * fila[0] + r.left * punto[0], fila[0], punto[0], cw, ch, out, 0, w, h);
        for (int k = 1; k < 3; k++) {
            escalar(datos[k], (r.top / 2) * fila[k] + (r.left / 2) * punto[k], fila[k], punto[k], cw / 2, ch / 2, out,
                    w * h + (k - 1) * (w / 2) * (h / 2), w / 2, h / 2);
        }
        return out;
    }

    // deBuffer: lo mismo, si el móvil no da Image (los dos formatos de siempre: planos y NV12).
    private byte[] deBuffer(ByteBuffer b, MediaFormat fmt, MediaCodec.BufferInfo info) {
        if (b == null || fmt == null) {
            return null;
        }
        int vw = fmt.getInteger(MediaFormat.KEY_WIDTH), vh = fmt.getInteger(MediaFormat.KEY_HEIGHT);
        int stride = fmt.containsKey("stride") ? fmt.getInteger("stride") : vw;
        int alto = fmt.containsKey("slice-height") ? fmt.getInteger("slice-height") : vh;
        if (stride < vw) {
            stride = vw;
        }
        if (alto < vh) {
            alto = vh;
        }
        int color = fmt.containsKey(MediaFormat.KEY_COLOR_FORMAT) ? fmt.getInteger(MediaFormat.KEY_COLOR_FORMAT) : 0;
        byte[] d = new byte[info.size];
        b.position(info.offset);
        b.get(d, 0, Math.min(info.size, b.remaining()));
        byte[] out = new byte[w * h + 2 * (w / 2) * (h / 2)];
        escalar(d, 0, stride, 1, vw, vh, out, 0, w, h);
        int base = stride * alto, cb = w * h, cr = w * h + (w / 2) * (h / 2);
        if (color == MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Planar) {
            escalar(d, base, stride / 2, 1, vw / 2, vh / 2, out, cb, w / 2, h / 2);
            escalar(d, base + (stride / 2) * (alto / 2), stride / 2, 1, vw / 2, vh / 2, out, cr, w / 2, h / 2);
        } else { // NV12 y casi todos los demás: Cb y Cr juntos
            escalar(d, base, stride, 2, vw / 2, vh / 2, out, cb, w / 2, h / 2);
            escalar(d, base + 1, stride, 2, vw / 2, vh / 2, out, cr, w / 2, h / 2);
        }
        return out;
    }

    // escalar: un plano de sw×sh (empieza en inicio; fila y punto son los pasos) a dw×dh, haciendo
    // la media de 2×2.
    private static void escalar(byte[] s, int inicio, int fila, int punto, int sw, int sh, byte[] o, int base, int dw, int dh) {
        if (sw <= 0 || sh <= 0) {
            return;
        }
        int n = s.length;
        for (int y = 0; y < dh; y++) {
            int sy = y * sh / dh, sy2 = Math.min(sh - 1, sy + 1);
            int f1 = inicio + sy * fila, f2 = inicio + sy2 * fila;
            for (int x = 0; x < dw; x++) {
                int sx = x * sw / dw, sx2 = Math.min(sw - 1, sx + 1);
                int a = f1 + sx * punto, b = f1 + sx2 * punto, c = f2 + sx * punto, d = f2 + sx2 * punto;
                if (d >= n || a < 0) {
                    o[base + y * dw + x] = (byte) 128;
                    continue;
                }
                o[base + y * dw + x] = (byte) (((s[a] & 255) + (s[b] & 255) + (s[c] & 255) + (s[d] & 255) + 2) >> 2);
            }
        }
    }

    // --- El sonido: una sola voz, a la frecuencia de la DS (cada muestra, la media de las que caen) ---

    private void sonido(MediaExtractor ea) throws Exception {
        MediaFormat fa = null;
        for (int i = 0; i < ea.getTrackCount(); i++) {
            String m = ea.getTrackFormat(i).getString(MediaFormat.KEY_MIME);
            if (m != null && m.startsWith("audio/")) {
                fa = ea.getTrackFormat(i);
            }
        }
        if (fa == null) {
            return;
        }
        MediaCodec dec = MediaCodec.createDecoderByType(fa.getString(MediaFormat.KEY_MIME));
        dec.configure(fa, null, null, 0);
        dec.start();
        MediaCodec.BufferInfo info = new MediaCodec.BufferInfo();
        int canales = fa.containsKey(MediaFormat.KEY_CHANNEL_COUNT) ? fa.getInteger(MediaFormat.KEY_CHANNEL_COUNT) : 2;
        int frecuencia = fa.containsKey(MediaFormat.KEY_SAMPLE_RATE) ? fa.getInteger(MediaFormat.KEY_SAMPLE_RATE) : 44100;
        boolean finEntrada = false;
        double pos = 0;
        long suma = 0;
        int nsuma = 0, ultimo = 0;
        short[] bloque = new short[tasa / 10];
        int nb = 0;
        long tsBloque = -1, salidas = 0, tsBase = -1;
        try {
            while (!parar) {
                if (!finEntrada) {
                    int i = dec.dequeueInputBuffer(10000);
                    if (i >= 0) {
                        ByteBuffer b = dec.getInputBuffer(i);
                        int n = b == null ? -1 : ea.readSampleData(b, 0);
                        if (n < 0) {
                            dec.queueInputBuffer(i, 0, 0, 0, MediaCodec.BUFFER_FLAG_END_OF_STREAM);
                            finEntrada = true;
                        } else {
                            dec.queueInputBuffer(i, 0, n, ea.getSampleTime(), 0);
                            ea.advance();
                        }
                    }
                }
                int o = dec.dequeueOutputBuffer(info, 10000);
                if (o == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                    MediaFormat s = dec.getOutputFormat();
                    canales = s.getInteger(MediaFormat.KEY_CHANNEL_COUNT);
                    frecuencia = s.getInteger(MediaFormat.KEY_SAMPLE_RATE);
                } else if (o >= 0) {
                    boolean fin = (info.flags & MediaCodec.BUFFER_FLAG_END_OF_STREAM) != 0;
                    ByteBuffer b = dec.getOutputBuffer(o);
                    if (b != null && info.size > 0) {
                        b.position(info.offset);
                        b.limit(info.offset + info.size);
                        ShortBuffer sb = b.order(ByteOrder.LITTLE_ENDIAN).asShortBuffer();
                        int muestras = sb.remaining() / Math.max(1, canales);
                        long ts = info.presentationTimeUs / 1000;
                        // lo de antes de «desde» (desde el principio del trozo) no se manda
                        int saltar = ts < desde ? (int) Math.min(muestras, (desde - ts) * frecuencia / 1000) : 0;
                        if (tsBase < 0 && muestras > saltar) {
                            tsBase = Math.max(ts, desde);
                        }
                        double paso = (double) frecuencia / tasa;
                        for (int k = saltar; k < muestras; k++) {
                            int v = 0;
                            for (int c = 0; c < canales; c++) {
                                v += sb.get(k * canales + c);
                            }
                            v /= Math.max(1, canales);
                            suma += v;
                            nsuma++;
                            ultimo = v;
                            pos += 1;
                            while (pos >= paso) {
                                pos -= paso;
                                int m = nsuma > 0 ? (int) (suma / nsuma) : ultimo;
                                suma = 0;
                                nsuma = 0;
                                if (nb == 0) {
                                    tsBloque = tsBase + salidas * 1000 / tasa;
                                }
                                bloque[nb++] = (short) m;
                                salidas++;
                                if (nb == bloque.length) {
                                    colaA.poner(new Paquete((byte) 'A', tsBloque, pcm(bloque, nb)));
                                    nb = 0;
                                }
                            }
                        }
                    }
                    dec.releaseOutputBuffer(o, false);
                    if (fin) {
                        break;
                    }
                }
            }
            if (nb > 0 && !parar) {
                colaA.poner(new Paquete((byte) 'A', tsBloque, pcm(bloque, nb)));
            }
        } finally {
            try {
                dec.stop();
            } catch (Exception ignorado) {
            }
            dec.release();
        }
    }

    private static byte[] pcm(short[] m, int n) {
        byte[] b = new byte[n * 2];
        for (int i = 0; i < n; i++) {
            b[i * 2] = (byte) m[i];
            b[i * 2 + 1] = (byte) (m[i] >> 8);
        }
        return b;
    }
}
