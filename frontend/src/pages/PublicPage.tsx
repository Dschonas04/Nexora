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

  if (err) return <div className="empty-state">This page is not available.</div>;
  if (!page) return <div className="empty-state">Loading…</div>;

  return (
    <div className="editor-scroll">
      <div className="page">
        <h1 className="page-title" style={{ cursor: "default" }}>
          {page.title || "Untitled"}
        </h1>
        <Editor initialContent={page.content} editable={false} />
      </div>
    </div>
  );
}
