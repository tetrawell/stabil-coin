// Test için sahte bir "KriptoSimulator" klasörü kurar: stratejiler/ + kaynak/web (motor kopyası)
// kullanım: node test_klasoru_kur.js <proje_web_klasoru> <hedef_klasor> <kullanici_AGT_dosyasi>
const fs = require('fs'), path = require('path'), vm = require('vm');
const [web, hedef, agt] = process.argv.slice(2);
for (const f of ['gostergeler.js', 'kurallar.js', 'motor.js']) vm.runInThisContext(fs.readFileSync(path.join(web, f), 'utf8'));
const strat = path.join(hedef, 'KriptoSimulator', 'stratejiler');
const kaynak = path.join(hedef, 'KriptoSimulator', 'kaynak', 'web');
fs.rmSync(hedef, { recursive: true, force: true });
fs.mkdirSync(strat, { recursive: true }); fs.mkdirSync(kaynak, { recursive: true });
for (const f of ['gostergeler.js', 'kurallar.js', 'motor.js']) fs.copyFileSync(path.join(web, f), path.join(kaynak, f));
const simdi = Date.now();
const iso = ms => new Date(ms).toISOString();
function dosya(sembol, aralik, s, olusturulduMs, ek = {}) {
  return Object.assign({
    tur: 'kripto-simulator-strateji', surum: 3, olusturuldu: iso(olusturulduMs), borsa: 'mexc', sembol, aralik, strateji: s,
    gecmisSonuc: { mumSayisi: 1000, ilkMum: iso(olusturulduMs - 1000 * 60000), sonMum: iso(olusturulduMs), botYuzde: 4.3, alTutYuzde: 2.1, islemSayisi: 24, karliIslem: 14 },
    aciklama: 'test',
  }, ek);
}
const yaz = (ad, icerik) => fs.writeFileSync(path.join(strat, ad), typeof icerik === 'string' ? icerik : JSON.stringify(icerik, null, 2));
// 1) kullanıcının AGT dosyası, oluşturulma zamanı 25 dk önce olacak şekilde
const a = JSON.parse(fs.readFileSync(agt, 'utf8'));
a.olusturuldu = iso(simdi - 25 * 60000);
yaz('AGTUSDT_1m.json', a);
// 2) BTC 15 dk varsayılan strateji (vites açık)
const v = VARSAYILAN_STRATEJI();
yaz('BTCUSDT_15m.json', dosya('BTCUSDT', '15m', v, simdi - 3 * 3600e3, { gecmisSonuc: { mumSayisi: 2000, ilkMum: iso(simdi - 3 * 3600e3 - 2000 * 900e3), sonMum: iso(simdi - 3 * 3600e3), botYuzde: 6.12, alTutYuzde: -1.3, islemSayisi: 31, karliIslem: 19 } }));
// 3) BTC 1 sa (aynı coin, farklı mum süresi)
const v2 = VARSAYILAN_STRATEJI(); v2.kurallar.hedefKar.acik = true; v2.kurallar.hedefKar.d.yuzde = 2.5;
yaz('BTCUSDT_1h.json', dosya('BTCUSDT', '1h', v2, simdi - 30 * 3600e3));
// 4) süresi dolmuş ETH 5 dk (3 gün ömür, 80 saat önce)
yaz('ETHUSDT_5m.json', dosya('ETHUSDT', '5m', VARSAYILAN_STRATEJI(), simdi - 80 * 3600e3));
// 5) Binance verisiyle
yaz('SOLUSDT_15m.json', dosya('SOLUSDT', '15m', VARSAYILAN_STRATEJI(), simdi - 3600e3, { borsa: 'binance' }));
// 6) bilinmeyen açık kural
const v3 = VARSAYILAN_STRATEJI(); v3.kurallar.yepyeniKural = { acik: true, d: { x: 1 } };
yaz('XRPUSDT_1m.json', dosya('XRPUSDT', '1m', v3, simdi - 10 * 60000));
// 7) bozuk dosya
yaz('bozuk.json', '{ "tur": "kripto-simulator-strateji", ');
console.log('kuruldu:', strat);
