// What somebody can set on their own account.
//
// This existed before, but scattered: profile and password as two dialogs of
// their own in the account menu, the second factor only in the administration
// -- that is for administrators -- and one's own sessions there as well.
// Whoever is not an administrator could not reach three of them at all.
//
// Here all four stand behind the cog beside the name, in the same form as the
// administration next to it: a sidebar on the left, the content on the right.
//
// The line to the administration is the question of who something belongs to.
// The appearance belongs to the account and therefore stands here: whether
// somebody works light or dark is nobody else's business. As long as it stood
// in the administration, whoever switched to dark at night switched everybody
// else over too.
import { useCallback, useEffect, useRef, useState } from "react";

import { Sitzung, ZweitfaktorStand, api } from "../api/client";
import { useAuth } from "../auth";
import {
  AKZENTE,
  GRUND,
  GRUNDTOENE,
  TON_MARKEN,
  anwenden,
  flaecheLesbar,
  useDesign,
  verschobenAuf,
} from "../design";
import Fenster from "./Fenster";
import Listenkopf from "./Listenkopf";
import Profilbild from "./Profilbild";
import { useRueckfrage } from "./Rueckfrage";
import Zweitfaktor from "./Zweitfaktor";
import Sprachwahl from "../sprache/Sprachwahl";

// Edge length of the stored picture. 256 instead of 128, so the picture stays
// sharp on high-resolution screens and in the profile.
const KANTE = 256;

type Teil = "profil" | "aussehen" | "passwort" | "zweitfaktor" | "geraete";

const TEILE: { id: Teil; titel: string; unter: string }[] = [
  { id: "profil", titel: "Profile", unter: "Name and picture" },
  { id: "aussehen", titel: "Appearance", unter: "Base tone and accent" },
  { id: "passwort", titel: "Password", unter: "Change it" },
  { id: "zweitfaktor", titel: "Second factor", unter: "App and recovery codes" },
  { id: "geraete", titel: "Devices", unter: "Where you are signed in" },
];

/**
 * Crop to a square in the centre and shrink: the picture appears as a circle,
 * and a distorted face is worse than a cropped one. A photo out of a camera has
 * several megabytes and is shown as a small circle -- shrinking it in the
 * browser saves line and disk.
 */
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
      gescheitert(new Error("That could not be read as an image."));
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
  if (!stift) throw new Error("The browser cannot draw here.");
  stift.drawImage(bild, x, y, kante, kante, 0, 0, KANTE, KANTE);

  return new Promise<Blob>((fertig, gescheitert) =>
    leinwand.toBlob(
      // JPEG and not PNG: a photo as PNG would be several times larger.
      // Quality 0.9 is indistinguishable from 1.0 at this size.
      (b) => (b ? fertig(b) : gescheitert(new Error("The image could not be created."))),
      "image/jpeg",
      0.9,
    ),
  );
}

export default function MeinKonto({ onClose }: { onClose: () => void }) {
  const { user } = useAuth();
  const [teil, setTeil] = useState<Teil>("profil");

  if (!user) return null;

  return (
    <Fenster
      titel="My account"
      unter={user.email}
      breit
      schliessen={onClose}
      fuss={
        <>
          <span className="fuss-luecke" />
          <button className="btn btn-primary" onClick={onClose}>
            Close
          </button>
        </>
      }
    >
      {/* The same layout as in the administration: a sidebar on the left,
          the content on the right. Four entries are few for a sidebar -- but
          they make visible what there is at all, and that was the question
          before: nobody finds the second factor who does not know it exists. */}
      <div className="konto-fenster">
        <nav className="konto-leiste">
          {TEILE.map((t) => (
            <button
              key={t.id}
              className={"konto-leiste-eintrag" + (teil === t.id ? " aktiv" : "")}
              onClick={() => setTeil(t.id)}
            >
              <span>{t.titel}</span>
              <span className="muted small">{t.unter}</span>
            </button>
          ))}
        </nav>
        <div className="konto-inhalt">
          {teil === "profil" && <Profil />}
          {teil === "aussehen" && <Aussehen />}
          {teil === "passwort" && <Passwort />}
          {teil === "zweitfaktor" && <ZweitfaktorTeil />}
          {teil === "geraete" && <Geraete />}
        </div>
      </div>
    </Fenster>
  );
}

