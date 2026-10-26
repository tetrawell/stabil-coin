// node test/motor_ayni_mi.js <simülatörün kaynak/web klasörü>
// Al-Sat'taki motor dosyaları (gostergeler.js, kurallar.js, motor.js) simülatördekilerle birebir aynı mı?
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const sim = process.argv[2];
if (!sim) { console.log('kullanım: node test/motor_ayni_mi.js <simülatörün kaynak/web klasörü>'); process.exit(2); }
const ozet = yol => crypto.createHash('sha256').update(fs.readFileSync(yol, 'utf8').replace(/\r\n/g, '\n')).digest('hex');
let hata = 0;
for (const ad of ['gostergeler.js', 'kurallar.js', 'motor.js']) {
  const a = ozet(path.join(__dirname, '..', 'web', ad)), b = ozet(path.join(sim, ad));
  console.log((a === b ? 'AYNI   ' : 'FARKLI ') + ad + '  ' + a.slice(0, 16));
  if (a !== b) hata++;
}
console.log(hata ? `\n${hata} dosya farklı: simülatörün yeni motorunu Al-Sat'a kopyala ve testleri yeniden çalıştır.` : '\nMotor simülatörle birebir aynı.');
process.exit(hata ? 1 : 0);
