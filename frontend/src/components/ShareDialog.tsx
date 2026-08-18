// Sharing dialog. It covers the two independent mechanisms: naming individual
// accounts, and a public link anyone can open. A page can use both at once.
import { useEffect, useState } from "react";
import { ShareEntry, api } from "../api/client";

interface Props {
  pageId: string;
  isPublic: boolean;
  publicToken: string | null;
  onPublicChange: (isPublic: boolean, token: string | null) => void;
  onClose: () => void;
}

export default function ShareDialog({ pageId, isPublic, publicToken, onPublicChange, onClose }: Props) {
  const [shares, setShares] = useState<ShareEntry[]>([]);
  const [email, setEmail] = useState("");
  const [perm, setPerm] = useState("read");
  const [err, setErr] = useState("");

  const refresh = () => api.listShares(pageId).then(setShares).catch(() => setShares([]));
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);

  const add = async () => {
    setErr("");
    try {
      await api.addShare(pageId, email.trim(), perm);
      setEmail("");
      refresh();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  const remove = async (userId: string) => {
    await api.removeShare(pageId, userId);
    refresh();
  };

  // Publishing a page again reuses its existing token, so a link already handed
  // out keeps working. Revoking drops the token for good: switching the toggle
  // off and on issues a new link and kills every old one.
  const togglePublic = async () => {
    if (isPublic) {
      await api.unsharePage(pageId);
      onPublicChange(false, null);
    } else {
      const r = await api.sharePage(pageId);
      onPublicChange(true, r.publicToken);
    }
  };

  // Built from the current origin, so the link works with whatever host the app
  // is reached under without configuring a base URL.
  const publicUrl = publicToken ? `${window.location.origin}/share/${publicToken}` : "";

  return (
    /* Clicking the backdrop closes the dialog; the inner click handler stops
       the event so a click inside does not count as one outside. */
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Teilen</h3>
          <button className="icon-btn" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="modal-section">
          <div className="modal-label">Personen einladen</div>
          <div className="share-add">
            <input
              placeholder="E-Mail-Adresse"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && add()}
            />
            <select value={perm} onChange={(e) => setPerm(e.target.value)}>
              <option value="read">Kann ansehen</option>
              <option value="edit">Kann bearbeiten</option>
            </select>
            <button className="btn btn-primary" onClick={add}>
              Hinzufügen
            </button>
          </div>
          {err && <div className="error">{err}</div>}

          <div className="share-list">
            {shares.length === 0 && <div className="muted small">Noch mit niemandem geteilt.</div>}
            {shares.map((s) => (
              <div key={s.userId} className="share-row">
                <div>
                  <div className="share-name">{s.name}</div>
                  <div className="muted small">{s.email}</div>
                </div>
                <div className="share-perm">
                  <span className="pill">{s.permission === "edit" ? "Kann bearbeiten" : "Kann ansehen"}</span>
                  <button className="icon-btn" title="Entfernen" onClick={() => remove(s.userId)}>
                    ✕
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="modal-section">
          <div className="modal-label">Öffentlicher Link</div>
          <label className="toggle-row">
            <input type="checkbox" checked={isPublic} onChange={togglePublic} />
            <span>Jeder mit dem Link kann ansehen</span>
          </label>
          {isPublic && publicUrl && (
            <div className="share-add">
              {/* Selecting on focus makes the link copyable by keyboard, since
                  navigator.clipboard is unavailable over plain HTTP. */}
              <input readOnly value={publicUrl} onFocus={(e) => e.currentTarget.select()} />
              <button className="btn" onClick={() => navigator.clipboard?.writeText(publicUrl)}>
                Kopieren
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