/**
 * Base tone and accent, for this account alone.
 *
 * Saved immediately on the click and without a button: one picks a colour by
 * seeing it, and a Save button in between turns trying things out into a chain
 * of confirmations. It is applied before the server's answer, otherwise the
 * interface blinks along behind the choice.
 */
function Aussehen() {
  const { design, neuLaden } = useDesign();
  const [ton, setTon] = useState(design.grundton);
  const [akzent, setAkzent] = useState(design.akzent.toLowerCase());
  const [fehler, setFehler] = useState("");

  const grund = GRUND[ton] ?? GRUND.grau;
  const verschoben = verschobenAuf(akzent, grund);

  const sichern = (g: string, a: string) => {
    setFehler("");
    api
      .aussehenSpeichern(g, a)
      .then(() => neuLaden())
      // If the choice stays unsaved, it is on the screen all the same. The
      // sentence says that it does not survive a restart.
      .catch((e) => setFehler(e instanceof Error ? e.message : "Not saved."));
  };

  const tonSetzen = (wert: string) => {
    anwenden({ grundton: wert, akzent });
    setTon(wert);
    sichern(wert, akzent);
  };

  const akzentSetzen = (wert: string, speichern: boolean) => {
    anwenden({ grundton: ton, akzent: wert });
    setAkzent(wert);
    if (speichern) sichern(ton, wert);
  };

  return (
    <>
      <h3>Language</h3>
      <p className="muted small">Applies to this browser. The editor's menus follow on the next page opened.</p>
      <Sprachwahl />
      <h3>Base tone</h3>
      <p className="muted small">
        Applies to this account, on every device you are signed in on.
      </p>
      <div className="tonwahl">
        {GRUNDTOENE.map((g) => {
          const marken = TON_MARKEN[g.wert];
          return (
            <button
              key={g.wert}
              type="button"
              className={"tonkachel" + (ton === g.wert ? " gewaehlt" : "")}
              style={{ background: marken[0], borderColor: marken[2], color: marken[3] }}
              onClick={() => tonSetzen(g.wert)}
            >
              <span
                className="tonkachel-probe"
                style={{ background: marken[1], borderColor: marken[2] }}
              />
              <span className="tonkachel-name">{g.titel}</span>
            </button>
          );
        })}
      </div>

      <h3>Accent colour</h3>
      <p className="muted small">
        Colours buttons, links and the highlight in lists.
      </p>
      <div className="akzentwahl">
        {AKZENTE.map((a) => (
          <button
            key={a.wert}
            type="button"
            className={"akzentknopf" + (akzent === a.wert ? " gewaehlt" : "")}
            style={{ background: a.wert }}
            title={`${a.titel} · ${a.wert}`}
            aria-label={a.titel}
            onClick={() => akzentSetzen(a.wert, true)}
          />
        ))}
        {/* A house colour comes as a hex value out of a manual and not out
            of the operating system's colour wheel. Hence the field, with the
            picker merely beside it. */}
        <input
          className="hex-feld"
          value={akzent}
          spellCheck={false}
          maxLength={7}
          aria-label="Custom value"
          placeholder="#2383e2"
          onChange={(ev) => {
            const w = ev.target.value.trim().toLowerCase();
            setAkzent(w);
            if (/^#[0-9a-f]{6}$/.test(w)) anwenden({ grundton: ton, akzent: w });
          }}
          onBlur={() => {
            // A half-typed value must not go into the database.
            if (!/^#[0-9a-f]{6}$/.test(akzent)) {
              akzentSetzen(design.akzent.toLowerCase(), false);
              return;
            }
            if (akzent !== design.akzent.toLowerCase()) sichern(ton, akzent);
          }}
        />
        <input
          type="color"
          className="farbwaehler"
          aria-label="Colour picker"
          value={/^#[0-9a-f]{6}$/.test(akzent) ? akzent : "#2383e2"}
          onChange={(ev) => akzentSetzen(ev.target.value.toLowerCase(), false)}
          onBlur={() => {
            if (akzent !== design.akzent.toLowerCase()) sichern(ton, akzent);
          }}
        />
      </div>

      {/* The sample stands in place of a column of contrast figures.
          Whoever picks a colour sees here what it does; they do not have to work
          it out. */}
      <div className="wirkprobe">
        <button className="btn btn-primary" type="button">
          Primary button
        </button>
        <button className="btn" type="button">
          Secondary button
        </button>
        <a
          className="wirkprobe-verweis"
          href="#aussehen"
          onClick={(e) => e.preventDefault()}
        >
          A link in running text
        </a>
        <span className="wirkprobe-zeile">Selected entry</span>
      </div>

      {!flaecheLesbar(akzent) && (
        <p className="muted small">
          Labels are hard to read on this surface. A darker or lighter value of the
          same colour helps.
        </p>
      )}
      {verschoben && (
        <p className="muted small">
          As text on the ground the colour is shifted to <code>{verschoben}</code>,
          otherwise a link in running text would be unreadable. Surfaces keep the value
          you entered.
        </p>
      )}
      {fehler && <div className="fehlertext small">{fehler}</div>}
    </>
  );
}

/** Name and picture. The address stands beside them and cannot be changed. */
function Profil() {
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
      await api.profilbildSetzen(await verkleinern(datei));
      await neuLaden();
      setMeldung("Picture saved.");
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(null);
      // Reset the field, so the same file can be chosen again.
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
      setMeldung("Picture removed.");
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
      setMeldung("Name saved.");
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(null);
    }
  };

  return (
    <>
      <h3>Profile</h3>
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
              {user.bildStand ? "Change picture" : "Choose a picture"}
            </button>
            {user.bildStand && (
              <button className="btn" disabled={laeuft !== null} onClick={bildWeg}>
                Remove
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
        Das Bild wird auf {KANTE} × {KANTE} zugeschnitten und verkleinert, bevor es hochgeht
        — mittig, weil es als Kreis erscheint. Sichtbar ist es für jeden, der hier angemeldet
        ist.
      </p>

      <div className="fenster-abschnitt">
        <div className="modal-label">Display name</div>
        <div className="fenster-zeile">
          <input
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && nameSpeichern()}
          />
          <button
            className="btn"
            disabled={!namePasst || !nameGeaendert || laeuft !== null}
            onClick={nameSpeichern}
          >
            {laeuft === "name" ? "Saving…" : "Save"}
          </button>
        </div>
        <p className="muted small">
          This is how you appear on pages, in comments and in share lists. The email
          address is the account’s identifier and cannot be changed here — sign-in
          and every share hang on it.
        </p>
      </div>

      {fehler && <div className="fehler">{fehler}</div>}
      {meldung && !fehler && <div className="hinweis-ok">{meldung}</div>}
    </>
  );
}

/**
 * Changing one's own password.
 *
 * Three fields instead of two: repeating the new one catches the typo that
 * would otherwise only show up at the next sign-in -- and then could no longer
 * be fixed without asking an administrator.
 */
function Passwort() {
  const [alt, setAlt] = useState("");
  const [neu, setNeu] = useState("");
  const [nochmal, setNochmal] = useState("");
  const [fehler, setFehler] = useState("");
  const [fertig, setFertig] = useState<string | null>(null);
  const [laeuft, setLaeuft] = useState(false);

  // The same limits as in the service (backend/internal/handlers/passwort.go).
  // The check here is only there for immediate feedback; it is decided at the
  // server.
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
      setAlt("");
      setNeu("");
      setNochmal("");
      setFertig(
        beendet > 0
          ? `Passwort gewechselt. ${beendet} andere ${
              beendet === 1 ? "session was" : "sessions were"
            } beendet, dieses Gerät bleibt angemeldet.`
          : "Password changed. No other session was open.",
      );
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(false);
    }
  };

  return (
    <>
      <h3>Change password</h3>
      {fertig ? (
        <>
          <div className="hinweis-ok">{fertig}</div>
          <div className="knopfreihe">
            <button className="btn" onClick={() => setFertig(null)}>
              Change it again
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="muted small">
            The current password is asked for even though you are signed in. It is not
            about who you are, but about the case where somebody else is sitting at your
            open browser. After the change every other session is ended; this device stays
            signed in.
          </p>
          <div className="fenster-felder">
            <label className="feld-breit">
              <span>Current password</span>
              <input
                type="password"
                autoComplete="current-password"
                value={alt}
                onChange={(e) => setAlt(e.target.value)}
              />
            </label>
            <label>
              <span>New password</span>
              <input
                type="password"
                autoComplete="new-password"
                value={neu}
                onChange={(e) => setNeu(e.target.value)}
              />
            </label>
            <label>
              <span>Once more</span>
              <input
                type="password"
                autoComplete="new-password"
                value={nochmal}
                onChange={(e) => setNochmal(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && senden()}
              />
            </label>
          </div>
          {zuKurz && <div className="fehler">At least 6 characters.</div>}
          {zuLang && (
            <div className="fehler">
              Zu lang. Mehr als 72 Zeichen liest die Prüfung nicht, der Rest fiele
              stillschweigend weg.
            </div>
          )}
          {passtNicht && <div className="fehler">The two entries differ.</div>}
          {fehler && <div className="fehler">{fehler}</div>}
          <div className="knopfreihe">
            <button className="btn" disabled={!bereit} onClick={senden}>
              {laeuft ? "Changing…" : "Change password"}
            </button>
          </div>
        </>
      )}
    </>
  );
}

