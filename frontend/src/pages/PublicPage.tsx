// Read-only view behind a public link. It is rendered outside the workspace,
// without a sidebar, and reaches the API through the one unauthenticated
// endpoint, so it also works for a visitor with no account.
import { Suspense, lazy, useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { PublicPage as PublicPageData, api } from "../api/client";
import Fehlergrenze from "../components/Fehlergrenze";

// Wie in PageView nachgeladen: BlockNote ist das groesste Stueck des
// Buendels, und eine oeffentliche Seite ist oft der erste Aufruf ueberhaupt.
const Editor = lazy(() => import("../components/Editor"));

export default function PublicPage() {
  const { token } = useParams();
  const [page, setPage] = useState<PublicPageData | null>(null);
  const [err, setErr] = useState(false);

  useEffect(() => {
    if (!token) return;
    api
      .getPublicPage(token)
      .then(setPage)
      .catch(() => setErr(true));
  }, [token]);

  // Der Name der Seite gehört in den Reiter des Browsers. Ohne das hießen zehn
  // geteilte Seiten in zehn Reitern alle gleich, nämlich "Nexora", und niemand
  // fand die wieder, die er suchte.
  useEffect(() => {
    if (!page) return;
    const vorher = document.title;
    document.title = page.title || "Ohne Titel";
    return () => {
      document.title = vorher;
    };
  }, [page]);

  // A revoked link and a token that never existed look the same on purpose, so
  // the page reveals nothing about what else is in the workspace.
  if (err)
    return <div className="empty-state">Diese Seite ist nicht verfügbar.</div>;
  if (!page) return <div className="empty-state spaet">Lädt…</div>;

  const stand = new Date(page.updatedAt);

  return (
    <div className="oeffentlich">
      {/* Eine Zeile, die sagt, woran man ist: das hier ist eine einzelne,
          weitergegebene Seite und keine Web-Seite, und mitschreiben kann man
          nicht. Ohne sie stand der Text im leeren Fenster, und ein Besucher
          sah ihm nicht an, ob er alles sieht. */}
      <div className="oeffentlich-kopf">
        <span className="oeffentlich-marke">Nexora</span>
        <span className="oeffentlich-hinweis">
          Geteilte Seite, nur zum Lesen
        </span>
      </div>

      <div className="editor-scroll">
        <div
          className={
            "page" +
            (page.breite && page.breite !== "normal" ? " " + page.breite : "")
          }
        >
          <h1 className="page-title" style={{ cursor: "default" }}>
            {page.icon && (
              <span className="oeffentlich-symbol">{page.icon}</span>
            )}
            {page.title || "Ohne Titel"}
          </h1>
          {/* Same editor as inside the app, but read-only, so a public page
              renders exactly like the original. */}
          <Fehlergrenze text="Der Inhalt dieser Seite liess sich nicht anzeigen.">
            <Suspense fallback={<div className="qv-none">Wird geladen…</div>}>
              <Editor
                initialContent={page.content}
                editable={false}
                // Ein Verweis auf eine andere Seite dieses Wikis führt für einen
                // Besucher nirgendwohin -- die Seite dahinter ist nicht geteilt.
                // Er wird darum bloß erkennbar gesetzt, nicht anklickbar: sonst
                // stünden im Text die eckigen Klammern roh da, als wäre etwas
                // kaputt.
                linkResolver={() => null}
              />
            </Suspense>
          </Fehlergrenze>
          <div className="oeffentlich-fuss">
            Stand:{" "}
            {stand.toLocaleDateString("de-DE", {
              day: "2-digit",
              month: "long",
              year: "numeric",
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
