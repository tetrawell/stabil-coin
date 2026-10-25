# Stabil Coin

MEXC'deki coinler arasından karakterli, yani hep benzer boyda gidip gelen coinleri bulan Android uygulaması.

## Telefona kurmak

En son sürüm: **[StabilCoin.apk](../../releases/latest/download/StabilCoin.apk)**

Bu bağlantıyı telefonda açıp dosyayı indir, sonra dosyaya dokunup kur. Yeni sürüm çıktığında aynı bağlantıdan indirip üstüne kurman yeterli; ayarların korunur. Eski sürümler [Sürümler](../../releases) sayfasında durur.

## Nasıl çalışır

1. Günlük hacmi sınırın üstündeki USDT paritelerini alır (varsayılan 1 milyon $).
2. Her coin için 1dk ve 5dk grafiklerinin son 1500, 15dk, 1s, 4s ve 1g grafiklerinin son 1000 mumunu indirir (MEXC bir seferde 500 verdiği için sayfa sayfa).
3. Şimdiden geriye bakar; ilk şokta (dev mum, uzun iğne, fiyatın kat değiştirdiği sıçrama ya da çöküş) durur.
4. Şoktan sonraki bölgede dalgalar benzer boyda, benzer sürede ve dar bir bantta tekrar ediyorsa coin "X saattir stabil" sayılır.
5. En az 5 tam dalga görülmeli, dipten çıkışlar en küçük dalga ayarından (varsayılan %1) büyük olmalı, puan en az 60 olmalı ve çok hızlı düşen coinler elenir.

Şok sayılanlar: normal dalganın 3 katı ve üstü bir sıçrama ya da çöküş, veya tek başına normal bir dalgadan büyük olan ve normal mumların 6 katını aşan bir mum.

## Sarkaç ★ listesi

Üstteki "Liste" seçiminden **Sarkaç ★**'a geçince uygulama, alım-sat botuna daha uygun görünen coinleri gösterir. Her kartta da "Sarkaç" puanı ve "BTC bağı" yazar.

- **Sarkaç puanı:** fiyat hep aynı ortalamaya geri mi dönüyor (sarkaç), yoksa bir yöne mi kaçıyor (yokuştan yuvarlanan top)? İndirilen bütün mumlarda kısa adımların (1 mum) oynaklığı, uzun adımlarınkiyle (60 mum) karşılaştırılır. Puan = 100 × (1 − oran). 40 ve üstü sarkaç, 0 civarı rastgele, eksi değer bir yöne kaçan coin.
- **BTC bağı:** coinin kısa vadede BTC ile ne kadar birlikte hareket ettiği (0 = bağımsız, 1 = tıpatıp aynı). 0,4'ün altı "bağımsız".
- **Sığ coin uyarısı:** 1 dk mumlarda tahmini alış-satış farkı %0,25'ten büyükse ya da mumların %20'sinden fazlasında fiyat hiç kıpırdamıyorsa coin sığdır. Sığ coinlerde fiyat alış ve satış arasında zıpladığı için sahte bir "sarkaç" görünür; bu coinler yıldız almaz.
- **★ (yıldız):** sarkaç (puan 40+) + BTC'den bağımsız + sığ değil.

Neden: Kripto Simülatör araştırmasında (Ekim 2026, 62 likit coin, 1/5/15 dk) botun sonucu bu coinlerde belirgin şekilde daha iyiydi; en net 5 dk grafikte. Bu kanıtlanmış bir kazanç değildir; yeni veride doğrulanana kadar sadece sanal parayla dene. Hesap `app/src/main/java/com/stabilcoin/core/Sarkac.kt` dosyasında.

Veri saklanmaz; her açılışta taze indirilir ve uygulama kapanınca silinir. Hesap, anahtar ya da şifre kullanılmaz; sadece MEXC'nin herkese açık fiyatları okunur.

Hesabın kendisi `app/src/main/java/com/stabilcoin/core/Analiz.kt` dosyasında.
