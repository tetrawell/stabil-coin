# Geçici araştırma dalı (Kripto Simülatör — seçenek seçimi)

Bu dal telefon uygulamasıyla ilgili değil. Kripto Simülatör'ün "En kârlı ayarı bul" kartındaki seçeneklerden
hangisinin, ayarın hiç görmediği sonraki günlerde (Kripto Al-Sat'taki dosya ömrü kadar) daha iyi sonuç verdiğini
ölçer: bulucunun önerisi, en kârlı görünen, sınavda en iyi olan ve "zararı da gören" puanla bulunan ayar.
Veri: `arastirma-veri-sonuc` dalındaki MEXC mumları. Sonuç `arastirma-secim-sonuc` dalına yazılır.

## 2. tur (28 Ekim 2026, dal `arastirma-secim2`)
Kullanıcının zinciriyle: Stabil Coin'in 5 dk hesabıyla "uygun" ve yönü düşmeyen coinler, 5 dk 10.000 mum, kullanıcının
ayarlarındaki kurallar (simülatörün varsayılan stratejisi), sadece güvenilir kâr hedefi. Her pencerede aramanın gördüğü
dönemin günlük hacmi ve 10 parçalık getirileri (düşüş / yatay / yükseliş görmüş mü) de yazılır. Sonuç
`arastirma-secim2-sonuc` dalına.
