package com.panthercode.puenteds;

import android.content.Context;

import org.json.JSONArray;
import org.json.JSONObject;
import org.schabi.newpipe.extractor.InfoItem;
import org.schabi.newpipe.extractor.ListExtractor;
import org.schabi.newpipe.extractor.MediaFormat;
import org.schabi.newpipe.extractor.NewPipe;
import org.schabi.newpipe.extractor.Page;
import org.schabi.newpipe.extractor.ServiceList;
import org.schabi.newpipe.extractor.localization.ContentCountry;
import org.schabi.newpipe.extractor.localization.Localization;
import org.schabi.newpipe.extractor.search.SearchExtractor;
import org.schabi.newpipe.extractor.services.youtube.linkHandler.YoutubeSearchQueryHandlerFactory;
import org.schabi.newpipe.extractor.services.youtube.linkHandler.YoutubeStreamLinkHandlerFactory;
import org.schabi.newpipe.extractor.stream.AudioStream;
import org.schabi.newpipe.extractor.stream.DeliveryMethod;
import org.schabi.newpipe.extractor.stream.StreamInfo;
import org.schabi.newpipe.extractor.stream.StreamInfoItem;
import org.schabi.newpipe.extractor.stream.StreamType;
import org.schabi.newpipe.extractor.stream.VideoStream;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

// El ayudante del puente: lo que el puente de Go no puede hacer en Android. Busca en YouTube y saca
// sus vídeos con NewPipe Extractor (la librería de la app NewPipe), y descodifica los vídeos con lo
// que trae el móvil (Video.java). Solo escucha en el propio móvil (127.0.0.1) y pide una clave.
//
// Cada petición es una conexión: «<clave>\n<petición>\n» y la respuesta.
//   BUSCAR\t<texto>   una línea de JSON: {"videos": [{url, id, titulo, canal, seg, vistas}], "mas": …}
//   MAS               lo mismo, con lo siguiente de la última búsqueda
//   VER\t<fuente>\t<desde ms>\t<ancho>\t<alto>\t<puntos>\t<frecuencia>\t<imágenes por segundo>
//                     el vídeo (ver Video.java). <fuente>: un vídeo de YouTube, la dirección de un
//                     vídeo (los de Instagram) o, en las pruebas, «archivo:prueba».
class Ayudante {
    final int puerto;
    final String clave;
    private final ServerSocket ss;
    private final Context ctx;
    private final boolean pruebas;
    private final Servicio.Apunte apunte;

    private SearchExtractor ultima;
    private Page siguiente;
    private final Map<String, Video.Fuente> fuentes = new HashMap<>(); // las de YouTube, un rato
    private final Map<String, Long> fuentesEn = new HashMap<>();

    Ayudante(Context ctx, boolean pruebas, Servicio.Apunte apunte) throws IOException {
        this.ctx = ctx;
        this.pruebas = pruebas;
        this.apunte = apunte;
        byte[] b = new byte[16];
        new SecureRandom().nextBytes(b);
        StringBuilder s = new StringBuilder();
        for (byte x : b) {
            s.append(String.format("%02x", x & 255));
        }
        clave = s.toString();
        ss = new ServerSocket(0, 16, InetAddress.getByName("127.0.0.1"));
        puerto = ss.getLocalPort();
        NewPipe.init(new Descargador(), new Localization("es", "ES"), new ContentCountry("ES"));
        Thread t = new Thread(this::escuchar, "ayudante");
        t.setDaemon(true);
        t.start();
    }

    void cerrar() {
        try {
            ss.close();
        } catch (IOException ignorado) {
        }
    }

    private void escuchar() {
        while (!ss.isClosed()) {
            try {
                Socket s = ss.accept();
                Thread t = new Thread(() -> atender(s), "ayudante-peticion");
                t.setDaemon(true);
                t.start();
            } catch (IOException e) {
                if (!ss.isClosed()) {
                    apunte.apuntar("ayudante: " + e);
                }
            }
        }
    }

