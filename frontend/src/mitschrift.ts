// Gemeinsames Schreiben an einer Seite, die Browser-Seite davon.
//
// Der Dienst reicht nur Pakete weiter, gerechnet wird hier. Yjs führt zwei
// Fassungen desselben Textes zusammen, ohne dass eine die andere überschreibt:
// zwei Leute in verschiedenen Absätzen merken nichts voneinander, zwei im
// selben Wort bekommen ein Ergebnis, das beide Eingaben enthält. Das ist der
// Unterschied zum Speichern der ganzen Seite, bei dem der Letzte gewinnt.
//
// Geschrieben wird trotzdem weiter in die Datenbank, und zwar von genau einem
// der Beteiligten. Wer das ist, wird nicht ausgehandelt, sondern gerechnet: die
// kleinste Kennung im Raum. Alle haben dieselbe Liste, also kommen alle auf
// denselben, und geht der, rückt der Nächste nach, ohne dass jemand etwas
// mitteilen müsste.
import { useEffect, useMemo, useState } from "react";
import * as Y from "yjs";
import * as anwesenheitProtokoll from "y-protocols/awareness";
import * as abgleichProtokoll from "y-protocols/sync";
import * as kodieren from "lib0/encoding";
import * as dekodieren from "lib0/decoding";

// Die beiden Arten von Paket, die über die Leitung gehen. Dieselben Zahlen wie
// bei y-websocket: das Format ist dort nicht erfunden, sondern nur benannt, und
// wer später einen fertigen Dienst davorsetzen will, spricht dieselbe Sprache.
const PAKET_ABGLEICH = 0;
const PAKET_ANWESENHEIT = 1;
// "Wer ist da?" — die Frage, die ein Dazugekommener stellt. Ohne sie wüsste er
// von den anderen erst, wenn einer von ihnen das nächste Mal etwas tut, und
// säße bis dahin scheinbar allein an einer Seite, an der drei arbeiten.
const PAKET_WER_IST_DA = 3;

// Wartezeiten beim Wiederverbinden, in Millisekunden. Kurz anfangen, weil der
// häufigste Fall ein Neustart des Dienstes von ein paar Sekunden ist; nach oben
// gedeckelt, weil ein Reiter, der über Nacht offen steht, nicht im Sekundentakt
// gegen eine tote Adresse klopfen soll.
const WARTE_ANFANG = 1000;
const WARTE_HOECHSTENS = 30000;

/**
 * Die Leitung zu den anderen Browsern.
 *
 * Eigene statt einer fertigen: gebraucht wird genau das hier, ein Anschluss,
 * der Yjs-Pakete hin und her trägt und sich wiederverbindet. Die gängige
 * Bibliothek dafür bringt einen ganzen Server samt Datenbank mit, dreiunddreißig
 * Pakete, von denen eines beim Einrichten nativ übersetzt werden will.
 */
export class Leitung {
  readonly anwesenheit: anwesenheitProtokoll.Awareness;
  verbunden = false;
  /** Der erste Abgleich ist durch: erst danach steht fest, ob schon Text da ist. */
  abgeglichen = false;

  private ws: WebSocket | null = null;
  private beendet = false;
  private warte = WARTE_ANFANG;
  private wecker: number | undefined;
  private horcher = new Set<() => void>();

  constructor(
    private adresse: string,
    private doc: Y.Doc,
  ) {
    this.anwesenheit = new anwesenheitProtokoll.Awareness(doc);
    this.doc.on("update", this.beiAenderung);
    this.anwesenheit.on("update", this.beiAnwesenheit);
    // Beim Schliessen des Reiters abmelden, damit die anderen den Namen nicht
    // noch eine halbe Minute stehen sehen, bis er von selbst veraltet.
    window.addEventListener("pagehide", this.beimVerlassen);
    this.verbinde();
  }

  /** Meldet sich, wenn sich Verbindung, Abgleich oder Anwesenheit ändern. */
  beiWechsel(fn: () => void): () => void {
    this.horcher.add(fn);
    return () => this.horcher.delete(fn);
  }

  destroy() {
    this.beendet = true;
    window.clearTimeout(this.wecker);
    window.removeEventListener("pagehide", this.beimVerlassen);
    // Abmelden, solange die Horcher noch hängen: das Abmelden IST eine
    // Änderung der Anwesenheit, und nur über den Horcher geht sie hinaus. Erst
    // danach die Leitung abbauen.
    this.abmelden();
    this.doc.off("update", this.beiAenderung);
    this.anwesenheit.off("update", this.beiAnwesenheit);
    this.anwesenheit.destroy();
    this.ws?.close();
    this.ws = null;
    this.horcher.clear();
  }

  private melde() {
    this.horcher.forEach((fn) => fn());
  }

