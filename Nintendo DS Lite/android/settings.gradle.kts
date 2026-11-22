// Puente DS: la app de Android que lleva el puente de Panther OS para la Nintendo DS Lite. Se
// compila con el gradlew de la raíz del repositorio (antes hay que poner el puente en jniLibs, como
// hace nintendo-ds-android.yml):
//   ./gradlew -p "Nintendo DS Lite/android" assembleDebug
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
        // NewPipe Extractor (YouTube) está en JitPack
        maven { url = uri("https://jitpack.io") }
    }
}

rootProject.name = "PuenteDS"
include(":app")