    private static String linea(InputStream in) throws IOException {
        ByteArrayOutputStream b = new ByteArrayOutputStream();
        int c;
        while ((c = in.read()) >= 0 && c != '\n') {
            if (b.size() > 8192) {
                throw new IOException("línea demasiado larga");
            }
            b.write(c);
        }
        if (c < 0 && b.size() == 0) {
            throw new IOException("cerrado");
        }
        return new String(b.toByteArray(), StandardCharsets.UTF_8);
    }

    private void atender(Socket s) {
        try (Socket sock = s) {
            sock.setSoTimeout(60000);
            InputStream in = new BufferedInputStream(sock.getInputStream());
            OutputStream out = new BufferedOutputStream(sock.getOutputStream(), 128 * 1024);
            if (!clave.equals(linea(in))) {
                return;
            }
            String[] p = linea(in).split("\t", -1);
            switch (p[0]) {
                case "BUSCAR":
                case "MAS": {
                    JSONObject r;
                    try {
                        r = "MAS".equals(p[0]) ? mas() : buscar(p.length > 1 ? p[1] : "");
                    } catch (Exception e) {
                        apunte.apuntar("YouTube: " + e);
                        r = new JSONObject();
                        r.put("error", "YouTube no contesta: " + corto(e));
                    }
                    out.write((r.toString() + "\n").getBytes(StandardCharsets.UTF_8));
                    out.flush();
                    break;
                }
                case "VER": {
                    if (p.length < 8) {
                        return;
                    }
                    Video.Fuente f;
                    try {
                        f = fuente(p[1]);
                    } catch (Exception e) {
                        apunte.apuntar("vídeo: " + e);
                        JSONObject r = new JSONObject();
                        r.put("error", "No se puede abrir el vídeo: " + corto(e));
                        out.write((r.toString() + "\n").getBytes(StandardCharsets.UTF_8));
                        out.flush();
                        return;
                    }
                    Video v = new Video(f, Long.parseLong(p[2]), Integer.parseInt(p[3]), Integer.parseInt(p[4]),
                            Integer.parseInt(p[5]), Integer.parseInt(p[6]), Integer.parseInt(p[7]));
                    try {
                        v.enviar(out);
                    } catch (Exception e) {
                        apunte.apuntar("vídeo: " + e);
                    }
                    break;
                }
                default:
                    break;
            }
        } catch (Exception e) {
            apunte.apuntar("ayudante: " + e);
        }
    }

    private static String corto(Exception e) {
        String m = e.getMessage();
        if (m == null || m.isEmpty()) {
            m = e.getClass().getSimpleName();
        }
        return m.length() > 120 ? m.substring(0, 120) : m;
    }

    // --- Buscar ---

    private synchronized JSONObject buscar(String q) throws Exception {
        SearchExtractor se = ServiceList.YouTube.getSearchExtractor(q,
                Collections.singletonList(YoutubeSearchQueryHandlerFactory.VIDEOS), "");
        se.fetchPage();
        ListExtractor.InfoItemsPage<InfoItem> pg = se.getInitialPage();
        ultima = se;
        siguiente = pg.hasNextPage() ? pg.getNextPage() : null;
        return pagina(pg.getItems(), siguiente != null);
    }

    private synchronized JSONObject mas() throws Exception {
        if (ultima == null || siguiente == null) {
            return pagina(Collections.emptyList(), false);
        }
        ListExtractor.InfoItemsPage<InfoItem> pg = ultima.getPage(siguiente);
        siguiente = pg.hasNextPage() ? pg.getNextPage() : null;
        return pagina(pg.getItems(), siguiente != null);
    }

    private static JSONObject pagina(List<InfoItem> items, boolean hayMas) throws Exception {
        JSONArray l = new JSONArray();
        for (InfoItem it : items) {
            if (!(it instanceof StreamInfoItem)) {
                continue;
            }
            StreamInfoItem s = (StreamInfoItem) it;
            if (s.getStreamType() == StreamType.LIVE_STREAM || s.getStreamType() == StreamType.AUDIO_LIVE_STREAM) {
                continue; // los directos no se pueden pasar a la DS
            }
            JSONObject v = new JSONObject();
            v.put("url", s.getUrl());
            try {
                v.put("id", YoutubeStreamLinkHandlerFactory.getInstance().getId(s.getUrl()));
            } catch (Exception ignorado) {
            }
            v.put("titulo", s.getName());
            v.put("canal", s.getUploaderName());
            v.put("seg", s.getDuration());
            v.put("vistas", s.getViewCount());
            l.put(v);
        }
        JSONObject r = new JSONObject();
        r.put("videos", l);
        r.put("mas", hayMas);
        return r;
    }

