// The inbox.
//
// One list, newest first, unread marked. No folders, no filters, no bulk
// selection: an inbox in a wiki is a list of things that happened while you
// were not looking, and the only two questions are "what" and "where", both
// answered by a line and a click.
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Nachricht, api } from "../api/client";
import Profilbild from "../components/Profilbild";

// Der Satz je Art, zweigeteilt: was jemand getan hat, und wie der Seitentitel
// daran anschließt. Vorher stand beides in einem Stück und ergab "hat
// kommentiert auf Seitenname" -- verständlich, aber kein Deutsch, das jemand
// so schreiben würde.
const SATZ: Record<Nachricht["art"], { tat: string; vor: string }> = {
  kommentar: { tat: "hat kommentiert", vor: "in" },
  antwort: { tat: "hat auf deinen Kommentar geantwortet", vor: "in" },
  erwaehnung: { tat: "hat dich erwähnt", vor: "in" },
  freigabe: { tat: "hat eine Seite mit dir geteilt", vor: "" },
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
      // Örtlich mitziehen statt neu zu laden: die Zeile soll sofort gelesen
      // aussehen, und die Liste soll dabei nicht unter der Hand umspringen --
      // besonders nicht, wenn "Nur ungelesene" an ist und die eben angeklickte
      // Zeile sonst unter dem Zeigefinger verschwände.
      setItems((vorher) =>
        vorher.map((m) => (m.id === n.id ? { ...m, gelesenAm: new Date().toISOString() } : m)),
      );
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
  const gelesene = items.length - ungelesen;

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
          {/* Nur zeigen, was auch etwas bewirkt. Ein Knopf, der bei leerem
              Postfach dasteht und beim Drücken nichts tut, sieht defekt aus. */}
          {gelesene > 0 && (
            <button className="btn" onClick={aufraeumen} title="Entfernt nur die gelesenen">
              Gelesene wegräumen
            </button>
          )}
        </div>

        {items.length === 0 ? (
          <div className="postfach-leer">
            <div className="postfach-leer-zeichen" aria-hidden="true">
              &#9993;
            </div>
            {nurUngelesen ? (
              <>
                <strong>Nichts Ungelesenes.</strong>
                <p className="muted small">
                  Alles gelesen. Der Schalter oben zeigt wieder die ganze Liste.
                </p>
              </>
            ) : (
              <>
                <strong>Das Postfach ist leer.</strong>
                <p className="muted small">
                  Hier landet, was geschieht, während du nicht hinsiehst.
                </p>
              </>
            )}
          </div>
        ) : (
          <div className="list">
            {items.map((n) => (
              <div
                key={n.id}
                className={
                  "list-row postfach-zeile" +
                  (n.gelesenAm ? "" : " ungelesen") +
                  (n.pageId ? "" : " ohne-ziel")
                }
                role="button"
                tabIndex={0}
                onClick={() => oeffnen(n)}
                onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && oeffnen(n)}
              >
                <span className="postfach-punkt" aria-hidden="true" />
                <Profilbild
                  id={n.ausloeserId ?? n.id}
                  name={n.ausloeserName || "Jemand"}
                  stand={n.ausloeserBild}
                />
                <span className="postfach-inhalt">
                  <span className="postfach-kopf">
                    <strong>{n.ausloeserName || "Jemand"}</strong> {SATZ[n.art].tat}
                    {/* Ohne Titel keine leere unterstrichene Lücke: eine Seite,
                        die inzwischen fort ist, wird benannt und nicht
                        verschwiegen. */}
                    {n.seitenTitel ? (
                      <>
                        {SATZ[n.art].vor ? " " + SATZ[n.art].vor : ""}{" "}
                        <span className="postfach-seite">{n.seitenTitel}</span>
                      </>
                    ) : (
                      <span className="muted"> &mdash; die Seite gibt es nicht mehr</span>
                    )}
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

// wannText says how long ago it was. "vor 3 Stunden" answers the question one
// asks while reading; a date with a clock time would have to be converted
// first.
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
