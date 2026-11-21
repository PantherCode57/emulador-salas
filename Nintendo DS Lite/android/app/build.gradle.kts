plugins {
    id("com.android.application")
}

android {
    namespace = "com.panthercode.puenteds"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.panthercode.puenteds"
        // Para móviles antiguos: desde Android 5.
        minSdk = 21
        targetSdk = 34
        versionCode = 1
        versionName = "1.0"
    }

    // La misma llave de pruebas que las otras apps del repositorio: así cada versión nueva se
    // instala encima de la anterior sin perder la sesión de WhatsApp.
    signingConfigs {
        getByName("debug") {
            storeFile = rootProject.file("../../debug.keystore")
        }
    }

    buildTypes {
        getByName("release") {
            isMinifyEnabled = false
            signingConfig = signingConfigs["debug"]
        }
    }

    // El puente (libpuente.so) es un programa: tiene que estar suelto en el móvil para poder
    // arrancarlo (desde Android 10 solo se pueden arrancar programas de la carpeta de librerías).
    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }

    lint {
        abortOnError = false
        checkReleaseBuilds = false
    }
}
