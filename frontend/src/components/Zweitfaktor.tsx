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
      feld: "Dein Passwort",
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
            <strong>Der zweite Faktor steht.</strong>
            <span className="muted small">
              {stand.seit && `Seit ${new Date(stand.seit).toLocaleDateString("de-DE")}. `}
              Bei jeder Anmeldung wird nach dem Code aus der App gefragt.
            </span>
            <span className={stand.offen === 0 ? "fehlertext small" : "muted small"}>
              {stand.offen === 0
                ? "Kein Ersatzcode mehr offen. Ohne das Telefon kommst du nicht mehr hinein."
                : `${stand.offen} von 10 Ersatzcodes noch offen.`}
            </span>
          </div>
          <div className="zweit-knoepfe">
            <button
              className="btn"
              onClick={() =>
                mitPasswort(
                  "Neue Ersatzcodes",
                  "Die bisherigen Ersatzcodes gelten danach nicht mehr, auch die unbenutzten. Die neuen stehen anschließend genau einmal da.",
                  "Neue erzeugen",
                  async (pw) => setCodes((await api.zweitfaktorCodesNeu(pw)).ersatzcodes),
                )
              }
            >
              Neue Ersatzcodes
            </button>
            <button
              className="btn"
              disabled={stand.pflicht}
              title={
                stand.pflicht
                  ? "Der zweite Faktor ist für diese Instanz vorgeschrieben"
                  : undefined
              }
              onClick={() =>
                mitPasswort(
                  "Zweiten Faktor abschalten",
                  "Danach genügt zum Anmelden wieder das Passwort allein. Die Ersatzcodes verfallen dabei.",
                  "Abschalten",
                  async (pw) => {
                    await api.zweitfaktorAus(pw);
                  },
                )
              }
            >
              Abschalten
            </button>
          </div>
        </div>
      ) : (
        <div className="zweit-karte">
          <div className="zweit-lage">
            <strong>Kein zweiter Faktor eingerichtet.</strong>
            <span className="muted small">
              Zum Anmelden genügt das Passwort. Mit zweitem Faktor kommt eine
              sechsstellige Zahl aus einer Authenticator-App dazu, die alle dreißig
              Sekunden wechselt. Ein gestohlenes Passwort allein reicht dann nicht mehr.
            </span>
            {stand.pflicht && (
              <span className="fehlertext small">
                Diese Instanz verlangt ihn von jedem Konto.
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
      titel="Zweiten Faktor einrichten"
      unter="Scannen, Code eintragen, fertig"
      schliessen={schliessen}
      fuss={
        <>
          <span className="fuss-luecke" />
          <button className="btn" onClick={schliessen}>
            Abbrechen
          </button>
          <button
            className="btn btn-primary"
            disabled={busy || !start || code.trim().length < 6}
            onClick={pruefen}
          >
            {busy ? "Prüft…" : "Einschalten"}
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
            In der Authenticator-App scannen. Läuft die App auf demselben Gerät, trage
            diesen Schlüssel von Hand ein:
          </p>
          <code className="zweit-geheim">{start.geheim}</code>
          <div className="fenster-abschnitt">
            <div className="modal-label">Code aus der App</div>
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
              Erst dieser Code schaltet ein. Bis dahin ändert sich an der Anmeldung nichts.
            </p>
          </div>
        </>
      ) : (
        !fehler && <p className="muted small">Erzeugt den Schlüssel…</p>
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
      titel="Ersatzcodes"
      unter="Jeder gilt einmal"
      schliessen={schliessen}
      fuss={
        <>
          <button className="btn" onClick={kopieren}>
            {kopiert ? "Kopiert" : "Kopieren"}
          </button>
          <span className="fuss-luecke" />
          <button className="btn btn-primary" onClick={schliessen}>
            Ich habe sie notiert
          </button>
        </>
      }
    >
      <p className="muted small">
        Für den Fall, dass das Telefon weg ist. Leg sie an einen Ort, an den du auch
        ohne dieses Konto kommst: ein Passwortspeicher oder ein Zettel im Portemonnaie.
        Nach dem Schließen dieses Fensters lassen sie sich nicht wieder anzeigen, nur
        neu erzeugen.
      </p>
      <div className="codeliste">
        {codes.map((c) => (
          <code key={c}>{c}</code>
        ))}
      </div>
    </Fenster>
  );
}
