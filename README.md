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
- **Alıp satma masrafı (1.0.5):** uygulama her coinin MEXC alış-satış defterini (en iyi 100 basamak) ve MEXC'nin o coinden aldığı komisyonu da indirir. Ayarlardaki **kumbara tutarı** (varsayılan 100 USDT) kadar piyasa emriyle bir alış + bir satışın masrafı hesaplanır: alış-satış farkı + defteri yiyerek oluşan fiyat kayması + iki komisyon. Defteri ince coinde tutar büyüdükçe masraf artar.
- **Salınım:** fiyatın son 20 mumluk ortalamasından tipik uzaklığı (2 standart sapma; Bollinger bandının ortadan kenara mesafesi), bütün pencerelerin ortancası. Her zaman diliminde ayrı hesaplanır.
- **Kat:** salınım ÷ masraf. Köprü geçiş ücreti gibi: karşıda kazanılacak para ücretin birkaç katı değilse geçmeye değmez. Kart her zaman dilimi için masrafı, salınımı ve katı yazar; 3'ün altındaysa kırmızı uyarı çıkar.
- **★ (yıldız):** sarkaç (puan 40+) + BTC'den bağımsız + sığ değil + salınım masrafın en az 3 katı. "Masrafı karşılamayanları da göster" seçilince yıldızı olmayan sarkaç adayları da listelenir (yıldızlılar her zaman üstte).

Neden masraf (26 Ekim 2026): Raspberry Pi'deki Kripto Al-Sat'ta 16 saatlik sanal denemede simülatörün beklediği ile gerçekleşen arasındaki farkın neredeyse tamamı defteri ince dört coinden geldi (AXIS, BDX, DOS, MBG). Hacim bunu göstermiyordu: AXIS ile BNB'nin günlük hacmi aynıydı ama fiyatın %0,5 altına kadar bekleyen alıcı BNB'de 134.000 $, AXIS'te 582 $ idi. Aynı gün bütün coinlerde yapılan taramada 16 sarkaç adayının salınımı ya masrafın 1,7 katından az ya da 4,7 katından fazlaydı; aradaki her sınır (2–4) aynı sonucu verdi. Sarkaç puanı yüksek ama salınımı küçük coinlerin "sarkacı" çoğu zaman alış-satış farkının kendisidir. 1 dk grafikte salınımı masrafın 3 katını geçen coin neredeyse yok (44 coinde 3); 5 dk'da yarısı, 15 dk'da üçte ikisi geçiyor. Hesap `app/src/main/java/com/stabilcoin/core/Defter.kt` dosyasında.

Neden: Kripto Simülatör araştırmasında (Ekim 2026, 62 likit coin, 1/5/15 dk) botun sonucu bu coinlerde belirgin şekilde daha iyiydi; en net 5 dk grafikte. Bu kanıtlanmış bir kazanç değildir; yeni veride doğrulanana kadar sadece sanal parayla dene. Hesap `app/src/main/java/com/stabilcoin/core/Sarkac.kt` dosyasında.

Veri saklanmaz; her açılışta taze indirilir ve uygulama kapanınca silinir. Hesap, anahtar ya da şifre kullanılmaz; sadece MEXC'nin herkese açık fiyatları okunur.

Hesabın kendisi `app/src/main/java/com/stabilcoin/core/Analiz.kt` dosyasında.
