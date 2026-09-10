// Der zweite Faktor am eigenen Konto: einrichten, Ersatzcodes, abschalten.
//
// Es steht als eigene Datei und nicht im Einstellungsblatt, weil es drei
// Zustände mit je eigenem Fenster sind und die Seite darüber sonst noch einmal
// um dreihundert Zeilen wüchse.
//
// Der Ablauf hat mit Absicht zwei Schritte. Beim Öffnen entsteht ein
// Geheimnis, das noch nicht gilt; erst der erste richtige Code schaltet es ein.
// Wer das Fenster dazwischen schließt, hat nichts verstellt -- ohne diese
// Trennung sperrte ein abgebrochenes Einrichten das Konto aus.
import { useEffect, useState } from "react";

import { api } from "../api/client";
import Fenster from "./Fenster";
import { useEingabe } from "./Rueckfrage";

interface Stand {
  aktiv: boolean;
  seit?: string;
  offen: number;
  pflicht: boolean;
}

export default function Zweitfaktor({
  stand,
  neuLaden,
}: {
  stand: Stand | null;
  /** Nach jeder Änderung: den Stand neu holen, damit die Karte ihn zeigt. */
  neuLaden: () => void;
}) {
  const eingabeFragen = useEingabe();
  const [einrichten, setEinrichten] = useState(false);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [fehler, setFehler] = useState("");

  if (!stand) return null;

  // Abschalten und neue Ersatzcodes verlangen beide das Passwort. Ohne diese
  // Rückfrage genügte ein unbeaufsichtigter Bildschirm, um den zweiten Faktor
  // zu entfernen -- und damit genau das, wogegen er steht.
  const mitPasswort = async (
    titel: string,
    text: string,
    bestaetigen: string,
    tun: (pw: string) => Promise<void>,
  ) => {
    const pw = await eingabeFragen({
      titel,
      text,
      feld: "Your password",
      art: "passwort",
      bestaetigen,
    });
    if (pw === null) return;
    setFehler("");
    try {
      await tun(pw);
      neuLaden();
    } catch (e) {
      setFehler((e as Error).message);
    }
  };

  return (
    <>
      {fehler && <div className="fehler">{fehler}</div>}

      {stand.aktiv ? (
        <div className="zweit-karte">
          <div className="zweit-lage">
            <strong>The second factor is in place.</strong>
            <span className="muted small">
              {stand.seit && `Seit ${new Date(stand.seit).toLocaleDateString()}. `}
              Every sign-in asks for the code from the app.
            </span>
            <span className={stand.offen === 0 ? "fehlertext small" : "muted small"}>
              {stand.offen === 0
                ? "No recovery code left. Without the phone you cannot get in."
                : `${stand.offen} of 10 recovery codes left.`}
            </span>
          </div>
          <div className="zweit-knoepfe">
            <button
              className="btn"
              onClick={() =>
                mitPasswort(
                  "New recovery codes",
                  "The existing recovery codes stop working, unused ones included. The new ones are shown exactly once.",
                  "Create new ones",
                  async (pw) => setCodes((await api.zweitfaktorCodesNeu(pw)).ersatzcodes),
                )
              }
            >
              New recovery codes
            </button>
            <button
              className="btn"
              disabled={stand.pflicht}
              title={
                stand.pflicht
                  ? "The second factor is mandatory on this instance"
                  : undefined
              }
              onClick={() =>
                mitPasswort(
                  "Switch off the second factor",
                  "After that the password alone is enough again to sign in. The recovery codes expire with it.",
                  "Switch off",
                  async (pw) => {
                    await api.zweitfaktorAus(pw);
                  },
                )
              }
            >
              Switch off
            </button>
          </div>
        </div>
      ) : (
        <div className="zweit-karte">
          <div className="zweit-lage">
            <strong>No second factor set up.</strong>
            <span className="muted small">
              The password alone is enough to sign in. With a second factor a
              six-digit number from an authenticator app is added, changing every thirty
              seconds. A stolen password alone is then no longer enough.
            </span>
            {stand.pflicht && (
              <span className="fehlertext small">
                This instance requires it of every account.
              </span>
            )}
          </div>
          <div className="zweit-knoepfe">
            <button className="btn btn-primary" onClick={() => setEinrichten(true)}>
              Einrichten
            </button>
          </div>
        </div>
      )}

      {einrichten && (
        <EinrichtenFenster
          schliessen={() => setEinrichten(false)}
          fertig={(neue) => {
            setEinrichten(false);
            setCodes(neue);
            neuLaden();
          }}
        />
      )}
      {codes && <CodeFenster codes={codes} schliessen={() => setCodes(null)} />}
    </>
  );
}

