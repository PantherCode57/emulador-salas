plugins {
    id("com.android.application")
}

// Lo de fuera (SDL2 y mGBA con Lua ya compilados): lo deja ahí preparar.sh.
val terceros = rootProject.file("terceros")
val sdl = File(terceros, "SDL2-2.32.10")

android {
    namespace = "com.panthercode.panther"
    compileSdk = 35
    ndkVersion = (findProperty("ndk") as String?) ?: "27.1.12297006"

    defaultConfig {
        applicationId = "com.panthercode.panther"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "1.0"
        ndk {
            abiFilters += ((findProperty("abis") as String?) ?: "arm64-v8a,x86_64").split(",")
        }
        externalNativeBuild {
            cmake {
                arguments += listOf("-DPANTHER_TERCEROS=${terceros.absolutePath.replace('\\', '/')}")
            }
        }
    }

    // La misma llave de pruebas que Pokemulador (la de la raíz del repositorio): así cada versión
    // nueva se instala encima de la anterior sin perder las partidas.
    signingConfigs {
        getByName("debug") {
            storeFile = rootProject.file("../debug.keystore")
        }
    }

    buildTypes {
        getByName("release") {
            isMinifyEnabled = false
            signingConfig = signingConfigs["debug"]
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    sourceSets["main"].java.srcDir(File(sdl, "android-project/app/src/main/java"))
    // Los Caramelos Raros de la consola (consola/trucos/caramelos-frlg.lua).
    sourceSets["main"].assets.srcDir(rootProject.file("../consola/trucos"))

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }

    lint {
        abortOnError = false
        checkReleaseBuilds = false
    }
}
