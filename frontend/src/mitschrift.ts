// Writing together on a page, the browser's side of it.
//
// The service only passes packets on, the computing happens here. Yjs merges
// two versions of the same text without one overwriting the other: two people
// in different paragraphs notice nothing of each other, two in the same word
// get a result containing both inputs. That is the difference from saving the
// whole page, where the last one wins.
//
// Writing to the database still goes on all the same, and by exactly one of the
// participants. Who that is is not negotiated but computed: the smallest id in
// the room. Everybody has the same list, so everybody arrives at the same
// person, and when they leave the next one moves up without anybody having to
// announce anything.
import { useEffect, useMemo, useState } from "react";
import * as Y from "yjs";
import * as anwesenheitProtokoll from "y-protocols/awareness";
import * as abgleichProtokoll from "y-protocols/sync";
import * as kodieren from "lib0/encoding";
import * as dekodieren from "lib0/decoding";

// The two kinds of packet that go over the wire. The same numbers as with
// y-websocket: the format is not invented there, only named, and whoever later
// wants to put a ready-made service in front speaks the same language.
const PAKET_ABGLEICH = 0;
const PAKET_ANWESENHEIT = 1;
// "Who is there?" — the question somebody who has just joined asks. Without it
// they would only learn of the others the next time one of them does something,
// and until then would seemingly sit alone on a page three people work on.
const PAKET_WER_IST_DA = 3;

// Waiting times when reconnecting, in milliseconds. Start short, because the
// most frequent case is a restart of the service lasting a few seconds; capped
// at the top, because a tab left open overnight should not knock on a dead
// address every second.
const WARTE_ANFANG = 1000;
const WARTE_HOECHSTENS = 30000;

/**
 * The wire to the other browsers.
 *
 * One of our own instead of a ready-made one: what is needed is exactly this, a
 * connection that carries Yjs packets back and forth and reconnects itself. The
 * usual library for it brings along a whole server plus database, thirty-three
 * packages, one of which wants to be compiled natively at install time.
 */
export class Leitung {
  readonly anwesenheit: anwesenheitProtokoll.Awareness;
  verbunden = false;
  /** The first sync is through: only after it is it certain whether text is already there. */
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
    // Sign off when the tab closes, so the others do not go on seeing the name
    // stand there for another half minute until it goes stale by itself.
    window.addEventListener("pagehide", this.beimVerlassen);
    this.verbinde();
  }

  /** Fires when connection, sync or presence changes. */
  beiWechsel(fn: () => void): () => void {
    this.horcher.add(fn);
    return () => this.horcher.delete(fn);
  }

  destroy() {
    this.beendet = true;
    window.clearTimeout(this.wecker);
    window.removeEventListener("pagehide", this.beimVerlassen);
    // Sign off while the listeners still hang on: signing off IS a change of
    // presence, and only through the listener does it go out. Tear the wire
    // down after that.
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
      // Two things by way of greeting: what I have, and who I am. The others
      // answer the first with whatever I do not have yet.
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
      // Take the others out of the presence: without a wire this browser no
      // longer knows who is still there, and a list of names that may be right
      // is worse than none.
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
      // The origin is this wire: that is how the sender further down
      // recognises that it need not send a foreign change back as its own.
      const art2 = abgleichProtokoll.readSyncMessage(leser, antwort, this.doc, this);
      if (kodieren.length(antwort) > 1) this.sende(kodieren.toUint8Array(antwort));
      // Step 2 is the answer to one's own question "what do you have?": from
      // here on this browser is at the same state as the room.
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
      // Somebody has joined: say everything this browser knows about those
      // present. The others answer too, which does no harm; the same
      // information twice changes nothing.
      this.sendeAnwesenheit(Array.from(this.anwesenheit.getStates().keys()));
    }
  }

  private beiAenderung = (aenderung: Uint8Array, herkunft: unknown) => {
    // What came off the wire does not go back onto it.
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
  /** The shared document. Besides the text, the marker that the first revision
      has already been written hangs here too. */
  doc: Y.Doc;
  fragment: Y.XmlFragment;
  /** What BlockNote expects as a provider: it only reads the presence out of it. */
  provider: { awareness: anwesenheitProtokoll.Awareness };
  user: { name: string; color: string };
  bereit: boolean;
  /** This browser writes to the database, the others do not. */
  fuehrend: boolean;
  /** The connection is up. If it is not, one types for oneself for the time being. */
  verbunden: boolean;
  anwesend: Anwesend[];
}

// Fixed colours instead of rolled ones: they stand at another person's cursor,
// have to be distinguishable from each other and stay readable on a light as
// well as a dark ground. Six are enough; more than six at once on one paragraph
// is no way of working anyway.
const FARBEN = ["#2383e2", "#bf5b04", "#0f7b6c", "#9065b0", "#c1442e", "#4d6ad0"];

export function farbeFuer(kennung: string): string {
  let summe = 0;
  for (let i = 0; i < kennung.length; i++) summe = (summe * 31 + kennung.charCodeAt(i)) >>> 0;
  return FARBEN[summe % FARBEN.length];
}

// The address of the wire. Built from our own origin, so that it works under
// every name the application can be reached under, and without a second
// address having to be maintained anywhere.
function adresseFuer(seiteId: string): string {
  const schema = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${schema}//${window.location.host}/api/echtzeit/${encodeURIComponent(seiteId)}`;
}

/**
 * Opens the session for a page as long as aktiv holds.
 *
 * Returns null when there is no writing together. The caller then works as
 * before: own text, own saving.
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
    // One's own entry in the presence. BlockNote reads name and color out of
    // it and draws the other people's carets with them.
    leitung.anwesenheit.setLocalStateField("user", user);
    const ab = leitung.beiWechsel(() => setStand((n) => n + 1));
    setGespann({ doc, leitung });

    return () => {
      ab();
      leitung.destroy();
      doc.destroy();
      setGespann(null);
    };
    // user hangs on ich and does not change during a session.
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

    // The leader is the one with the smallest id. As long as the presence is
    // still empty, nobody leads: otherwise every browser would start writing
    // for itself alone in the first half second after connecting.
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
    // stand counts up as soon as something changes on the wire.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [gespann, stand, user]);
}
