// The second factor on one's own account: setting it up, recovery codes,
// switching it off.
//
// It stands as a file of its own and not in the settings sheet, because it is
// three states with a dialog each and the page above would otherwise grow by
// another three hundred lines.
//
// The flow has two steps on purpose. On opening, a secret comes into being that
// is not yet in force; only the first correct code switches it on. Whoever
// closes the dialog in between has changed nothing -- without that separation
// an aborted setup locked the account out.
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
  /** After every change: fetch the state again, so the card shows it. */
  neuLaden: () => void;
}) {
  const eingabeFragen = useEingabe();
  const [einrichten, setEinrichten] = useState(false);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [fehler, setFehler] = useState("");

  if (!stand) return null;

  // Switching off and demanding new recovery codes both require the password.
  // Without this confirmation an unattended screen would be enough to remove
  // the second factor -- and thereby precisely what it stands against.
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

/** The dialog with the QR code and the check field. */
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

  // Only when the dialog is opened, not when the page is built: the call
  // creates a secret, and that should only happen when somebody really wants
  // it.
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
          {/* The SVG comes from our own service and contains nothing but
              rectangles; it is inserted rather than rebuilt, because otherwise
              the frontend would have to bring a library along for a single
              screen. */}
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
 * The recovery codes. They stand there exactly once -- the database holds only
 * their hashes, and nobody can display them a second time.
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