/** Das Fenster mit QR-Code und Prüffeld. */
function EinrichtenFenster({
  schliessen,
  fertig,
}: {
  schliessen: () => void;
  fertig: (codes: string[]) => void;
}) {
  const [start, setStart] = useState<{ geheim: string; uri: string; qr: string } | null>(null);
  const [code, setCode] = useState("");
  const [fehler, setFehler] = useState("");
  const [busy, setBusy] = useState(false);

  // Erst beim Öffnen des Fensters, nicht beim Aufbau der Seite: der Aufruf legt
  // ein Geheimnis an, und das soll nur passieren, wenn jemand es wirklich will.
  useEffect(() => {
    api
      .zweitfaktorStart()
      .then(setStart)
      .catch((e) => setFehler((e as Error).message));
  }, []);

  const pruefen = async () => {
    setBusy(true);
    setFehler("");
    try {
      const { ersatzcodes } = await api.zweitfaktorAn(code);
      fertig(ersatzcodes);
    } catch (e) {
      setFehler((e as Error).message);
      setCode("");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Fenster
      titel="Set up the second factor"
      unter="Scan, enter the code, done"
      schliessen={schliessen}
      fuss={
        <>
          <span className="fuss-luecke" />
          <button className="btn" onClick={schliessen}>
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={busy || !start || code.trim().length < 6}
            onClick={pruefen}
          >
            {busy ? "Checking…" : "Switch on"}
          </button>
        </>
      }
    >
      {fehler && <div className="fehler">{fehler}</div>}
      {start ? (
        <>
          {/* Das SVG kommt vom eigenen Dienst und enthaelt nur Rechtecke; es
              wird eingesetzt statt nachgebaut, weil das Frontend sonst eine
              Bibliothek fuer einen einzigen Bildschirm mitbraechte. */}
          <div className="qr-kasten" dangerouslySetInnerHTML={{ __html: start.qr }} />
          <p className="muted small">
            Scan this in your authenticator app. If the app runs on this same device,
            enter the key by hand:
          </p>
          <code className="zweit-geheim">{start.geheim}</code>
          <div className="fenster-abschnitt">
            <div className="modal-label">Code from the app</div>
            <input
              className="codefeld"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="123456"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && code.trim().length >= 6 && pruefen()}
            />
            <p className="muted small">
              Only this code switches it on. Until then nothing about signing in changes.
            </p>
          </div>
        </>
      ) : (
        !fehler && <p className="muted small">Creating the key…</p>
      )}
    </Fenster>
  );
}

/**
 * Die Ersatzcodes. Sie stehen genau einmal da -- in der Datenbank liegen nur
 * ihre Hashes, und niemand kann sie noch einmal anzeigen.
 */
function CodeFenster({ codes, schliessen }: { codes: string[]; schliessen: () => void }) {
  const [kopiert, setKopiert] = useState(false);
  const kopieren = () => {
    navigator.clipboard
      ?.writeText(codes.join("\n"))
      .then(() => setKopiert(true))
      .catch(() => {});
  };

  return (
    <Fenster
      titel="Recovery codes"
      unter="Each one works once"
      schliessen={schliessen}
      fuss={
        <>
          <button className="btn" onClick={kopieren}>
            {kopiert ? "Copied" : "Copy"}
          </button>
          <span className="fuss-luecke" />
          <button className="btn btn-primary" onClick={schliessen}>
            I have written them down
          </button>
        </>
      }
    >
      <p className="muted small">
        For the case where the phone is gone. Put them somewhere you can reach without
        this account: a password manager, or a note in your wallet. After you close this
        window they cannot be shown again, only created anew.
      </p>
      <div className="codeliste">
        {codes.map((c) => (
          <code key={c}>{c}</code>
        ))}
      </div>
    </Fenster>
  );
}