function ZweitfaktorTeil() {
  const [stand, setStand] = useState<ZweitfaktorStand | null>(null);
  const laden = useCallback(() => {
    api.zweitfaktorStand().then(setStand).catch(() => setStand(null));
  }, []);
  useEffect(laden, [laden]);

  return (
    <>
      <h3>Second factor</h3>
      {stand === null ? (
        <p className="muted small">Loading…</p>
      ) : (
        <Zweitfaktor stand={stand} neuLaden={laden} />
      )}
    </>
  );
}

/**
 * One's own sessions.
 *
 * They used to stand only in the administration, that is for administrators --
 * although the list is one's own anyway: the service gives everybody only
 * theirs.
 */
function Geraete() {
  const frage = useRueckfrage();
  const [sitzungen, setSitzungen] = useState<Sitzung[] | null>(null);
  const [filter, setFilter] = useState("");
  const laden = useCallback(() => {
    api.sitzungen().then(setSitzungen).catch(() => setSitzungen([]));
  }, []);
  useEffect(laden, [laden]);

  const begriff = filter.trim().toLowerCase();
  const sichtbar = (sitzungen ?? []).filter(
    (s) => !begriff || (s.browser + " " + s.ip).toLowerCase().includes(begriff),
  );

  const beenden = async (s: Sitzung) => {
    if (
      !(await frage({
        titel: s.diese ? "Sign out here" : "End session",
        text: s.diese
          ? "This is the session you are working in right now. After ending it you have to sign in again."
          : `The session on ${s.browser} becomes invalid at once. Whoever uses it lands on the sign-in page.`,
        bestaetigen: "End it",
        gefaehrlich: !s.diese,
      }))
    )
      return;
    await api.sitzungBeenden(s.id).catch(() => {});
    if (s.diese) window.location.href = "/login";
    else laden();
  };

  return (
    <>
      <Listenkopf
        titel="Signed-in devices"
        zahl={
          sitzungen === null
            ? "wird geladen"
            : `${sitzungen.length} ${sitzungen.length === 1 ? "session" : "sessions"}`
        }
        filter={filter}
        setFilter={setFilter}
        platzhalter="Filter by device or address"
      >
        {(sitzungen?.length ?? 0) > 1 && (
          <button
            className="btn"
            onClick={async () => {
              if (
                !(await frage({
                  titel: "Sign out everywhere else",
                  text: "Every other session is ended. This one stays.",
                  bestaetigen: "End all others",
                  gefaehrlich: true,
                }))
              )
                return;
              await api.sitzungenBeenden().catch(() => {});
              laden();
            }}
          >
            Sign out everywhere else
          </button>
        )}
      </Listenkopf>
      <p className="muted small">
        If you find a device here that is not yours, end the session and change your
        password afterwards — it ends at once, but whoever has the password would
        otherwise just sign in again.
      </p>
      <table className="tabelle">
        <thead>
          <tr>
            <th>Gerät</th>
            <th>Address</th>
            <th>Last seen</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {sichtbar.map((s) => (
            <tr key={s.id}>
              <td>
                {s.browser}
                {s.diese && (
                  <span className="pill frei" style={{ marginLeft: 8 }}>
                    dieses Gerät
                  </span>
                )}
              </td>
              <td className="muted small">{s.ip || "—"}</td>
              <td className="muted small">
                {new Date(s.zuletztAm).toLocaleString(undefined, {
                  day: "2-digit",
                  month: "2-digit",
                  year: "numeric",
                  hour: "2-digit",
                  minute: "2-digit",
                })}
              </td>
              <td className="zeilen-aktionen">
                <button className="btn-schlicht gefaehrlich" onClick={() => beenden(s)}>
                  End it
                </button>
              </td>
            </tr>
          ))}
          {sichtbar.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                {sitzungen === null
                  ? "Loading…"
                  : sitzungen.length === 0
                    ? "No stored session."
                    : "No session matches the filter."}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}
