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
  // Die vorhandenen Konten, wenn dieses Konto sie sehen darf. Die Liste ist
  // Verwaltungssache; wer sie nicht bekommt, tippt Adressen ein und merkt von
  // dieser Hälfte nichts.
  const [konten, setKonten] = useState<User[] | null>(null);
  const [gewaehlt, setGewaehlt] = useState<Set<string>>(new Set());
  const [suche, setSuche] = useState("");
  // Wie viele gerade an dieser Seite sitzen. Wer teilt, will sehen, ob jemand
  // drin ist, bevor er ein Recht wegnimmt.
  const [dabei, setDabei] = useState<{ anzahl: number; moeglich: boolean } | null>(null);
  const { frei } = useLizenz();

  const refresh = () => api.listShares(pageId).then(setShares).catch(() => setShares([]));
  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId]);

  // Nur solange das Fenster offen ist, und nur wenn die Lizenz es überhaupt
  // hergibt: sonst wäre es eine Abfrage im Takt für eine Zahl, die nie kommt.
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

  // Mehrere auf einmal, und aus zwei Quellen zugleich: die angehakten Konten
  // und was im Feld steht. Das Feld nimmt eine Adresse oder zwanzig, getrennt
  // durch Komma, Semikolon, Leerzeichen oder Zeilenumbruch -- eine Liste aus
  // einer Mail lässt sich so hineinkopieren, ohne sie vorher zu zerlegen.
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
    // Doppelte fallen weg: wer jemanden anhakt und seine Adresse zusätzlich
    // eintippt, meint einmal.
    const alle = Array.from(new Set([...ausListe, ...ausFeld]));
    if (alle.length === 0) {
      setErr("Niemand ausgewählt.");
      return;
    }

    setLaeuft(true);
    // Nacheinander und nicht alle auf einmal: jede Adresse bekommt ihre eigene
    // Antwort, und eine, die scheitert, soll die anderen nicht mitreißen.
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
        geschafft === 1 ? "Eine Person hinzugefügt." : `${geschafft} Personen hinzugefügt.`,
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
          <h3>Teilen</h3>
          <button className="icon-btn" onClick={onClose}>
            ✕
          </button>
        </div>

        <div className="modal-section">
          <div className="modal-label">Personen einladen</div>
          <div className="share-add">
            <input
              placeholder="E-Mail-Adressen, durch Komma getrennt"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && add()}
            />
            <select value={perm} onChange={(e) => setPerm(e.target.value)}>
              <option value="read">Kann ansehen</option>
              <option value="edit">Kann bearbeiten</option>
            </select>
            <button className="btn btn-primary" disabled={laeuft} onClick={add}>
              {laeuft ? "Fügt hinzu…" : "Hinzufügen"}
            </button>
          </div>

          {/* Die Kontenliste, wenn dieses Konto sie sehen darf. Ankreuzen statt
              abtippen: bei einem Dutzend Leuten ist das der Unterschied
              zwischen einer Handbewegung und einer Fleißaufgabe. Wer schon
              dabei ist, steht ausgegraut da und lässt sich nicht doppelt
              vergeben. */}
          {konten && konten.length > 0 && (
            <div className="konten-wahl">
              {konten.length > 8 && (
                <input
                  className="konten-suche"
                  placeholder="In der Liste suchen"
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
                        title={schonDa ? "Hat bereits Zugriff" : k.email}
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
                  {gewaehlt.size === 1 ? "Eine Person" : `${gewaehlt.size} Personen`} ausgewählt
                  — <strong>{perm === "edit" ? "kann bearbeiten" : "kann ansehen"}</strong>
                </div>
              )}
            </div>
          )}

          {hinweis && <div className="hinweis-ok">{hinweis}</div>}
          {err && <div className="error">{err}</div>}

          {frei("echtzeit") && (
            <p className="muted small">
              Wer <strong>bearbeiten</strong> darf, schreibt gleichzeitig mit: alle sehen die
              Änderungen der anderen sofort, mit Schreibmarke und Namen. Wer nur ansehen darf,
              liest.
              {dabei && dabei.moeglich && dabei.anzahl > 0 && (
                <>
                  {" "}
                  Gerade {dabei.anzahl === 1 ? "sitzt eine Person" : `sitzen ${dabei.anzahl} Personen`}{" "}
                  an dieser Seite.
                </>
              )}
              {dabei && !dabei.moeglich && (
                <> Gemeinsames Bearbeiten ist in den Einstellungen abgeschaltet.</>
              )}
            </p>
          )}

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
