// Testte sık işlem yapan strateji dosyası yazar: başlatma/tetik kuralları kapalı (hemen alır), sıkı zarar durdur ve hedef.
// kullanım: node hizli_strateji.js <proje_web_klasoru> <stratejiler_klasoru> <SEMBOL> <aralik> [yuzde] [omur_dk_once]
const fs = require('fs'), path = require('path'), vm = require('vm');
const [web, hedefKlasor, sembol, aralik, yuzde, once] = process.argv.slice(2);
for (const f of ['gostergeler.js', 'kurallar.js', 'motor.js']) vm.runInThisContext(fs.readFileSync(path.join(web, f), 'utf8'));
const s = VARSAYILAN_STRATEJI();
for (const k of KURALLAR) {
  if (!s.kurallar[k.id]) s.kurallar[k.id] = { acik: false, d: {} };
  s.kurallar[k.id].acik = false;
}
s.kurallar.zararDurdur.acik = true; s.kurallar.zararDurdur.d.yuzde = +(yuzde || 0.3);
s.kurallar.hedefKar.acik = true; s.kurallar.hedefKar.d.yuzde = +(yuzde || 0.3);
const simdi = Date.now();
const dosya = { tur: 'kripto-simulator-strateji', surum: 3, olusturuldu: new Date(simdi - (+(once || 1)) * 60000).toISOString(), borsa: 'mexc', sembol, aralik, strateji: s,
  gecmisSonuc: { mumSayisi: 300, ilkMum: new Date(simdi - 300 * 60000).toISOString(), sonMum: new Date(simdi).toISOString(), botYuzde: 1, alTutYuzde: 0, islemSayisi: 50, karliIslem: 25 }, aciklama: 'test' };
fs.writeFileSync(path.join(hedefKlasor, `${sembol}_${aralik}.json`), JSON.stringify(dosya, null, 2));
console.log('yazıldı', sembol, aralik);
