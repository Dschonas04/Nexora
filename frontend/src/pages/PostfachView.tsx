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
  kommentar: { tat: "commented", vor: "on" },
  antwort: { tat: "replied to your comment", vor: "on" },
  erwaehnung: { tat: "mentioned you", vor: "on" },
  freigabe: { tat: "shared a page with you", vor: "" },
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
        <h1 className="view-title">Inbox</h1>
        <p className="muted">
          Comments on your pages, replies to your comments, mentions of your name
          and pages someone shared with you.
        </p>

        <div className="postfach-leiste">
          <label className="postfach-schalter">
            <input
              type="checkbox"
              checked={nurUngelesen}
              onChange={(e) => setNurUngelesen(e.target.checked)}
            />
            Unread only
          </label>
          <span className="wachsen" />
          {ungelesen > 0 && (
            <button className="btn" onClick={alleGelesen}>
              Mark all as read
            </button>
          )}
          {/* Nur zeigen, was auch etwas bewirkt. Ein Knopf, der bei leerem
              Postfach dasteht und beim Drücken nichts tut, sieht defekt aus. */}
          {gelesene > 0 && (
            <button className="btn" onClick={aufraeumen} title="Removes only what you have read">
              Clear the read ones
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
                <strong>Nothing unread.</strong>
                <p className="muted small">
                  Alles gelesen. Der Schalter oben zeigt wieder die ganze Liste.
                </p>
              </>
            ) : (
              <>
                <strong>The inbox is empty.</strong>
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
                  name={n.ausloeserName || "Someone"}
                  stand={n.ausloeserBild}
                />
                <span className="postfach-inhalt">
                  <span className="postfach-kopf">
                    <strong>{n.ausloeserName || "Someone"}</strong> {SATZ[n.art].tat}
                    {/* Ohne Titel keine leere unterstrichene Lücke: eine Seite,
                        die inzwischen fort ist, wird benannt und nicht
                        verschwiegen. */}
                    {n.seitenTitel ? (
                      <>
                        {SATZ[n.art].vor ? " " + SATZ[n.art].vor : ""}{" "}
                        <span className="postfach-seite">{n.seitenTitel}</span>
                      </>
                    ) : (
                      <span className="muted"> &mdash; that page no longer exists</span>
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
  if (min < 1) return "just now";
  if (min < 60) return `${min} min ago`;
  const std = Math.floor(min / 60);
  if (std < 24) return `${std} ${std === 1 ? "hour" : "hours"} ago`;
  const tage = Math.floor(std / 24);
  if (tage < 30) return `${tage} ${tage === 1 ? "day" : "days"} ago`;
  return new Date(zeitpunkt).toLocaleDateString();
}
