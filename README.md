# Stabil Coin

MEXC'deki coinler arasından karakterli, yani hep benzer boyda gidip gelen coinleri bulan Android uygulaması.

## Telefona kurmak

En son sürüm: **[StabilCoin.apk](../../releases/latest/download/StabilCoin.apk)**

Bu bağlantıyı telefonda açıp dosyayı indir, sonra dosyaya dokunup kur. Yeni sürüm çıktığında aynı bağlantıdan indirip üstüne kurman yeterli; ayarların korunur. Eski sürümler [Sürümler](../../releases) sayfasında durur.

## Nasıl çalışır

1. Günlük hacmi sınırın üstündeki USDT paritelerini alır (varsayılan 1 milyon $).
2. Her coin için 1dk, 5dk, 15dk, 1s, 4s ve 1g grafiklerinin son 1000 mumunu indirir.
3. Şimdiden geriye bakar; ilk şokta (dev mum, uzun iğne, fiyatın kat değiştirdiği sıçrama ya da çöküş) durur.
4. Şoktan sonraki bölgede dalgalar benzer boyda, benzer sürede ve dar bir bantta tekrar ediyorsa coin "X saattir stabil" sayılır.
5. En az 5 tam dalga görülmeli, dipten çıkışlar en küçük dalga ayarından (varsayılan %1) büyük olmalı ve çok hızlı düşen coinler elenir.

Veri saklanmaz; her açılışta taze indirilir ve uygulama kapanınca silinir. Hesap, anahtar ya da şifre kullanılmaz; sadece MEXC'nin herkese açık fiyatları okunur.

Hesabın kendisi `app/src/main/java/com/stabilcoin/core/Analiz.kt` dosyasında.
