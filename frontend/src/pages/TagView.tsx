// Every page carrying one tag.
//
// This view exists because the tags in the sidebar used to be decoration: you
// could create them, attach them and see them, and then nothing. A label you
// cannot follow is worse than no label — it promises an order that is not
// actually reachable.
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router";

import { PageMeta, Tag, api } from "../api/client";
import { useRueckfrage } from "../components/Rueckfrage";

export default function TagView({
  allTags,
  onTagsChange,
}: {
  allTags: Tag[];
  onTagsChange: () => void;
}) {
  const { tagId } = useParams();
  const nav = useNavigate();
  const frageStellen = useRueckfrage();
  const [seiten, setSeiten] = useState<PageMeta[]>([]);
  const [laedt, setLaedt] = useState(true);
  const [fehler, setFehler] = useState<string | null>(null);

  const tag = allTags.find((t) => t.id === tagId);

  const laden = useCallback(() => {
    if (!tagId) return;
    setLaedt(true);
    api
      .seitenZuTag(tagId)
      .then((s) => {
        setSeiten(s);
        setFehler(null);
      })
      .catch((e: Error) => setFehler(e.message))
      .finally(() => setLaedt(false));
  }, [tagId]);

  useEffect(laden, [laden]);

  const loeschen = async () => {
    if (!tagId || !tag) return;
    const frage =
      seiten.length === 0
        ? `Delete the tag \u201c${tag.name}\u201d?`
        : `Delete the tag \u201c${tag.name}\u201d? It is removed from ${seiten.length} pages. ` +
          `The pages themselves remain.`;
    // Here the confirmation is worth it: unlike a deleted comment a tag cannot
    // be restored, and all its assignments are lost with it.
    if (
      !(await frageStellen({
        titel: "Delete tag",
        text: frage,
        bestaetigen: "Delete tag",
        gefaehrlich: true,
      }))
    )
      return;
    await api.deleteTag(tagId).catch(() => {});
    onTagsChange();
    nav("/");
  };

  if (!tagId) return null;

  return (
    <div className="page-pad">
      <div className="tagkopf">
        <h2>
          {tag && <span className="tag-dot gross" style={{ background: tag.color }} />}
          {tag ? tag.name : "Tag"}
        </h2>
        {tag && (
          <button className="link-btn" onClick={loeschen}>
            Schlagwort löschen
          </button>
        )}
      </div>

      {fehler && <div className="fehler">{fehler}</div>}
      {laedt && <div className="muted">Loading…</div>}

      {!laedt && seiten.length === 0 && !fehler && (
        <p className="muted">
          Keine Seite trägt dieses Schlagwort. Vergeben wird es unter dem Titel einer
          Seite.
        </p>
      )}

      {seiten.length > 0 && (
        <>
          <p className="muted small">
            {seiten.length === 1 ? "One page" : `${seiten.length} pages`}, most recently
            geänderte zuerst.
          </p>
          <div className="tagliste">
            {seiten.map((p) => (
              <div key={p.id} className="tree-row" onClick={() => nav(`/page/${p.id}`)}>
                <span className="tree-label">{p.title || "Untitled"}</span>
                {/* A tag can hang on a shared page, and that belongs said,
                    otherwise one wonders why it does not stand in one's own
                    tree. */}
                {p.shared && <span className="pill klein">shared</span>}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
