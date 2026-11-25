package com.swordfish.lemuroid.app.shared.game

import android.content.Context
import android.os.Process
import timber.log.Timber
import java.io.File
import java.io.RandomAccessFile
import java.nio.ByteBuffer
import java.nio.channels.FileChannel
import java.nio.channels.FileLock

object GameProcessLock {
    private const val LOCK_FILE_NAME = "game_process.lock"
    private const val INTENTOS = 5
    private const val ESPERA_MS = 40L

    @Volatile
    private var channel: FileChannel? = null

    @Volatile
    private var lock: FileLock? = null

    fun acquire(appContext: Context) {
        if (lock != null) return
        val canal = RandomAccessFile(archivo(appContext), "rw").channel
        channel = canal
        // Pokemulador: si justo lo está mirando la app (isHeldByAnotherProcess lo coge un instante),
        // se vuelve a intentar.
        for (i in 1..INTENTOS) {
            lock = runCatching { canal.tryLock() }.getOrNull()
            if (lock != null) break
            Thread.sleep(ESPERA_MS)
        }
        if (lock == null) {
            Timber.e("No se ha podido coger el cerrojo del juego")
            return
        }
        // Qué proceso es, por si hay que cerrarlo porque se ha quedado colgado.
        runCatching {
            canal.truncate(0)
            canal.write(ByteBuffer.wrap(Process.myPid().toString().toByteArray()), 0)
        }
    }

    fun isHeldByAnotherProcess(appContext: Context): Boolean {
        RandomAccessFile(archivo(appContext), "rw").channel.use { ch ->
            val testLock = ch.tryLock()
            return if (testLock != null) {
                testLock.release()
                false
            } else {
                true
            }
        }
    }

    /** Pokemulador: el proceso que apuntó el último juego al abrirse (puede que ya no exista). */
    fun procesoApuntado(appContext: Context): Int? =
        runCatching { archivo(appContext).readText().trim().toInt() }.getOrNull()

    private fun archivo(appContext: Context) = File(appContext.filesDir, LOCK_FILE_NAME)
}
