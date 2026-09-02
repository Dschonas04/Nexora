// Zwei Browser an derselben Seite.
//
// Geprüft wird die Leitung aus src/mitschrift.ts gegen einen Verteiler, der
// sich verhält wie der echte: jedes Paket geht an alle, den Absender
// eingeschlossen. Ein Netz braucht es dafür nicht, wohl aber die richtige
// Reihenfolge der Pakete, und genau dort sassen beide Fehler, die diese Probe
// beim ersten Lauf gefunden hat: ein Dazugekommener erfuhr nichts von den
// bereits Anwesenden, und wer ging, blieb bei den anderen stehen.
//
// Ausgeführt wird sie nicht von einem Testläufer, den es hier nicht gibt,
// sondern von der Werkbank:
//
//   npx esbuild test/eingang.ts --bundle --format=esm --outfile=/tmp/m.mjs
//   node test/mitschrift-probe.mjs /tmp/m.mjs
const anschluesse = new Set();

class FakeWS {
  static OPEN = 1;
  constructor() {
    this.readyState = 1;
    anschluesse.add(this);
    queueMicrotask(() => this.onopen && this.onopen());
  }
  send(paket) {
    // Der Verteiler des Dienstes: an alle, auch zurück an den Absender.
    for (const a of anschluesse) {
      queueMicrotask(() => a.onmessage && a.onmessage({ data: paket.buffer.slice(paket.byteOffset, paket.byteOffset + paket.byteLength) }));
    }
  }
  close() { this.readyState = 3; anschluesse.delete(this); this.onclose && this.onclose(); }
}
globalThis.WebSocket = FakeWS;
globalThis.window = { addEventListener() {}, removeEventListener() {}, setTimeout, clearTimeout,
  location: { protocol: "http:", host: "probe" } };

const gebaut = process.argv[2];
if (!gebaut) {
  console.error("Aufruf: node mitschrift-probe.mjs <gebündelte eingang.ts>");
  process.exit(2);
}
const { Leitung, Y } = await import(gebaut.startsWith("/") ? gebaut : "./" + gebaut);

const ruhe = () => new Promise((f) => setTimeout(f, 60));
const fehler = [];
const pruefe = (was, erwartet, bekommen) => {
  const ok = JSON.stringify(erwartet) === JSON.stringify(bekommen);
  console.log(`  ${ok ? "ok   " : "FEHLT"} ${was}` + (ok ? "" : ` (erwartet ${JSON.stringify(erwartet)}, bekam ${JSON.stringify(bekommen)})`));
  if (!ok) fehler.push(was);
};

// --- Erster Browser, allein im Raum
const docA = new Y.Doc();
const a = new Leitung("ws://probe/api/echtzeit/x", docA);
a.anwesenheit.setLocalStateField("user", { name: "Anna", color: "#2383e2" });
await ruhe();
pruefe("allein: Abgleich kommt trotzdem zustande", true, a.abgeglichen);
pruefe("allein: verbunden", true, a.verbunden);

docA.getText("t").insert(0, "Hallo");
await ruhe();

// --- Zweiter Browser kommt dazu
const docB = new Y.Doc();
const b = new Leitung("ws://probe/api/echtzeit/x", docB);
b.anwesenheit.setLocalStateField("user", { name: "Bert", color: "#bf5b04" });
await ruhe();
pruefe("der Dazugekommene bekommt den Text", "Hallo", docB.getText("t").toString());
pruefe("beide sehen zwei Anwesende", [2, 2], [a.anwesenheit.getStates().size, b.anwesenheit.getStates().size]);
pruefe("der Name des anderen kommt an", "Anna",
  [...b.anwesenheit.getStates().values()].map((z) => z.user.name).sort()[0]);

// --- Gleichzeitig tippen, an derselben Stelle
docA.getText("t").insert(5, " von Anna");
docB.getText("t").insert(5, " von Bert");
await ruhe();
pruefe("beide Fassungen sind gleich", docA.getText("t").toString(), docB.getText("t").toString());
pruefe("keine Eingabe ist verloren", true,
  docA.getText("t").toString().includes("Anna") && docA.getText("t").toString().includes("Bert"));

// --- Die Marke, an der die Saat hängt
docA.getMap("nexora").set("gesaet", true);
await ruhe();
pruefe("die Marke fährt mit", true, docB.getMap("nexora").get("gesaet"));

// --- Einer geht
b.destroy();
await ruhe();
pruefe("der Abgang verschwindet aus der Anwesenheit", 1, a.anwesenheit.getStates().size);

// --- Verbindung weg: es wird weiter getippt, nur nicht verteilt
const docC = new Y.Doc();
const c = new Leitung("ws://probe/api/echtzeit/x", docC);
await ruhe();
[...anschluesse].forEach((ws) => ws.close());
await ruhe();
pruefe("Abriss wird bemerkt", [false, false], [a.verbunden, c.verbunden]);
docA.getText("t").insert(0, "!");
pruefe("ohne Leitung geht das Tippen weiter", true, docA.getText("t").toString().startsWith("!"));

a.destroy(); c.destroy();
console.log(fehler.length === 0 ? "\nProbe bestanden." : `\n${fehler.length} gefallen.`);
process.exit(fehler.length === 0 ? 0 : 1);
