package com.panthercode.puenteds;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.IBinder;
import android.os.PowerManager;

import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;
import java.util.TimeZone;

// El puente en segundo plano: arranca libpuente.so (el puente de Go, «puente/» en la carpeta de la
// DS) y, si se para, lo vuelve a arrancar. Antes abre el ayudante (Ayudante.java: YouTube y los
// vídeos, que el puente de Go no puede hacer solo en Android). Con un aviso fijo: Android lo pide para no cerrarlo, y
// así se ve que está encendido. Mantiene el wifi despierto y deja oír los mensajes a toda la red
// (con los que la DS busca el puente).
public class Servicio extends Service {
    interface Apunte {
        void apuntar(String s);
    }

    static final String CANAL = "puente";
    static volatile boolean enMarcha;

    private volatile boolean parar;
    private volatile Process proceso;
    private WifiManager.MulticastLock multicast;
    private WifiManager.WifiLock wifiLock;
    private PowerManager.WakeLock wakeLock;
    private boolean falso;
    private String codigoPrueba;
    private Ayudante ayudante;

    static File datos(Context c) {
        return new File(c.getFilesDir(), "puente");
    }

    static File registro(Context c) {
        return new File(c.getFilesDir(), "registro.txt");
    }

    static void encender(Context c, Intent extras) {
        Intent i = new Intent(c, Servicio.class);
        if (extras != null && extras.getExtras() != null) {
            i.putExtras(extras.getExtras());
        }
        if (Build.VERSION.SDK_INT >= 26) {
            c.startForegroundService(i);
        } else {
            c.startService(i);
        }
    }

