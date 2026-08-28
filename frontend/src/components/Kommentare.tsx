// The comment thread under a page.
//
// Two levels only: a comment and its replies. Deeper nesting reads badly and
// covers nothing people actually do — the backend flattens anything deeper.
import { useEffect, useRef, useState } from "react";

import { Kommentar, Person, api } from "../api/client";
import { useAuth } from "../auth";

function zeit(iso: string): string {
  const d = new Date(iso);
  const heute = new Date();
  const gleicherTag = d.toDateString() === heute.toDateString();
  return gleicherTag
    ? d.toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" })
    : d.toLocaleString("de-DE", { day: "2-digit", month: "2-digit", year: "2-digit", hour: "2-digit", minute: "2-digit" });
}

// Was hinter einem @ steht, bis zum Schreibzeiger. Der Name eines Kontos darf
// Leerzeichen tragen ("Anna Schmidt"), darum endet die Suche nicht am ersten
// Wort; ein zweites @ und ein Zeilenwechsel beenden sie sehr wohl.
const ERWAEHNUNG = /@([^\n@]{0,40})$/;

// Erwaehnfeld ist ein Textfeld, das beim Tippen von @ die Namen anbietet.
//
// Ohne die Liste musste man den Namen eines Kontos auf den Buchstaben genau
// treffen, sonst ging die Erwähnung ins Leere -- und zwar still: der Kommentar
// stand da, benachrichtigt wurde niemand. Wer die Namen der Kollegen nicht
// auswendig kann, konnte die Funktion nicht benutzen.
function Erwaehnfeld({
  wert,
  setzen,
  personen,
  zeilen,
  platzhalter,
  autoFocus,
}: {
  wert: string;
  setzen: (t: string) => void;
  personen: Person[];
  zeilen: number;
  platzhalter?: string;
  autoFocus?: boolean;
}) {
  const feld = useRef<HTMLTextAreaElement>(null);
  // Die Suche ist null, solange keine offen ist. Der Unterschied zur leeren
  // Zeichenkette zählt: direkt hinter einem @ steht sie leer, und dann sollen
  // alle Namen dastehen.
  const [suche, setSuche] = useState<string | null>(null);
  const [gewaehlt, setGewaehlt] = useState(0);

  const treffer =
    suche === null
      ? []
      : personen.filter((p) => p.name.toLowerCase().includes(suche.toLowerCase())).slice(0, 8);

  // Prüft nach jeder Änderung und nach jedem Sprung des Schreibzeigers, ob vor
  // ihm eine angefangene Erwähnung steht.
  const pruefen = (text: string, stelle: number) => {
    const treffer = ERWAEHNUNG.exec(text.slice(0, stelle));
    setSuche(treffer ? treffer[1] : null);
    setGewaehlt(0);
  };

  const einsetzen = (name: string) => {
    const el = feld.current;
    if (!el) return;
    const stelle = el.selectionStart ?? wert.length;
    const davor = wert.slice(0, stelle);
    const gefunden = ERWAEHNUNG.exec(davor);
    if (!gefunden) return;
    const neu = davor.slice(0, gefunden.index) + "@" + name + " " + wert.slice(stelle);
    setzen(neu);
    setSuche(null);
    // Der Schreibzeiger gehört hinter den eingesetzten Namen, nicht ans Ende
    // des Textes: geschrieben wird oft mitten im Satz.
    const hinter = gefunden.index + name.length + 2;
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(hinter, hinter);
    });
  };

  return (
    <div className="erwaehnfeld">
      <textarea
        ref={feld}
        rows={zeilen}
        autoFocus={autoFocus}
        placeholder={platzhalter}
        value={wert}
        onChange={(e) => {
          setzen(e.target.value);
          pruefen(e.target.value, e.target.selectionStart ?? 0);
        }}
        onClick={(e) => pruefen(wert, e.currentTarget.selectionStart ?? 0)}
        onBlur={() => {
          // Verzögert, sonst wäre die Liste weg, ehe der Klick auf einen Namen
          // ankommt: der Verlust des Fokus geht dem Klick voraus.
          window.setTimeout(() => setSuche(null), 150);
        }}
        onKeyDown={(e) => {
          if (treffer.length === 0) return;
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setGewaehlt((n) => (n + 1) % treffer.length);
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setGewaehlt((n) => (n - 1 + treffer.length) % treffer.length);
          } else if (e.key === "Enter" || e.key === "Tab") {
            e.preventDefault();
            einsetzen(treffer[gewaehlt].name);
          } else if (e.key === "Escape") {
            setSuche(null);
          }
        }}
      />
      {treffer.length > 0 && (
        <div className="erwaehnliste">
          {treffer.map((p, i) => (
            <button
              key={p.name}
              // hervor und nicht gewaehlt: der Haken der anderen Listen sagt
              // "das ist der gesetzte Wert", hier heißt es bloß "hier steht
              // gerade der Zeiger".
              className={"vorlageneintrag" + (i === gewaehlt ? " hervor" : "")}
              // onMouseDown und nicht onClick: der Klick käme erst, wenn das
              // Feld den Fokus schon verloren hat und die Liste zu ist.
              onMouseDown={(e) => {
                e.preventDefault();
                einsetzen(p.name);
              }}
            >
              {p.name}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// mitErwaehnungen setzt die Namen im Text hervor, die wirklich zu einem Konto
// gehören. Damit sieht man einem abgeschickten Kommentar an, ob die Erwähnung
// angekommen ist -- vorher war ein Tippfehler im Namen von einer richtigen
// Erwähnung nicht zu unterscheiden.
function mitErwaehnungen(text: string, personen: Person[]) {
  if (personen.length === 0 || !text.includes("@")) return text;
  // Die längsten Namen zuerst: sonst nähme "@Anna" den Anfang von
  // "@Anna Schmidt" weg und der Nachname bliebe als gewöhnlicher Text stehen.
  const namen = [...personen].sort((a, b) => b.name.length - a.name.length);

  const stuecke: (string | { name: string })[] = [];
  let rest = text;
  while (rest.length > 0) {
    const at = rest.indexOf("@");
    if (at < 0) {
      stuecke.push(rest);
      break;
    }
    const name = namen.find((p) =>
      rest.slice(at + 1).toLowerCase().startsWith(p.name.toLowerCase()),
    );
    if (!name) {
      stuecke.push(rest.slice(0, at + 1));
      rest = rest.slice(at + 1);
      continue;
    }
    if (at > 0) stuecke.push(rest.slice(0, at));
    stuecke.push({ name: name.name });
    rest = rest.slice(at + 1 + name.name.length);
  }

  return stuecke.map((st, i) =>
    typeof st === "string" ? (
      <span key={i}>{st}</span>
    ) : (
      <span key={i} className="erwaehnung">
        @{st.name}
      </span>
    ),
  );
}

export default function Kommentare({ pageId }: { pageId: string }) {
  const { user } = useAuth();
  const [alle, setAlle] = useState<Kommentar[]>([]);
  const [neu, setNeu] = useState("");
  const [antwortAuf, setAntwortAuf] = useState<string | null>(null);
  const [antwortText, setAntwortText] = useState("");
  const [bearbeitet, setBearbeitet] = useState<string | null>(null);
  const [bearbeitText, setBearbeitText] = useState("");
  const [erledigteZeigen, setErledigteZeigen] = useState(false);
  const [fehler, setFehler] = useState<string | null>(null);
  // Wen man hier ansprechen kann. Bleibt die Liste leer -- kein Recht, keine
  // Antwort --, verhält sich das Feld wie ein gewöhnliches Textfeld.
  const [personen, setPersonen] = useState<Person[]>([]);

  const laden = () =>
    api
      .kommentare(pageId)
      .then((k) => {
        setAlle(k);
        setFehler(null);
      })
      .catch(() => setAlle([]));

  useEffect(() => {
    setAntwortAuf(null);
    setBearbeitet(null);
    setNeu("");
    laden();
    api
      .erwaehnbare(pageId)
      .then(setPersonen)
      .catch(() => setPersonen([]));
  }, [pageId]);

  const anlegen = async (text: string, eltern?: string) => {
    const t = text.trim();
    if (!t) return;
    try {
      await api.kommentarAnlegen(pageId, t, eltern);
      setNeu("");
      setAntwortText("");
      setAntwortAuf(null);
      laden();
    } catch (e) {
      setFehler((e as Error).message);
    }
  };

  const speichern = async (id: string) => {
    const t = bearbeitText.trim();
    if (!t) return;
    await api.kommentarAendern(id, t).catch(() => {});
    setBearbeitet(null);
    laden();
  };

  const loeschen = async (id: string) => {
    // No window.confirm: the comment stays in the thread as a shell and could be
    // reconstructed from the audit trail if need be. A confirmation for every
    // click would be more of a nuisance than the damage.
    await api.kommentarLoeschen(id).catch(() => {});
    laden();
  };

  const faeden = alle.filter((k) => !k.elternId);
  const antwortenZu = (id: string) => alle.filter((k) => k.elternId === id);

  const sichtbareFaeden = erledigteZeigen ? faeden : faeden.filter((k) => !k.erledigt);
  const erledigteAnzahl = faeden.filter((k) => k.erledigt).length;

  const zeile = (k: Kommentar, istAntwort: boolean) => (
    <div key={k.id} className={"kommentar" + (istAntwort ? " antwort" : "") + (k.erledigt ? " erledigt" : "")}>
      <div className="kommentar-kopf">
        <span className="kommentar-autor">{k.autorName || "Unbekannt"}</span>
        <span className="muted small">{zeit(k.erstelltAm)}</span>
        {k.geaendertAm && <span className="muted small">bearbeitet</span>}
        {k.erledigt && !istAntwort && <span className="pill klein">erledigt</span>}
      </div>

      {k.geloescht ? (
        <div className="kommentar-text muted">
          <em>Kommentar gelöscht</em>
        </div>
      ) : bearbeitet === k.id ? (
        <div className="kommentar-eingabe">
          <Erwaehnfeld wert={bearbeitText} setzen={setBearbeitText} personen={personen} zeilen={3} />
          <div className="kommentar-aktionen">
            <button className="btn" onClick={() => speichern(k.id)}>
              Speichern
            </button>
            <button className="btn" onClick={() => setBearbeitet(null)}>
              Abbrechen
            </button>
          </div>
        </div>
      ) : (
        <div className="kommentar-text">{mitErwaehnungen(k.text, personen)}</div>
      )}

      {!k.geloescht && bearbeitet !== k.id && (
        <div className="kommentar-aktionen">
          {!istAntwort && (
            <button className="link-btn" onClick={() => { setAntwortAuf(k.id); setAntwortText(""); }}>
              Antworten
            </button>
          )}
          {k.darf && (
            <>
              <button className="link-btn" onClick={() => { setBearbeitet(k.id); setBearbeitText(k.text); }}>
                Bearbeiten
              </button>
              <button className="link-btn" onClick={() => loeschen(k.id)}>
                Löschen
              </button>
            </>
          )}
          {!istAntwort && k.darf && (
            <button
              className="link-btn"
              onClick={() => api.kommentarErledigt(k.id).then(laden).catch(() => {})}
            >
              {k.erledigt ? "Wieder öffnen" : "Erledigt"}
            </button>
          )}
        </div>
      )}

      {antwortAuf === k.id && (
        <div className="kommentar-eingabe antwort">
          <Erwaehnfeld
            autoFocus
            zeilen={2}
            platzhalter={"Antwort an " + (k.autorName || "…")}
            wert={antwortText}
            setzen={setAntwortText}
            personen={personen}
          />
          <div className="kommentar-aktionen">
            <button className="btn" onClick={() => anlegen(antwortText, k.id)}>
              Antworten
            </button>
            <button className="btn" onClick={() => setAntwortAuf(null)}>
              Abbrechen
            </button>
          </div>
        </div>
      )}

      {antwortenZu(k.id).map((a) => zeile(a, true))}
    </div>
  );

  return (
    <div className="kommentare">
      <div className="kommentare-kopf">
        <h3>Kommentare</h3>
        {erledigteAnzahl > 0 && (
          <button className="link-btn" onClick={() => setErledigteZeigen((v) => !v)}>
            {erledigteZeigen ? "Erledigte ausblenden" : `${erledigteAnzahl} erledigte einblenden`}
          </button>
        )}
      </div>

      {fehler && <div className="fehler">{fehler}</div>}

      <div className="kommentar-eingabe">
        <Erwaehnfeld
          zeilen={3}
          platzhalter={
            user
              ? personen.length > 0
                ? "Kommentar schreiben… (@ benachrichtigt jemanden)"
                : "Kommentar schreiben…"
              : "Anmelden, um zu kommentieren"
          }
          wert={neu}
          setzen={setNeu}
          personen={personen}
        />
        <div className="kommentar-aktionen">
          <button className="btn" disabled={!neu.trim()} onClick={() => anlegen(neu)}>
            Kommentieren
          </button>
        </div>
      </div>

      {sichtbareFaeden.length === 0 && (
        <div className="muted small">Noch keine Kommentare.</div>
      )}
      {sichtbareFaeden.map((k) => zeile(k, false))}
    </div>
  );
}
