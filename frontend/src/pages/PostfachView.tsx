// The inbox.
//
// One list, newest first, unread marked. No folders, no filters, no bulk
// selection: an inbox in a wiki is a list of things that happened while you
// were not looking, and the only two questions are "what" and "where" -- both
// answered by a line and a click.
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Nachricht, api } from "../api/client";

// Der Satz je Art. Der Name des Auslösers steht davor, deshalb fängt jeder
// Satz mit einem Verb an.
const SATZ: Record<Nachricht["art"], string> = {
  kommentar: "hat kommentiert auf",
  antwort: "hat auf deinen Kommentar geantwortet in",
  erwaehnung: "hat dich erwähnt in",
  freigabe: "hat eine Seite mit dir geteilt:",
};

export default function PostfachView({ onGelesen }: { onGelesen: () => void }) {
  const nav = useNavigate();
  const [items, setItems] = useState<Nachricht[]>([]);
  const [nurUngelesen, setNurUngelesen] = useState(false);

  const laden = useCallback(() => {
    api
      .postfach(nurUngelesen)
      .then(setItems)
      .catch(() => setItems([]));
  }, [nurUngelesen]);

  useEffect(() => {
    laden();
  }, [laden]);

  const oeffnen = async (n: Nachricht) => {
    if (!n.gelesenAm) {
      await api.postfachGelesen(n.id).catch(() => {});
      onGelesen();
    }
    if (n.pageId) nav(`/page/${n.pageId}`);
  };

  const alleGelesen = async () => {
    await api.postfachGelesen().catch(() => {});
    laden();
    onGelesen();
  };

  const aufraeumen = async () => {
    await api.postfachLeeren().catch(() => {});
    laden();
  };

  const ungelesen = items.filter((n) => !n.gelesenAm).length;

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Postfach</h1>
        <p className="muted">
          Kommentare auf deinen Seiten, Antworten auf deine Kommentare, Erwähnungen deines
          Namens und Seiten, die jemand mit dir geteilt hat.
        </p>

        <div className="postfach-leiste">
          <label className="postfach-schalter">
            <input
              type="checkbox"
              checked={nurUngelesen}
              onChange={(e) => setNurUngelesen(e.target.checked)}
            />
            Nur ungelesene
          </label>
          <span className="wachsen" />
          {ungelesen > 0 && (
            <button className="btn" onClick={alleGelesen}>
              Alle als gelesen
            </button>
          )}
          <button className="btn" onClick={aufraeumen} title="Entfernt nur die gelesenen">
            Gelesene wegräumen
          </button>
        </div>

        {items.length === 0 ? (
          <div className="muted" style={{ marginTop: 20 }}>
            {nurUngelesen ? "Nichts Ungelesenes." : "Das Postfach ist leer."}
          </div>
        ) : (
          <div className="list">
            {items.map((n) => (
              <div
                key={n.id}
                className={"list-row postfach-zeile" + (n.gelesenAm ? "" : " ungelesen")}
                onClick={() => oeffnen(n)}
              >
                <span className="postfach-punkt" aria-hidden="true" />
                <span className="postfach-inhalt">
                  <span className="postfach-kopf">
                    <strong>{n.ausloeserName || "Jemand"}</strong> {SATZ[n.art]}{" "}
                    <span className="postfach-seite">{n.seitenTitel}</span>
                  </span>
                  {n.text && <span className="muted small postfach-auszug">{n.text}</span>}
                </span>
                <span className="muted small einzeilig">{wannText(n.erstelltAm)}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// wannText sagt, wie lange es her ist. "vor 3 Stunden" beantwortet die Frage,
// die man beim Lesen stellt; ein Datum mit Uhrzeit müsste man erst umrechnen.
function wannText(zeitpunkt: string): string {
  const min = Math.floor((Date.now() - new Date(zeitpunkt).getTime()) / 60000);
  if (min < 1) return "gerade eben";
  if (min < 60) return `vor ${min} min`;
  const std = Math.floor(min / 60);
  if (std < 24) return `vor ${std} ${std === 1 ? "Stunde" : "Stunden"}`;
  const tage = Math.floor(std / 24);
  if (tage < 30) return `vor ${tage} ${tage === 1 ? "Tag" : "Tagen"}`;
  return new Date(zeitpunkt).toLocaleDateString("de-DE");
}
