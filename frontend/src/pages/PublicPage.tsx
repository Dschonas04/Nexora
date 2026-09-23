// Read-only view behind a public link. It is rendered outside the workspace,
// without a sidebar, and reaches the API through the one unauthenticated
// endpoint, so it also works for a visitor with no account.
import { Suspense, lazy, useEffect, useState } from "react";
import { useParams } from "react-router";
import { PublicPage as PublicPageData, api } from "../api/client";
import Fehlergrenze from "../components/Fehlergrenze";

// Loaded on demand as in PageView: BlockNote is the largest piece of the
// bundle, and a public page is often the very first request.
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

  // The name of the page belongs in the browser tab. Without it ten shared
  // pages in ten tabs would all be called the same, namely "Nexora", and
  // nobody would find the one they were looking for again.
  useEffect(() => {
    if (!page) return;
    const vorher = document.title;
    document.title = page.title || "Untitled";
    return () => {
      document.title = vorher;
    };
  }, [page]);

  // A revoked link and a token that never existed look the same on purpose, so
  // the page reveals nothing about what else is in the workspace.
  if (err)
    return <div className="empty-state">This page is not available.</div>;
  if (!page) return <div className="empty-state spaet">Loading…</div>;

  const stand = new Date(page.updatedAt);

  return (
    <div className="oeffentlich">
      {/* A line saying where one stands: this is a single page that has
          been passed on and not a website, and one cannot write along. Without
          it the text stood in an empty window, and a visitor could not tell
          whether they were seeing everything. */}
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
            {page.title || "Untitled"}
          </h1>
          {/* Same editor as inside the app, but read-only, so a public page
              renders exactly like the original. */}
          <Fehlergrenze text="The content of this page could not be shown.">
            <Suspense fallback={<div className="qv-none">Loading…</div>}>
              <Editor
                initialContent={page.content}
                editable={false}
                // A link to another page of this wiki leads nowhere for a
                // visitor -- the page behind it is not shared. It is therefore
                // merely set recognisably and not made clickable: otherwise the
                // square brackets would stand raw in the text as if something
                // were broken.
                linkResolver={() => null}
              />
            </Suspense>
          </Fehlergrenze>
          <div className="oeffentlich-fuss">
            Stand:{" "}
            {stand.toLocaleDateString(undefined, {
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
