// La app Panther para el móvil (aparte de Pokemulador). Se compila con el gradlew de la raíz:
//   ./gradlew -p panther-android assembleDebug      (antes: bash panther-android/preparar.sh)
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
    }
}

rootProject.name = "Panther"
include(":app")