  private verbinde() {
    if (this.beendet) return;
    const ws = new WebSocket(this.adresse);
    ws.binaryType = "arraybuffer";
    this.ws = ws;

    ws.onopen = () => {
      if (this.ws !== ws) return;
      this.verbunden = true;
      this.warte = WARTE_ANFANG;
      // Zwei Dinge zum Gruss: was ich habe, und wer ich bin. Das erste
      // beantworten die anderen mit dem, was ich noch nicht habe.
      const gruss = kodieren.createEncoder();
      kodieren.writeVarUint(gruss, PAKET_ABGLEICH);
      abgleichProtokoll.writeSyncStep1(gruss, this.doc);
      this.sende(kodieren.toUint8Array(gruss));
      this.sendeAnwesenheit([this.doc.clientID]);
      const frage = kodieren.createEncoder();
      kodieren.writeVarUint(frage, PAKET_WER_IST_DA);
      this.sende(kodieren.toUint8Array(frage));
      this.melde();
    };

    ws.onmessage = (e) => {
      if (this.ws !== ws) return;
      this.lies(new Uint8Array(e.data as ArrayBuffer));
    };

    const weg = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      const warVerbunden = this.verbunden;
      this.verbunden = false;
      this.abgeglichen = false;
      // Die anderen aus der Anwesenheit nehmen: ohne Leitung weiss dieser
      // Browser nicht mehr, wer noch da ist, und eine Liste von Namen, die
      // vielleicht stimmt, ist schlimmer als keine.
      anwesenheitProtokoll.removeAwarenessStates(
        this.anwesenheit,
        Array.from(this.anwesenheit.getStates().keys()).filter((k) => k !== this.doc.clientID),
        this,
      );
      if (warVerbunden) this.melde();
      if (this.beendet) return;
      this.wecker = window.setTimeout(() => this.verbinde(), this.warte);
      this.warte = Math.min(this.warte * 2, WARTE_HOECHSTENS);
    };
    ws.onclose = weg;
    ws.onerror = weg;
  }

  private sende(paket: Uint8Array) {
    // Die Enge kommt von TypeScript 7: send() nimmt keine Sicht auf einen
    // SharedArrayBuffer an, und ein blankes Uint8Array laesst offen, auf
    // welcher Art Puffer es sitzt. Hier kommt jedes Paket aus lib0, und das
    // legt gewoehnliche Puffer an -- ein geteilter kann es nicht sein.
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(paket as Uint8Array<ArrayBuffer>);
    }
  }

  private lies(paket: Uint8Array) {
    const leser = dekodieren.createDecoder(paket);
    const art = dekodieren.readVarUint(leser);
    if (art === PAKET_ABGLEICH) {
      const antwort = kodieren.createEncoder();
      kodieren.writeVarUint(antwort, PAKET_ABGLEICH);
      // Die Herkunft ist diese Leitung: daran erkennt der Absender weiter
      // unten, dass er eine fremde Änderung nicht als eigene zurückschicken
      // muss.
      const art2 = abgleichProtokoll.readSyncMessage(leser, antwort, this.doc, this);
      if (kodieren.length(antwort) > 1) this.sende(kodieren.toUint8Array(antwort));
      // Schritt 2 ist die Antwort auf die eigene Frage "was hast du?": ab hier
      // ist dieser Browser auf demselben Stand wie der Raum.
      if (art2 === abgleichProtokoll.messageYjsSyncStep2 && !this.abgeglichen) {
        this.abgeglichen = true;
        this.melde();
      }
    } else if (art === PAKET_ANWESENHEIT) {
      anwesenheitProtokoll.applyAwarenessUpdate(
        this.anwesenheit,
        dekodieren.readVarUint8Array(leser),
        this,
      );
    } else if (art === PAKET_WER_IST_DA) {
      // Jemand ist dazugekommen: alles sagen, was dieser Browser über die
      // Anwesenden weiss. Auch die anderen antworten, das schadet nichts,
      // dieselbe Auskunft zweimal ändert nichts.
      this.sendeAnwesenheit(Array.from(this.anwesenheit.getStates().keys()));
    }
  }

  private beiAenderung = (aenderung: Uint8Array, herkunft: unknown) => {
    // Was von der Leitung kam, geht nicht auf ihr zurück.
    if (herkunft === this) return;
    const paket = kodieren.createEncoder();
    kodieren.writeVarUint(paket, PAKET_ABGLEICH);
    abgleichProtokoll.writeUpdate(paket, aenderung);
    this.sende(kodieren.toUint8Array(paket));
  };

  private beiAnwesenheit = (
    { added, updated, removed }: { added: number[]; updated: number[]; removed: number[] },
    herkunft: unknown,
  ) => {
    if (herkunft !== this) this.sendeAnwesenheit([...added, ...updated, ...removed]);
    this.melde();
  };

  private sendeAnwesenheit(wen: number[]) {
    if (wen.length === 0) return;
    const paket = kodieren.createEncoder();
    kodieren.writeVarUint(paket, PAKET_ANWESENHEIT);
    kodieren.writeVarUint8Array(
      paket,
      anwesenheitProtokoll.encodeAwarenessUpdate(this.anwesenheit, wen),
    );
    this.sende(kodieren.toUint8Array(paket));
  }

  private abmelden() {
    anwesenheitProtokoll.removeAwarenessStates(this.anwesenheit, [this.doc.clientID], "Abgang");
  }

  private beimVerlassen = () => this.abmelden();
}

