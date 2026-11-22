package com.panthercode.puenteds;

import org.schabi.newpipe.extractor.downloader.Downloader;
import org.schabi.newpipe.extractor.downloader.Request;
import org.schabi.newpipe.extractor.downloader.Response;
import org.schabi.newpipe.extractor.exceptions.ReCaptchaException;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.zip.GZIPInputStream;

// Lo que usa NewPipe Extractor para hablar con YouTube: las peticiones tal cual, con lo que trae
// Android (HttpURLConnection).
class Descargador extends Downloader {
    static final String AGENTE = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0";

    @Override
    public Response execute(Request r) throws IOException, ReCaptchaException {
        HttpURLConnection c = (HttpURLConnection) new URL(r.url()).openConnection();
        c.setConnectTimeout(15000);
        c.setReadTimeout(30000);
        c.setInstanceFollowRedirects(true);
        c.setRequestMethod(r.httpMethod());
        c.setRequestProperty("User-Agent", AGENTE);
        boolean gzip = false;
        for (Map.Entry<String, List<String>> h : r.headers().entrySet()) {
            if (h.getKey() == null || h.getValue() == null) {
                continue;
            }
            boolean primero = true;
            for (String v : h.getValue()) {
                if (primero) {
                    c.setRequestProperty(h.getKey(), v);
                    primero = false;
                } else {
                    c.addRequestProperty(h.getKey(), v);
                }
                if (h.getKey().equalsIgnoreCase("Accept-Encoding") && v.contains("gzip")) {
                    gzip = true;
                }
            }
        }
        byte[] datos = r.dataToSend();
        if (datos != null) {
            c.setDoOutput(true);
            try (OutputStream o = c.getOutputStream()) {
                o.write(datos);
            }
        }
        int codigo = c.getResponseCode();
        if (codigo == 429) {
            c.disconnect();
            throw new ReCaptchaException("YouTube pide confirmar que no es un robot", r.url());
        }
        InputStream in = codigo >= 400 ? c.getErrorStream() : c.getInputStream();
        String cuerpo = "";
        if (in != null && !"HEAD".equals(r.httpMethod())) {
            if (gzip && "gzip".equalsIgnoreCase(c.getContentEncoding())) {
                in = new GZIPInputStream(in);
            }
            ByteArrayOutputStream b = new ByteArrayOutputStream();
            byte[] buf = new byte[16384];
            int n;
            while ((n = in.read(buf)) > 0) {
                b.write(buf, 0, n);
            }
            in.close();
            cuerpo = new String(b.toByteArray(), StandardCharsets.UTF_8);
        }
        Map<String, List<String>> cabeceras = new HashMap<>();
        for (Map.Entry<String, List<String>> h : c.getHeaderFields().entrySet()) {
            if (h.getKey() != null) {
                cabeceras.put(h.getKey(), h.getValue());
            }
        }
        String final_ = c.getURL().toString();
        String mensaje = c.getResponseMessage();
        c.disconnect();
        return new Response(codigo, mensaje, cabeceras, cuerpo, final_);
    }
}
