plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// GitHub her paketlemede sürüm numarasını bir artırır; telefon yeni sürümü eskisinin üstüne kurar.
val calismaNo = (System.getenv("GITHUB_RUN_NUMBER") ?: "1").toInt()

// İmza anahtarı depoda şifreyle kilitli durur (imza/stabilcoin.p12); şifresi GitHub'ın gizli kasasındadır.
val imzaDosyasi: String? = System.getenv("IMZA_DOSYASI")

android {
    namespace = "com.stabilcoin"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.stabilcoin"
        minSdk = 26
        targetSdk = 35
        versionCode = calismaNo
        versionName = "1.0.$calismaNo"
    }

    signingConfigs {
        create("surum") {
            if (imzaDosyasi != null) {
                storeFile = file(imzaDosyasi)
                storePassword = System.getenv("IMZA_SIFRE")
                keyAlias = System.getenv("IMZA_ADI")
                keyPassword = System.getenv("IMZA_SIFRE")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = if (imzaDosyasi != null) signingConfigs.getByName("surum") else signingConfigs.getByName("debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        compose = true
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-core")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-viewmodel-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    testImplementation("junit:junit:4.13.2")
    // Testlerde MEXC cevaplarını gerçekten okuyabilmek için (Android'in kendi org.json'u testte boş bir kalıptır).
    testImplementation("org.json:json:20240303")
}
