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

  const togglePublic = async () => {
    if (isPublic) {
      await api.unsharePage(pageId);
      onPublicChange(false, null);
    } else {
      const r = await api.sharePage(pageId);
      onPublicChange(true, r.publicToken);
    }
  };

  const publicUrl = publicToken ? `${window.location.origin}/share/${publicToken}` : "";

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Share</h3>
          <button className="icon-btn" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="modal-section">
          <div className="modal-label">Invite people</div>
          <div className="share-add">
            <input
              placeholder="email address"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && add()}
            />
            <select value={perm} onChange={(e) => setPerm(e.target.value)}>
              <option value="read">Can view</option>
              <option value="edit">Can edit</option>
            </select>
            <button className="btn btn-primary" onClick={add}>
              Add
            </button>
          </div>
          {err && <div className="error">{err}</div>}

          <div className="share-list">
            {shares.length === 0 && <div className="muted small">Not shared with anyone yet.</div>}
            {shares.map((s) => (
              <div key={s.userId} className="share-row">
                <div>
                  <div className="share-name">{s.name}</div>
                  <div className="muted small">{s.email}</div>
                </div>
                <div className="share-perm">
                  <span className="pill">{s.permission === "edit" ? "Can edit" : "Can view"}</span>
                  <button className="icon-btn" title="Remove" onClick={() => remove(s.userId)}>
                    ✕
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="modal-section">
          <div className="modal-label">Public link</div>
          <label className="toggle-row">
            <input type="checkbox" checked={isPublic} onChange={togglePublic} />
            <span>Anyone with the link can view</span>
          </label>
          {isPublic && publicUrl && (
            <div className="share-add">
              <input readOnly value={publicUrl} onFocus={(e) => e.currentTarget.select()} />
              <button className="btn" onClick={() => navigator.clipboard?.writeText(publicUrl)}>
                Copy
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
