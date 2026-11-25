plugins {
    id("com.android.library")
}

// El emulador Panther (el de la consola) dentro de Pokemulador, para GB, GBC y GBA. Lo de fuera (SDL2 y
// mGBA con Lua ya compilados) lo deja en panther-android/terceros el script panther-android/preparar.sh.
val terceros = rootProject.file("panther-android/terceros")
val sdl = File(terceros, "SDL2-2.32.10")

android {
    namespace = "com.pokemulador.panther"
    ndkVersion = (findProperty("ndkPanther") as String?) ?: "27.1.12297006"

    defaultConfig {
        ndk {
            abiFilters += ((findProperty("abis") as String?) ?: "arm64-v8a").split(",")
        }
        externalNativeBuild {
            cmake {
                // mGBA y Lua se compilan para Android 8 (26); con uno más viejo no se abre Panther
                // (lo mira Panther.kt) y van los núcleos de antes.
                arguments +=
                    listOf(
                        "-DPANTHER_TERCEROS=${terceros.absolutePath.replace('\\', '/')}",
                        "-DANDROID_PLATFORM=android-26",
                    )
            }
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.22.1"
        }
    }

    sourceSets["main"].java.srcDir(File(sdl, "android-project/app/src/main/java"))
    // La letra (Selawik) de la consola.
    sourceSets["main"].assets.srcDir(rootProject.file("panther-android/app/src/main/assets"))

    lint {
        abortOnError = false
        checkReleaseBuilds = false
    }
}
