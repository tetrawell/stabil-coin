// Kullanıcının pencereyi kapatmasını taklit eder (Chrome hata ayıklama protokolüyle):
// sayfayı kapatır, "çıkmak istiyor musun" sorusu gelirse "evet" der. Çıktı: sorulan soru türü (yoksa "yok").
// kullanım: node pencere_kapat.js <ws adresi>
const ws = new WebSocket(process.argv[2]);
let no = 0, soru = 'yok';
const gonder = (method, params = {}) => ws.send(JSON.stringify({ id: ++no, method, params }));
// Chrome "çıkmak istiyor musun"u sadece sayfaya bir kez tıklanmışsa sorar: önce boş bir yere tıkla (gerçek kullanıcı gibi)
ws.onopen = () => {
  gonder('Page.enable');
  for (const type of ['mousePressed', 'mouseReleased']) gonder('Input.dispatchMouseEvent', { type, x: 800, y: 6, button: 'left', clickCount: 1 });
  setTimeout(() => gonder('Page.close'), 500);
};
ws.onmessage = e => {
  const m = JSON.parse(e.data);
  if (m.method === 'Page.javascriptDialogOpening') { soru = m.params.type; gonder('Page.handleJavaScriptDialog', { accept: true }); }
};
ws.onclose = () => { console.log(soru); process.exit(0); };
ws.onerror = () => {};
setTimeout(() => { console.log(soru); process.exit(0); }, 8000);
