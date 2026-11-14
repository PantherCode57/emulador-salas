package com.swordfish.lemuroid.app.mobile.feature.online

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.feature.main.MainActivity

/**
 * Mantiene viva la app mientras hay una sala abierta (si no, Android podría cerrar el enlace
 * de Tailscale al pasar al juego). Solo muestra una notificación con el código de la sala.
 */
class SalaService : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(
        intent: Intent?,
        flags: Int,
        startId: Int,
    ): Int {
        crearCanal()
        val codigo = intent?.getStringExtra(EXTRA_CODIGO).orEmpty()
        val abrirApp =
            PendingIntent.getActivity(
                this,
                0,
                Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
        val notificacion =
            NotificationCompat.Builder(this, CANAL)
                .setSmallIcon(R.drawable.ic_lemuroid_tiny)
                .setContentTitle(getString(R.string.emudex_sala_notificacion_titulo))
                .setContentText(getString(R.string.emudex_sala_notificacion_texto, codigo))
                .setContentIntent(abrirApp)
                .setOngoing(true)
                .setPriority(NotificationCompat.PRIORITY_LOW)
                .build()
        ServiceCompat.startForeground(
            this,
            ID_NOTIFICACION,
            notificacion,
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC else 0,
        )
        return START_NOT_STICKY
    }

    private fun crearCanal() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val canal =
                NotificationChannel(
                    CANAL,
                    getString(R.string.emudex_online_title),
                    NotificationManager.IMPORTANCE_LOW,
                )
            ContextCompat.getSystemService(this, NotificationManager::class.java)?.createNotificationChannel(canal)
        }
    }

    companion object {
        private const val CANAL = "SALA_ONLINE"
        private const val ID_NOTIFICACION = 4242
        private const val EXTRA_CODIGO = "codigo"

        fun iniciar(
            context: Context,
            codigo: String,
        ) {
            val intent = Intent(context, SalaService::class.java).putExtra(EXTRA_CODIGO, codigo)
            ContextCompat.startForegroundService(context, intent)
        }

        fun detener(context: Context) {
            context.stopService(Intent(context, SalaService::class.java))
        }
    }
}
