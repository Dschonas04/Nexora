// Read-only view behind a public link. It is rendered outside the workspace,
// without a sidebar, and reaches the API through the one unauthenticated
// endpoint, so it also works for a visitor with no account.
import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { PublicPage as PublicPageData, api } from "../api/client";
import Editor from "../components/Editor";

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

  // A revoked link and a token that never existed look the same on purpose, so
  // the page reveals nothing about what else is in the workspace.
  if (err) return <div className="empty-state">Diese Seite ist nicht verfügbar.</div>;
  if (!page) return <div className="empty-state">Lädt…</div>;

  return (
    <div className="editor-scroll">
      <div className="page">
        <h1 className="page-title" style={{ cursor: "default" }}>
          {page.title || "Ohne Titel"}
        </h1>
        {/* Same editor as inside the app, but read-only, so a public page
            renders exactly like the original. */}
        <Editor initialContent={page.content} editable={false} />
      </div>
    </div>
  );
}