    static void apagar(Context c) {
        c.stopService(new Intent(c, Servicio.class));
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null) {
            falso = intent.getBooleanExtra("falso", falso);
            String c = intent.getStringExtra("codigo");
            if (c != null) {
                codigoPrueba = c;
            }
        }
        ponerAviso();
        if (!enMarcha) {
            arrancar();
        }
        return START_STICKY;
    }

    private void ponerAviso() {
        NotificationManager nm = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        Notification.Builder b;
        if (Build.VERSION.SDK_INT >= 26) {
            nm.createNotificationChannel(new NotificationChannel(CANAL, "Puente DS", NotificationManager.IMPORTANCE_LOW));
            b = new Notification.Builder(this, CANAL);
        } else {
            b = new Notification.Builder(this);
        }
        PendingIntent abrir = PendingIntent.getActivity(this, 0, new Intent(this, Principal.class),
                Build.VERSION.SDK_INT >= 23 ? PendingIntent.FLAG_IMMUTABLE : 0);
        b.setSmallIcon(R.drawable.icono)
                .setContentTitle("Puente DS en marcha")
                .setContentText("La Nintendo DS usa WhatsApp, Instagram y YouTube a través de este móvil")
                .setContentIntent(abrir)
                .setOngoing(true);
        Notification n = b.build();
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        } else {
            startForeground(1, n);
        }
    }

    private void arrancar() {
        enMarcha = true;
        parar = false;
        WifiManager wm = (WifiManager) getApplicationContext().getSystemService(WIFI_SERVICE);
        multicast = wm.createMulticastLock("puente-ds");
        multicast.setReferenceCounted(false);
        multicast.acquire();
        wifiLock = wm.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "puente-ds");
        wifiLock.setReferenceCounted(false);
        wifiLock.acquire();
        PowerManager pm = (PowerManager) getSystemService(POWER_SERVICE);
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "puente-ds:red");
        wakeLock.setReferenceCounted(false);
        wakeLock.acquire();
        try {
            ayudante = new Ayudante(this, falso, this::apuntar);
        } catch (Exception e) {
            apuntar("sin ayudante (YouTube y vídeos): " + e);
        }
        new Thread(this::vigilar, "puente").start();
    }

    // vigilar: arranca el puente y, si se para, lo vuelve a arrancar a los 5 segundos.
    private void vigilar() {
        while (!parar) {
            try {
                File bin = new File(getApplicationInfo().nativeLibraryDir, "libpuente.so");
                List<String> args = new ArrayList<>();
                args.add(bin.getAbsolutePath());
                args.add("-datos");
                args.add(datos(this).getAbsolutePath());
                args.add("-nombre");
                args.add("Puente DS (" + Build.MODEL + ")");
                args.add("-probar-red");
                args.add("-dns");
                args.add(servidoresDNS());
                if (falso) {
                    args.add("-falso");
                    args.add("-pruebas");
                }
                if (codigoPrueba != null) {
                    args.add("-codigo");
                    args.add(codigoPrueba);
                }
                ProcessBuilder pb = new ProcessBuilder(args);
                pb.environment().put("TZ", TimeZone.getDefault().getID());
                pb.environment().put("HOME", getFilesDir().getAbsolutePath());
                if (ayudante != null) {
                    pb.environment().put("PUENTE_AYUDANTE", "127.0.0.1:" + ayudante.puerto);
                    pb.environment().put("PUENTE_AYUDANTE_CLAVE", ayudante.clave);
                }
                pb.redirectErrorStream(true);
                apuntar("arranco el puente");
                proceso = pb.start();
                copiarRegistro(proceso.getInputStream());
                int salida = proceso.waitFor();
                apuntar("el puente se ha parado (" + salida + ")");
            } catch (Exception e) {
                apuntar("no se puede arrancar el puente: " + e);
            }
            if (!parar) {
                try {
                    Thread.sleep(5000);
                } catch (InterruptedException ignorado) {
                    return;
                }
            }
        }
    }

    // servidoresDNS: los del wifi (Go no los encuentra solo en Android), y unos de reserva.
    private String servidoresDNS() {
        Set<String> s = new LinkedHashSet<>();
        try {
            ConnectivityManager cm = (ConnectivityManager) getSystemService(CONNECTIVITY_SERVICE);
            for (Network n : cm.getAllNetworks()) {
                LinkProperties lp = cm.getLinkProperties(n);
                if (lp == null) {
                    continue;
                }
                for (InetAddress a : lp.getDnsServers()) {
                    String h = a.getHostAddress();
                    if (h != null) {
                        int pct = h.indexOf('%');
                        s.add(pct >= 0 ? h.substring(0, pct) : h);
                    }
                }
            }
        } catch (Exception e) {
            apuntar("DNS: " + e);
        }
        s.add("8.8.8.8");
        s.add("1.1.1.1");
        return android.text.TextUtils.join(",", s);
    }

    private void copiarRegistro(InputStream in) {
        Thread t = new Thread(() -> {
            byte[] b = new byte[4096];
            try {
                int n;
                while ((n = in.read(b)) > 0) {
                    File f = registro(this);
                    if (f.length() > 512 * 1024) { // que no crezca sin fin
                        f.renameTo(new File(getFilesDir(), "registro-anterior.txt"));
                    }
                    try (FileOutputStream o = new FileOutputStream(registro(this), true)) {
                        o.write(b, 0, n);
                    }
                }
            } catch (Exception ignorado) {
            }
        }, "registro");
        t.start();
    }

    private synchronized void apuntar(String s) {
        try (FileOutputStream o = new FileOutputStream(registro(this), true)) {
            o.write(("[app] " + s + "\n").getBytes("UTF-8"));
        } catch (Exception ignorado) {
        }
    }

    @Override
    public void onDestroy() {
        parar = true;
        Process p = proceso;
        if (p != null) {
            p.destroy();
        }
        if (ayudante != null) {
            ayudante.cerrar();
        }
        if (multicast != null && multicast.isHeld()) {
            multicast.release();
        }
        if (wifiLock != null && wifiLock.isHeld()) {
            wifiLock.release();
        }
        if (wakeLock != null && wakeLock.isHeld()) {
            wakeLock.release();
        }
        enMarcha = false;
        apuntar("puente apagado");
        super.onDestroy();
    }
}
