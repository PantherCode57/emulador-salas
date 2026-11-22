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
        resources {
            excludes += setOf("META-INF/*.md", "META-INF/LICENSE*", "META-INF/NOTICE*", "META-INF/DEPENDENCIES",
                "META-INF/*.kotlin_module", "META-INF/versions/**")
        }
    }

    // NewPipe Extractor es de Java 17 y usa cosas de Java que los Android antiguos no traen: van
    // dentro de la app («desugaring»).
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
        isCoreLibraryDesugaringEnabled = true
    }

    lint {
        abortOnError = false
        checkReleaseBuilds = false
    }
}

dependencies {
    // YouTube: la librería de la app NewPipe (buscar y sacar los vídeos)
    implementation("com.github.TeamNewPipe:NewPipeExtractor:v0.27.1")
    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs_nio:2.0.4")
}
