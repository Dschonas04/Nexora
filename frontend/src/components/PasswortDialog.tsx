// Change the current user's password.
//
// This is a dedicated dialog rather than a settings page because changing
// one's password is a personal action. The entry point is in the footer next
// to the username.
//
// Three fields instead of two: repeating the new password catches a typo that
// would otherwise only surface on the next login.
import { useState } from "react";
import { api } from "../api/client";

export default function PasswortDialog({ onClose }: { onClose: () => void }) {
  const [alt, setAlt] = useState("");
  const [neu, setNeu] = useState("");
  const [nochmal, setNochmal] = useState("");
  const [fehler, setFehler] = useState("");
  const [fertig, setFertig] = useState<string | null>(null);
  const [laeuft, setLaeuft] = useState(false);

  // The same limits as enforced on the server (see backend/internal/handlers/passwort.go).
  // These checks are only cosmetic to provide immediate feedback; the server
  // is the authority.
  const zuKurz = neu.length > 0 && [...neu].length < 6;
  const zuLang = new TextEncoder().encode(neu).length > 72;
  const passtNicht = nochmal.length > 0 && neu !== nochmal;
  const bereit =
    alt.length > 0 && neu.length > 0 && neu === nochmal && !zuKurz && !zuLang && !laeuft;

  const senden = async () => {
    if (!bereit) return;
    setFehler("");
    setLaeuft(true);
    try {
      const { beendet } = await api.passwortWechseln(alt, neu);
      setFertig(
        beendet > 0
          ? `Passwort gewechselt. ${beendet} andere ${
              beendet === 1 ? "Sitzung wurde" : "Sitzungen wurden"
            } beendet, dieses Gerät bleibt angemeldet.`
          : "Passwort gewechselt. Es war keine weitere Sitzung offen.",
      );
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Passwort ändern</h3>
          <button className="icon-btn" onClick={onClose} aria-label="Schließen">
            ✕
          </button>
        </div>

        {fertig ? (
          <>
            <div className="modal-section">
              <div className="hinweis-ok">{fertig}</div>
            </div>
            <div className="rueckfrage-knoepfe">
              <button className="btn betont" onClick={onClose}>
                Schließen
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="modal-section">
              <p className="muted small">
                Das bisherige Passwort wird verlangt, obwohl du angemeldet bist. Es geht nicht
                darum, wer du bist, sondern um den Fall, dass jemand anders vor deinem offenen
                Browser sitzt. Nach dem Wechsel werden alle anderen Sitzungen beendet, dieses
                Gerät bleibt angemeldet.
              </p>

              <label className="modal-label" htmlFor="pw-alt">
                Bisheriges Passwort
              </label>
              <input
                id="pw-alt"
                className="rueckfrage-feld"
                type="password"
                autoComplete="current-password"
                autoFocus
                value={alt}
                onChange={(e) => setAlt(e.target.value)}
              />

              <label className="modal-label" htmlFor="pw-neu">
                Neues Passwort
              </label>
              <input
                id="pw-neu"
                className="rueckfrage-feld"
                type="password"
                autoComplete="new-password"
                value={neu}
                onChange={(e) => setNeu(e.target.value)}
              />
              {zuKurz && <div className="fehler">Mindestens 6 Zeichen.</div>}
              {zuLang && (
                <div className="fehler">
                  Zu lang. Mehr als 72 Zeichen liest die Prüfung nicht, der Rest fiele
                  stillschweigend weg.
                </div>
              )}

              <label className="modal-label" htmlFor="pw-nochmal">
                Noch einmal
              </label>
              <input
                id="pw-nochmal"
                className="rueckfrage-feld"
                type="password"
                autoComplete="new-password"
                value={nochmal}
                onChange={(e) => setNochmal(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && senden()}
              />
              {passtNicht && <div className="fehler">Die beiden Eingaben sind verschieden.</div>}

              {fehler && <div className="fehler">{fehler}</div>}
            </div>
            <div className="rueckfrage-knoepfe">
              <button className="btn" onClick={onClose}>
                Abbrechen
              </button>
              <button className="btn betont" disabled={!bereit} onClick={senden}>
                {laeuft ? "Wechselt…" : "Passwort ändern"}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
