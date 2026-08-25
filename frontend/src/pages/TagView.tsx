// Every page carrying one tag.
//
// This view exists because the tags in the sidebar used to be decoration: you
// could create them, attach them and see them, and then nothing. A label you
// cannot follow is worse than no label — it promises an order that is not
// actually reachable.
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

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
        ? `Schlagwort „${tag.name}“ löschen?`
        : `Schlagwort „${tag.name}“ löschen? Es wird von ${seiten.length} Seiten entfernt. ` +
          `Die Seiten selbst bleiben.`;
    // Hier lohnt die Rückfrage: anders als ein gelöschter Kommentar lässt sich
    // ein Schlagwort nicht wiederherstellen, und mit ihm gehen alle
    // Zuordnungen verloren.
    if (
      !(await frageStellen({
        titel: "Schlagwort löschen",
        text: frage,
        bestaetigen: "Schlagwort löschen",
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
          {tag ? tag.name : "Schlagwort"}
        </h2>
        {tag && (
          <button className="link-btn" onClick={loeschen}>
            Schlagwort löschen
          </button>
        )}
      </div>

      {fehler && <div className="fehler">{fehler}</div>}
      {laedt && <div className="muted">Lädt…</div>}

      {!laedt && seiten.length === 0 && !fehler && (
        <p className="muted">
          Keine Seite trägt dieses Schlagwort. Vergeben wird es unter dem Titel einer
          Seite.
        </p>
      )}

      {seiten.length > 0 && (
        <>
          <p className="muted small">
            {seiten.length === 1 ? "Eine Seite" : `${seiten.length} Seiten`}, zuletzt
            geänderte zuerst.
          </p>
          <div className="tagliste">
            {seiten.map((p) => (
              <div key={p.id} className="tree-row" onClick={() => nav(`/page/${p.id}`)}>
                <span className="tree-label">{p.title || "Ohne Titel"}</span>
                {/* Ein Schlagwort kann an einer geteilten Seite hängen, das
                    gehört dazugesagt, sonst wundert man sich, warum sie nicht
                    im eigenen Baum steht. */}
                {p.shared && <span className="pill klein">geteilt</span>}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
