package com.panthercode.puenteds;

import android.app.Activity;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.webkit.CookieManager;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import java.io.File;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;

// Entrar en Instagram para la DS: la web de Instagram dentro de la app. La dueña de la DS escribe
// aquí su usuario y su contraseña (en la página de Instagram, como en el navegador); en cuanto
// Instagram da la sesión, se le pasa al puente (puente/instagram.go la comprueba y la guarda) y se
// cierra. La contraseña no la ve la app: solo la sesión, que se queda en el móvil.
public class EntrarInstagram extends Activity {
    // el mismo que agenteNavegador en puente/ig/instagram.go
    static final String AGENTE = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36";
    private WebView web;
    private final Handler reloj = new Handler(Looper.getMainLooper());
    private boolean hecho;

    static File carpeta(android.content.Context c) {
        return new File(Servicio.datos(c), "instagram");
    }

    @Override
    protected void onCreate(Bundle b) {
        super.onCreate(b);
        CookieManager cm = CookieManager.getInstance();
        cm.setAcceptCookie(true);
        if (Build.VERSION.SDK_INT >= 21) {
            cm.removeAllCookies(null); // empezar de cero (solo las de esta app)
        }
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        // como el Chrome de un PC: es lo mismo que dice el puente al usar la sesión
        s.setUserAgentString(AGENTE);
        s.setUseWideViewPort(true);
        s.setLoadWithOverviewMode(true);
        s.setBuiltInZoomControls(true);
        s.setDisplayZoomControls(false);
        if (Build.VERSION.SDK_INT >= 21) {
            cm.setAcceptThirdPartyCookies(web, true);
        }
        web.setWebViewClient(new WebViewClient() {
            @Override
            public void onPageFinished(WebView v, String url) {
                mirar();
            }
        });
        setContentView(web);
        web.loadUrl("https://www.instagram.com/accounts/login/");
        reloj.postDelayed(this::cadaSegundo, 1000);
    }

    private void cadaSegundo() {
        if (hecho) {
            return;
        }
        mirar();
        reloj.postDelayed(this::cadaSegundo, 1000);
    }

    // mirar: si Instagram ya ha dado la sesión, al puente.
    private void mirar() {
        if (hecho) {
            return;
        }
        String c = CookieManager.getInstance().getCookie("https://www.instagram.com");
        if (c == null || !c.contains("sessionid=")) {
            return;
        }
        hecho = true;
        try {
            File d = carpeta(this);
            d.mkdirs();
            File tmp = new File(d, "sesion-nueva.tmp");
            try (FileOutputStream o = new FileOutputStream(tmp)) {
                o.write(c.getBytes(StandardCharsets.UTF_8));
            }
            tmp.renameTo(new File(d, "sesion-nueva"));
            Toast.makeText(this, "Hecho: el puente ya tiene tu Instagram", Toast.LENGTH_LONG).show();
        } catch (Exception e) {
            Toast.makeText(this, "No se ha podido guardar: " + e.getMessage(), Toast.LENGTH_LONG).show();
        }
        if (Build.VERSION.SDK_INT >= 21) {
            CookieManager.getInstance().removeAllCookies(null); // la sesión ya la tiene el puente
        }
        finish();
    }

    @Override
    protected void onDestroy() {
        reloj.removeCallbacksAndMessages(null);
        if (web != null) {
            web.destroy();
        }
        super.onDestroy();
    }
}
