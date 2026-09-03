// The user's profile: how they are named and how they appear.
//
// Both belong to the user and not to administrators, so this is a dedicated
// dialog in the footer rather than a settings entry.
//
// The image is resized here before upload. A camera photo may be several
// megabytes while it is displayed as a small circle; resizing in the browser
// saves bandwidth and storage.
import { useRef, useState } from "react";

import { api } from "../api/client";
import { useAuth } from "../auth";
import Profilbild from "./Profilbild";

// Edge length of the stored image. Use 256 rather than 128 so the avatar
// remains sharp on high-DPI screens and in the profile dialog.
const KANTE = 256;

// Crop centered to a square: the avatar is shown as a circle and a distorted
// face is worse than a cropped one.
async function verkleinern(datei: File): Promise<Blob> {
  const bild = await new Promise<HTMLImageElement>((fertig, gescheitert) => {
    const url = URL.createObjectURL(datei);
    const i = new Image();
    i.onload = () => {
      URL.revokeObjectURL(url);
      fertig(i);
    };
    i.onerror = () => {
      URL.revokeObjectURL(url);
      gescheitert(new Error("Das ließ sich nicht als Bild lesen."));
    };
    i.src = url;
  });

  const kante = Math.min(bild.naturalWidth, bild.naturalHeight);
  const x = (bild.naturalWidth - kante) / 2;
  const y = (bild.naturalHeight - kante) / 2;

  const leinwand = document.createElement("canvas");
  leinwand.width = KANTE;
  leinwand.height = KANTE;
  const stift = leinwand.getContext("2d");
  if (!stift) throw new Error("Der Browser kann hier nicht zeichnen.");
  stift.drawImage(bild, x, y, kante, kante, 0, 0, KANTE, KANTE);

  return new Promise<Blob>((fertig, gescheitert) =>
    leinwand.toBlob(
      (b) => (b ? fertig(b) : gescheitert(new Error("Das Bild ließ sich nicht erzeugen."))),
      // Use JPEG rather than PNG: a photo encoded as PNG would be much larger.
      // Quality 0.9 preserves visible fidelity at this size.
      "image/jpeg",
      0.9,
    ),
  );
}

export default function ProfilDialog({ onClose }: { onClose: () => void }) {
  const { user, neuLaden } = useAuth();
  const [name, setName] = useState(user?.name ?? "");
  const [fehler, setFehler] = useState("");
  const [meldung, setMeldung] = useState("");
  const [laeuft, setLaeuft] = useState<null | "name" | "bild">(null);
  const dateiFeld = useRef<HTMLInputElement>(null);

  if (!user) return null;

  const bildWaehlen = async (datei: File | undefined) => {
    if (!datei) return;
    setFehler("");
    setMeldung("");
    setLaeuft("bild");
    try {
      const klein = await verkleinern(datei);
      await api.profilbildSetzen(klein);
      await neuLaden();
      setMeldung("Bild gespeichert.");
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(null);
      // Reset the file input so the same file can be selected again.
      if (dateiFeld.current) dateiFeld.current.value = "";
    }
  };

  const bildWeg = async () => {
    setFehler("");
    setMeldung("");
    setLaeuft("bild");
    try {
      await api.profilbildWeg();
      await neuLaden();
      setMeldung("Bild entfernt.");
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(null);
    }
  };

  const namePasst = name.trim().length > 0 && [...name.trim()].length <= 80;
  const nameGeaendert = name.trim() !== user.name;

  const nameSpeichern = async () => {
    if (!namePasst || !nameGeaendert) return;
    setFehler("");
    setMeldung("");
    setLaeuft("name");
    try {
      await api.profilAendern(name.trim());
      await neuLaden();
      setMeldung("Name gespeichert.");
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(null);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Profil</h3>
          <button className="icon-btn" onClick={onClose} aria-label="Schließen">
            ✕
          </button>
        </div>

        <div className="modal-section">
          <div className="profil-kopf">
            <Profilbild
              id={user.id}
              name={user.name}
              email={user.email}
              stand={user.bildStand}
              groesse={72}
            />
            <div className="profil-kopf-text">
              <strong>{user.name}</strong>
              <div className="muted small">{user.email}</div>
              <div className="knopfreihe">
                <button
                  className="btn"
                  disabled={laeuft !== null}
                  onClick={() => dateiFeld.current?.click()}
                >
                  {user.bildStand ? "Bild ändern" : "Bild wählen"}
                </button>
                {user.bildStand && (
                  <button className="btn" disabled={laeuft !== null} onClick={bildWeg}>
                    Entfernen
                  </button>
                )}
              </div>
            </div>
          </div>
          <input
            ref={dateiFeld}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            hidden
            onChange={(e) => bildWaehlen(e.target.files?.[0])}
          />
          <p className="muted small">
            Das Bild wird auf {KANTE} × {KANTE} zugeschnitten und verkleinert, bevor es
            hochgeht — mittig, weil es als Kreis erscheint. Sichtbar ist es für jeden, der
            hier angemeldet ist.
          </p>

          <label className="modal-label" htmlFor="profil-name">
            Angezeigter Name
          </label>
          <input
            id="profil-name"
            className="rueckfrage-feld"
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && nameSpeichern()}
          />
          <p className="muted small">
            So stehst du an Seiten, Kommentaren und in Freigabelisten. Die E-Mail-Adresse ist
            die Kennung des Kontos und lässt sich hier nicht ändern — daran hängen die
            Anmeldung und jede Freigabe.
          </p>

          {fehler && <div className="fehler">{fehler}</div>}
          {meldung && !fehler && <div className="hinweis-ok">{meldung}</div>}
        </div>

        <div className="rueckfrage-knoepfe">
          <button className="btn" onClick={onClose}>
            Schließen
          </button>
          <button
            className="btn betont"
            disabled={!namePasst || !nameGeaendert || laeuft !== null}
            onClick={nameSpeichern}
          >
            {laeuft === "name" ? "Speichert…" : "Namen speichern"}
          </button>
        </div>
      </div>
    </div>
  );
}