export interface Anwesend {
  kennung: number;
  name: string;
  farbe: string;
  ichSelbst: boolean;
}

export interface Mitschrift {
  /** Das geteilte Dokument. Hier hängt neben dem Text auch die Marke, dass die
      erste Fassung schon eingetragen wurde. */
  doc: Y.Doc;
  fragment: Y.XmlFragment;
  /** Was BlockNote als Anbieter erwartet: es liest daraus nur die Anwesenheit. */
  provider: { awareness: anwesenheitProtokoll.Awareness };
  user: { name: string; color: string };
  bereit: boolean;
  /** Dieser Browser schreibt in die Datenbank, die anderen nicht. */
  fuehrend: boolean;
  /** Verbindung steht. Steht sie nicht, tippt man vorerst für sich allein. */
  verbunden: boolean;
  anwesend: Anwesend[];
}

// Feste Farben statt gewürfelter: sie stehen am Cursor eines fremden Menschen,
// müssen sich voneinander unterscheiden und auf hellem wie dunklem Grund
// lesbar bleiben. Sechs reichen, mehr als sechs gleichzeitig an einem Absatz
// ist ohnehin keine Arbeitsweise.
const FARBEN = ["#2383e2", "#bf5b04", "#0f7b6c", "#9065b0", "#c1442e", "#4d6ad0"];

export function farbeFuer(kennung: string): string {
  let summe = 0;
  for (let i = 0; i < kennung.length; i++) summe = (summe * 31 + kennung.charCodeAt(i)) >>> 0;
  return FARBEN[summe % FARBEN.length];
}

// Die Adresse der Leitung. Aus der eigenen Herkunft gebaut, damit sie unter
// jedem Namen funktioniert, unter dem die Anwendung erreichbar ist, und ohne
// dass irgendwo eine zweite Adresse gepflegt werden müsste.
function adresseFuer(seiteId: string): string {
  const schema = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${schema}//${window.location.host}/api/echtzeit/${encodeURIComponent(seiteId)}`;
}

/**
 * Öffnet die Sitzung für eine Seite, solange aktiv gilt.
 *
 * Gibt null zurück, wenn nicht gemeinsam geschrieben wird. Der Aufrufer
 * arbeitet dann wie vorher: eigener Text, eigenes Speichern.
 */
export function useMitschrift(
  seiteId: string | undefined,
  aktiv: boolean,
  ich: { id: string; name: string } | null,
): Mitschrift | null {
  const [gespann, setGespann] = useState<{ doc: Y.Doc; leitung: Leitung } | null>(null);
  const [stand, setStand] = useState(0);

  const user = useMemo(
    () => ({ name: ich?.name || "Jemand", color: farbeFuer(ich?.id || "?") }),
    [ich?.id, ich?.name],
  );

  useEffect(() => {
    if (!aktiv || !seiteId || !ich) {
      setGespann(null);
      return;
    }
    const doc = new Y.Doc();
    const leitung = new Leitung(adresseFuer(seiteId), doc);
    // Der eigene Eintrag in der Anwesenheit. BlockNote liest name und color
    // daraus und zeichnet damit die fremden Schreibmarken.
    leitung.anwesenheit.setLocalStateField("user", user);
    const ab = leitung.beiWechsel(() => setStand((n) => n + 1));
    setGespann({ doc, leitung });

    return () => {
      ab();
      leitung.destroy();
      doc.destroy();
      setGespann(null);
    };
    // user hängt an ich und ändert sich nicht während einer Sitzung.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [seiteId, aktiv, ich?.id]);

  return useMemo(() => {
    if (!gespann) return null;
    const { doc, leitung } = gespann;

    const anwesend: Anwesend[] = [];
    leitung.anwesenheit.getStates().forEach((zustand, kennung) => {
      const u = (zustand as { user?: { name?: string; color?: string } }).user;
      anwesend.push({
        kennung,
        name: u?.name || "Jemand",
        farbe: u?.color || "#888888",
        ichSelbst: kennung === doc.clientID,
      });
    });
    anwesend.sort((a, b) => a.kennung - b.kennung);

    // Der Führende ist der mit der kleinsten Kennung. Solange die Anwesenheit
    // noch leer ist, führt niemand: sonst schriebe jeder Browser in der ersten
    // halben Sekunde nach dem Verbinden für sich allein los.
    const kleinste = anwesend.length > 0 ? anwesend[0].kennung : -1;

    return {
      doc,
      fragment: doc.getXmlFragment("document-store"),
      provider: { awareness: leitung.anwesenheit },
      user,
      bereit: leitung.abgeglichen,
      fuehrend: leitung.abgeglichen && kleinste === doc.clientID,
      verbunden: leitung.verbunden,
      anwesend,
    };
    // stand zählt hoch, sobald sich an der Leitung etwas ändert.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [gespann, stand, user]);
}
