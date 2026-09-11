// Sharing dialog. It covers the two independent mechanisms: naming individual
// accounts, and a public link anyone can open. A page can use both at once.
import { useEffect, useState } from "react";
import { ShareEntry, User, api } from "../api/client";
import { useLizenz } from "../lizenz";

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
  const [hinweis, setHinweis] = useState("");
  const [laeuft, setLaeuft] = useState(false);
  // The available accounts if this user is allowed to see them. The list is
  // an administrative convenience; users who cannot see it can still type
  // addresses manually.
  const [konten, setKonten] = useState<User[] | null>(null);
  const [gewaehlt, setGewaehlt] = useState<Set<string>>(new Set());
  const [suche, setSuche] = useState("");
  // How many people are currently on this page. When changing shares the user
  // wants to know if someone is active before revoking rights.
  const [dabei, setDabei] = useState<{ anzahl: number; moeglich: boolean } | null>(null);
  const { frei } = useLizenz();

  const refresh = () => api.listShares(pageId).then(setShares).catch(() => setShares([]));
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);

  // Poll the presence count only while the dialog is open and when the
  // license grants the feature; otherwise polling would repeatedly request a
  // number that will not be provided.
  useEffect(() => {
    if (!frei("echtzeit")) return;
    let lebt = true;
    const holen = () =>
      api
        .mitschreibende(pageId)
        .then((d) => lebt && setDabei(d))
        .catch(() => lebt && setDabei(null));
    holen();
    const takt = window.setInterval(holen, 4000);
    return () => {
      lebt = false;
      window.clearInterval(takt);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);

  useEffect(() => {
    api
      .listUsers()
      .then(setKonten)
      .catch(() => setKonten(null));
  }, []);

  // Support adding multiple recipients at once from two sources: checked
  // accounts and addresses typed into the field. The field accepts one or
  // many addresses separated by commas, semicolons, whitespace or newlines so
  // a pasted mailing list can be added without splitting it beforehand.
  const adressenAus = (roh: string) =>
    roh
      .split(/[\s,;]+/)
      .map((t) => t.trim())
      .filter(Boolean);

  const add = async () => {
    setErr("");
    setHinweis("");
    const ausListe = (konten ?? []).filter((k) => gewaehlt.has(k.id)).map((k) => k.email);
    const ausFeld = adressenAus(email);
    // Duplicates are removed: checking an account and typing the same email
    // once more counts only once.
    const alle = Array.from(new Set([...ausListe, ...ausFeld]));
    if (alle.length === 0) {
      setErr("Nobody selected.");
      return;
    }

    setLaeuft(true);
    // Send one request per address rather than in parallel: each address gets
    // its own response and a failing one should not abort the others.
    const gescheitert: string[] = [];
    let geschafft = 0;
    for (const adresse of alle) {
      try {
        await api.addShare(pageId, adresse, perm);
        geschafft++;
      } catch (e) {
        gescheitert.push(`${adresse}: ${(e as Error).message}`);
      }
    }
    setLaeuft(false);
    setEmail("");
    setGewaehlt(new Set());
    if (geschafft > 0) {
      setHinweis(
        geschafft === 1 ? "One person added." : `${geschafft} people added.`,
      );
    }
    if (gescheitert.length > 0) setErr(gescheitert.join(" · "));
    refresh();
  };

  const umschalten = (id: string) => {
    setGewaehlt((vorher) => {
      const neu = new Set(vorher);
      if (neu.has(id)) neu.delete(id);
      else neu.add(id);
      return neu;
    });
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
          <h3>Share</h3>
          <button className="icon-btn" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="modal-section">
          <div className="modal-label">Invite people</div>
          <div className="share-add">
            <input
              placeholder="Email addresses, separated by commas"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && add()}
            />
            <select value={perm} onChange={(e) => setPerm(e.target.value)}>
              <option value="read">Can view</option>
              <option value="edit">Can edit</option>
            </select>
            <button className="btn btn-primary" disabled={laeuft} onClick={add}>
              {laeuft ? "Adding…" : "Add"}
            </button>
          </div>

            {/* The account list shown when this user is allowed to see it. Checking
              boxes instead of typing is the difference between a single motion
              and a tedious chore when adding a dozen people. Accounts that
              already have access are shown disabled so they are not added
              twice. */}
          {konten && konten.length > 0 && (
            <div className="konten-wahl">
              {konten.length > 8 && (
                <input
                  className="konten-suche"
                  placeholder="Search the list"
                  value={suche}
                  onChange={(e) => setSuche(e.target.value)}
                />
              )}
              <div className="konten-liste">
                {konten
                  .filter((k) => {
                    const q = suche.toLowerCase().trim();
                    if (!q) return true;
                    return (
                      (k.name || "").toLowerCase().includes(q) ||
                      k.email.toLowerCase().includes(q)
                    );
                  })
                  .map((k) => {
                    const schonDa = shares.some((sh) => sh.userId === k.id);
                    return (
                      <label
                        key={k.id}
                        className={"konten-zeile" + (schonDa ? " schon-da" : "")}
                        title={schonDa ? "Already has access" : k.email}
                      >
                        <input
                          type="checkbox"
                          disabled={schonDa}
                          checked={gewaehlt.has(k.id)}
                          onChange={() => umschalten(k.id)}
                        />
                        <span className="konten-name">{k.name || k.email}</span>
                        <span className="muted small">{k.email}</span>
                      </label>
                    );
                  })}
              </div>
              {gewaehlt.size > 0 && (
                <div className="muted small">
                  {gewaehlt.size === 1 ? "One person" : `${gewaehlt.size} people`} selected
                  — <strong>{perm === "edit" ? "can edit" : "can view"}</strong>
                </div>
              )}
            </div>
          )}

          {hinweis && <div className="hinweis-ok">{hinweis}</div>}
          {err && <div className="error">{err}</div>}

          {frei("echtzeit") && (
            <p className="muted small">
              Whoever may <strong>edit</strong> writes at the same time as everyone else:
              all of them see the others’ changes at once, cursor and name included.
              Whoever may only view, reads.
              {dabei && dabei.moeglich && dabei.anzahl > 0 && (
                <>
                  {" "}
                  {dabei.anzahl === 1 ? "One person is" : `${dabei.anzahl} people are`}{" "}
                  on this page right now.
                </>
              )}
              {dabei && !dabei.moeglich && (
                <> Editing together is switched off in the settings.</>
              )}
            </p>
          )}

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
              {/* Selecting on focus makes the link copyable by keyboard, since
                  navigator.clipboard is unavailable over plain HTTP. */}
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