    // --- De dónde sale cada vídeo ---

    private Video.Fuente fuente(String f) throws Exception {
        Video.Fuente r = new Video.Fuente();
        r.cabeceras = new HashMap<>();
        r.cabeceras.put("User-Agent", Descargador.AGENTE);
        if (f.startsWith("archivo:")) {
            if (!pruebas) {
                throw new IOException("solo en las pruebas");
            }
            File a = new File(ctx.getFilesDir(), f.substring(8).replaceAll("[^a-z0-9]", "") + ".mp4");
            r.video = r.audio = a.getAbsolutePath();
            return r;
        }
        if (!f.contains("youtube.com/") && !f.contains("youtu.be/")) {
            r.video = r.audio = f; // un vídeo cualquiera (los de Instagram)
            if (f.contains("instagram") || f.contains("fbcdn")) {
                r.cabeceras.put("Referer", "https://www.instagram.com/");
            }
            return r;
        }
        synchronized (fuentes) {
            Long en = fuentesEn.get(f);
            if (en != null && System.currentTimeMillis() - en < 30 * 60 * 1000) {
                return fuentes.get(f); // al pausar o saltar se vuelve a pedir el mismo
            }
        }
        StreamInfo info = StreamInfo.getInfo(ServiceList.YouTube, f);
        r.dur = info.getDuration() * 1000;
        // lo más pequeño que lleve imagen y sonido juntos (casi siempre el de 360p)…
        VideoStream junto = null;
        for (VideoStream v : info.getVideoStreams()) {
            if (v.isUrl() && v.getDeliveryMethod() == DeliveryMethod.PROGRESSIVE_HTTP && v.getFormat() == MediaFormat.MPEG_4
                    && (junto == null || v.getHeight() < junto.getHeight())) {
                junto = v;
            }
        }
        if (junto != null) {
            r.video = r.audio = junto.getContent();
        } else {
            // …y si no, la imagen sola (la más pequeña desde 144p) y el sonido aparte (el más ligero)
            VideoStream sola = null;
            for (VideoStream v : info.getVideoOnlyStreams()) {
                if (!v.isUrl() || v.getDeliveryMethod() != DeliveryMethod.PROGRESSIVE_HTTP) {
                    continue;
                }
                boolean mp4 = v.getFormat() == MediaFormat.MPEG_4, webm = v.getFormat() == MediaFormat.WEBM;
                if (!mp4 && !webm) {
                    continue;
                }
                int alto = v.getHeight();
                if (alto < 144 || alto > 480) {
                    continue;
                }
                if (sola == null || (mp4 && sola.getFormat() != MediaFormat.MPEG_4) || (mp4 == (sola.getFormat() == MediaFormat.MPEG_4) && alto < sola.getHeight())) {
                    sola = v;
                }
            }
            AudioStream sonido = null;
            for (AudioStream a : info.getAudioStreams()) {
                if (!a.isUrl() || a.getDeliveryMethod() != DeliveryMethod.PROGRESSIVE_HTTP) {
                    continue;
                }
                if (a.getFormat() != MediaFormat.M4A && a.getFormat() != MediaFormat.WEBMA_OPUS && a.getFormat() != MediaFormat.WEBMA) {
                    continue;
                }
                if (sonido == null || (a.getFormat() == MediaFormat.M4A && sonido.getFormat() != MediaFormat.M4A)
                        || (a.getFormat() == sonido.getFormat() && a.getAverageBitrate() < sonido.getAverageBitrate())) {
                    sonido = a;
                }
            }
            if (sola == null) {
                throw new IOException("YouTube no da este vídeo en un formato que se pueda pasar");
            }
            r.video = sola.getContent();
            r.audio = sonido != null ? sonido.getContent() : null;
        }
        synchronized (fuentes) {
            fuentes.put(f, r);
            fuentesEn.put(f, System.currentTimeMillis());
        }
        return r;
    }
}
