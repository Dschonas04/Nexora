// Side panel listing a page's history. The list carries metadata only; the
// content of a version is fetched when one is compared or restored.
import { useEffect, useState } from "react";
import { Page, PageVersion, api } from "../api/client";
import { useRueckfrage } from "./Rueckfrage";
import { Zeile, vergleiche, zaehle } from "../vergleich";

interface Props {
  pageId: string;
  canEdit: boolean;
  // The page as it stands now, read from the editor when the comparison is
  // opened -- not the state loaded with the page, which may be minutes old.
  aktuell: () => { title: string; content: unknown };
  onRestored: (page: Page) => void;
  onClose: () => void;
}

export default function VersionPanel({ pageId, canEdit, aktuell, onRestored, onClose }: Props) {
  const frage = useRueckfrage();
  const [versions, setVersions] = useState<PageVersion[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [vergleich, setVergleich] = useState<number | null>(null);

  useEffect(() => {
    api.listVersions(pageId).then(setVersions).catch(() => setVersions([]));
  }, [pageId]);

  // Restoring is safe because the backend snapshots the current state first;
  // the confirmation explains this so nobody fears losing their work.
  const restore = async (versionId: string) => {
    if (
      !(await frage({
        titel: "Restore version",
        text: "This version is brought back onto the page. The current state is not lost; it is filed in the history first.",
        bestaetigen: "Restore",
      }))
    )
      return;
    setBusy(versionId);
    try {
      const page = await api.restoreVersion(pageId, versionId);
      onRestored(page);
    } finally {
      setBusy(null);
    }
  };

  const fmt = (iso: string) => new Date(iso).toLocaleString();

  return (
    <div className="side-panel">
      <div className="side-panel-header">
        <h3>Version history</h3>
        <button className="icon-btn" onClick={onClose}>
          ✕
        </button>
      </div>
      <div className="side-panel-body">
        {versions.length === 0 && <div className="muted small">No earlier versions yet.</div>}
        {versions.map((v, i) => (
          <div key={v.id} className="version-row">
            <div>
              <div className="version-title">{v.title || "Untitled"}</div>
              <div className="muted small">
                {fmt(v.createdAt)} · {v.authorName}
              </div>
            </div>
            <div className="version-knoepfe">
              <button className="btn" onClick={() => setVergleich(i)} title="Show what changed since this version">
                Compare
              </button>
              {/* Read-only viewers see the history but cannot roll the page back. */}
              {canEdit && (
                <button className="btn" disabled={busy === v.id} onClick={() => restore(v.id)}>
                  {busy === v.id ? "…" : "Restore"}
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
      {vergleich !== null && versions[vergleich] && (
        <VersionVergleich
          pageId={pageId}
          versionen={versions}
          index={vergleich}
          aktuell={aktuell}
          onClose={() => setVergleich(null)}
        />
      )}
    </div>
  );
}

// The comparison of one version with the page as it is now, or with the
// version that followed it. "Since then" is the question one usually asks
// ("what did they change?"); "next version" answers "what happened in this one
// step?".
function VersionVergleich({
  pageId,
  versionen,
  index,
  aktuell,
  onClose,
}: {
  pageId: string;
  versionen: PageVersion[];
  index: number;
  aktuell: () => { title: string; content: unknown };
  onClose: () => void;
}) {
  const [gegen, setGegen] = useState<"jetzt" | "naechste">("jetzt");
  const [zeilen, setZeilen] = useState<Zeile[] | null>(null);
  const [titel, setTitel] = useState<{ alt: string; neu: string } | null>(null);
  const [nurAenderungen, setNurAenderungen] = useState(true);
  const v = versionen[index];
  // The list is newest first: the version that followed this one stands
  // above it.
  const naechste = index > 0 ? versionen[index - 1] : null;

  useEffect(() => {
    let weg = false;
    setZeilen(null);
    (async () => {
      const alt = await api.getVersion(pageId, v.id);
      const neu = gegen === "naechste" && naechste ? await api.getVersion(pageId, naechste.id) : aktuell();
      if (weg) return;
      setZeilen(vergleiche(alt.content, neu.content));
      setTitel({ alt: alt.title, neu: neu.title });
    })().catch(() => !weg && setZeilen([]));
    return () => {
      weg = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId, v.id, gegen]);

  const summe = zeilen ? zaehle(zeilen) : null;
  // Unchanged lines far from any change are folded away; three lines of
  // context on either side say where a change sits.
  const sichtbar = (i: number) => {
    if (!zeilen || !nurAenderungen) return true;
    for (let k = Math.max(0, i - 3); k <= Math.min(zeilen.length - 1, i + 3); k++) {
      if (zeilen[k].art !== "gleich") return true;
    }
    return false;
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal vergleich" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Compare versions</h3>
          <button className="icon-btn" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <div className="modal-section vergleich-kopf">
          <span className="muted small">
            {new Date(v.createdAt).toLocaleString()} · {v.authorName}
          </span>
          <span className="muted small">→</span>
          <select value={gegen} onChange={(e) => setGegen(e.target.value as "jetzt" | "naechste")}>
            <option value="jetzt">current state</option>
            {naechste && <option value="naechste">next version</option>}
          </select>
          <span className="wachsen" />
          <label className="postfach-schalter">
            <input type="checkbox" checked={nurAenderungen} onChange={(e) => setNurAenderungen(e.target.checked)} />
            Only changes
          </label>
        </div>
        <div className="vergleich-koerper notranslate">
          {zeilen === null && <div className="muted small">Loading…</div>}
          {titel && titel.alt !== titel.neu && (
            <div className="vergleich-zeile geaendert">
              <span className="vergleich-marke">T</span>
              <span>
                <del>{titel.alt || "Untitled"}</del> <ins>{titel.neu || "Untitled"}</ins>
              </span>
            </div>
          )}
          {zeilen && summe && summe.neu + summe.weg + summe.geaendert === 0 && (!titel || titel.alt === titel.neu) && (
            <div className="muted small">No differences in the text.</div>
          )}
          {zeilen?.map((z, i) => {
            if (!sichtbar(i)) {
              // One marker for a whole run of folded lines.
              return sichtbar(i - 1) ? (
                <div key={i} className="vergleich-luecke">
                  ⋯
                </div>
              ) : null;
            }
            if (z.art === "geaendert") {
              return (
                <div key={i} className="vergleich-zeile geaendert">
                  <span className="vergleich-marke">~</span>
                  <span>
                    {z.teile.map((t, k) =>
                      t.art === "weg" ? <del key={k}>{t.text}</del> : t.art === "neu" ? <ins key={k}>{t.text}</ins> : <span key={k}>{t.text}</span>,
                    )}
                  </span>
                </div>
              );
            }
            return (
              <div key={i} className={"vergleich-zeile " + z.art}>
                <span className="vergleich-marke">{z.art === "neu" ? "+" : z.art === "weg" ? "−" : ""}</span>
                <span>{z.text || " "}</span>
              </div>
            );
          })}
        </div>
        {summe && (
          <div className="modal-section muted small">
            {`${summe.neu} added · ${summe.weg} removed · ${summe.geaendert} changed`}
          </div>
        )}
      </div>
    </div>
  );
}
