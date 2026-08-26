// The comment thread under a page.
//
// Two levels only: a comment and its replies. Deeper nesting reads badly and
// covers nothing people actually do — the backend flattens anything deeper.
import { useEffect, useState } from "react";

import { Kommentar, api } from "../api/client";
import { useAuth } from "../auth";

function zeit(iso: string): string {
  const d = new Date(iso);
  const heute = new Date();
  const gleicherTag = d.toDateString() === heute.toDateString();
  return gleicherTag
    ? d.toLocaleTimeString("de-DE", { hour: "2-digit", minute: "2-digit" })
    : d.toLocaleString("de-DE", { day: "2-digit", month: "2-digit", year: "2-digit", hour: "2-digit", minute: "2-digit" });
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
          <textarea value={bearbeitText} onChange={(e) => setBearbeitText(e.target.value)} rows={3} />
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
        <div className="kommentar-text">{k.text}</div>
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
          <textarea
            autoFocus
            rows={2}
            placeholder={"Antwort an " + (k.autorName || "…")}
            value={antwortText}
            onChange={(e) => setAntwortText(e.target.value)}
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
        <textarea
          rows={3}
          placeholder={user ? "Kommentar schreiben… (@Name benachrichtigt jemanden)" : "Anmelden, um zu kommentieren"}
          value={neu}
          onChange={(e) => setNeu(e.target.value)}
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
