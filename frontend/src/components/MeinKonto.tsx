// Was jemand an seinem eigenen Konto einstellen kann.
//
// Es gab das bisher, aber verstreut: Profil und Passwort als zwei eigene
// Dialoge im Kontomenü, der zweite Faktor nur in der Verwaltung -- also für
// Administratoren --, und die eigenen Sitzungen ebenfalls dort. Wer kein
// Administrator ist, kam an drei davon gar nicht heran.
//
// Hier steht alles vier hinter dem Zahnrad neben dem Namen, in derselben Form
// wie die Verwaltung daneben: eine Leiste links, der Inhalt rechts.
//
// Die Grenze zur Verwaltung ist die Frage, wem etwas gehört. Das Aussehen
// gehört dem Konto und steht deshalb hier: ob jemand hell oder dunkel
// arbeitet, geht niemanden sonst etwas an. Solange es in der Verwaltung stand,
// stellte der, der nachts dunkel schaltete, alle anderen mit um.
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

// Kantenlänge des gespeicherten Bildes. 256 statt 128, damit das Bild auf
// hochauflösenden Bildschirmen und im Profil scharf bleibt.
const KANTE = 256;

type Teil = "profil" | "aussehen" | "passwort" | "zweitfaktor" | "geraete";

const TEILE: { id: Teil; titel: string; unter: string }[] = [
  { id: "profil", titel: "Profil", unter: "Name und Bild" },
  { id: "aussehen", titel: "Aussehen", unter: "Grundton und Akzent" },
  { id: "passwort", titel: "Passwort", unter: "Wechseln" },
  { id: "zweitfaktor", titel: "Zweiter Faktor", unter: "App und Ersatzcodes" },
  { id: "geraete", titel: "Geräte", unter: "Wo du angemeldet bist" },
];

/**
 * Mittig auf ein Quadrat beschneiden und verkleinern: das Bild erscheint als
 * Kreis, und ein verzerrtes Gesicht ist schlimmer als ein beschnittenes. Ein
 * Foto aus der Kamera hat mehrere Megabyte und wird als kleiner Kreis gezeigt --
 * im Browser zu verkleinern spart Leitung und Platte.
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
      // JPEG und nicht PNG: ein Foto als PNG wäre um ein Vielfaches größer.
      // Güte 0,9 ist bei dieser Größe nicht von 1,0 zu unterscheiden.
      (b) => (b ? fertig(b) : gescheitert(new Error("Das Bild ließ sich nicht erzeugen."))),
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
      titel="Mein Konto"
      unter={user.email}
      breit
      schliessen={onClose}
      fuss={
        <>
          <span className="fuss-luecke" />
          <button className="btn btn-primary" onClick={onClose}>
            Schließen
          </button>
        </>
      }
    >
      {/* Dieselbe Aufteilung wie in der Verwaltung: eine Leiste links, der
          Inhalt rechts. Vier Einträge sind wenig für eine Leiste -- aber sie
          machen sichtbar, was es überhaupt gibt, und das war vorher die Frage:
          den zweiten Faktor findet niemand, der nicht weiß, dass es ihn gibt. */}
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
 * Grundton und Akzent, für dieses Konto allein.
 *
 * Gespeichert wird sofort beim Klick und ohne Knopf: eine Farbe sucht man
 * aus, indem man sie sieht, und ein Speichern-Knopf dazwischen macht aus dem
 * Ausprobieren eine Kette von Bestätigungen. Angewandt wird schon vor der
 * Antwort des Servers, sonst blinkt die Oberfläche der Auswahl hinterher.
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
      // Bleibt die Wahl ungespeichert, steht sie trotzdem schon auf dem
      // Bildschirm. Der Satz sagt, dass sie den Neustart nicht überlebt.
      .catch((e) => setFehler(e instanceof Error ? e.message : "Nicht gespeichert."));
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
      <h3>Grundton</h3>
      <p className="muted small">
        Gilt für dieses Konto, auf jedem Gerät, an dem du angemeldet bist.
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

      <h3>Akzentfarbe</h3>
      <p className="muted small">
        Färbt Knöpfe, Verknüpfungen und die Markierung in Listen.
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
        {/* Eine Hausfarbe kommt als Hexwert aus einem Handbuch und nicht aus
            dem Farbrad des Betriebssystems. Deshalb das Feld, der Wähler nur
            daneben. */}
        <input
          className="hex-feld"
          value={akzent}
          spellCheck={false}
          maxLength={7}
          aria-label="Eigener Wert"
          placeholder="#2383e2"
          onChange={(ev) => {
            const w = ev.target.value.trim().toLowerCase();
            setAkzent(w);
            if (/^#[0-9a-f]{6}$/.test(w)) anwenden({ grundton: ton, akzent: w });
          }}
          onBlur={() => {
            // Ein halb getippter Wert darf nicht in die Datenbank.
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
          aria-label="Farbwähler"
          value={/^#[0-9a-f]{6}$/.test(akzent) ? akzent : "#2383e2"}
          onChange={(ev) => akzentSetzen(ev.target.value.toLowerCase(), false)}
          onBlur={() => {
            if (akzent !== design.akzent.toLowerCase()) sichern(ton, akzent);
          }}
        />
      </div>

      {/* Die Probe steht anstelle einer Spalte mit Kontrastzahlen. Wer eine
          Farbe aussucht, sieht hier, was sie anrichtet; nachrechnen muss er es
          nicht. */}
      <div className="wirkprobe">
        <button className="btn btn-primary" type="button">
          Primärer Knopf
        </button>
        <button className="btn" type="button">
          Sekundärer Knopf
        </button>
        <a
          className="wirkprobe-verweis"
          href="#aussehen"
          onClick={(e) => e.preventDefault()}
        >
          Verknüpfung im Fließtext
        </a>
        <span className="wirkprobe-zeile">Ausgewählter Eintrag</span>
      </div>

      {!flaecheLesbar(akzent) && (
        <p className="muted small">
          Auf dieser Fläche ist die Beschriftung schwer zu lesen. Ein dunklerer oder
          hellerer Wert derselben Farbe hilft.
        </p>
      )}
      {verschoben && (
        <p className="muted small">
          Als Text auf dem Grund wird die Farbe nach <code>{verschoben}</code> gerückt,
          sonst wäre eine Verknüpfung im Fließtext nicht zu lesen. Flächen behalten den
          eingetragenen Wert.
        </p>
      )}
      {fehler && <div className="fehlertext small">{fehler}</div>}
    </>
  );
}

/** Name und Bild. Die Adresse steht daneben und ist nicht änderbar. */
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
      setMeldung("Bild gespeichert.");
    } catch (e) {
      setFehler((e as Error).message);
    } finally {
      setLaeuft(null);
      // Das Feld zurücksetzen, damit dieselbe Datei erneut gewählt werden kann.
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
    <>
      <h3>Profil</h3>
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
        Das Bild wird auf {KANTE} × {KANTE} zugeschnitten und verkleinert, bevor es hochgeht
        — mittig, weil es als Kreis erscheint. Sichtbar ist es für jeden, der hier angemeldet
        ist.
      </p>

      <div className="fenster-abschnitt">
        <div className="modal-label">Angezeigter Name</div>
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
            {laeuft === "name" ? "Speichert…" : "Speichern"}
          </button>
        </div>
        <p className="muted small">
          So stehst du an Seiten, Kommentaren und in Freigabelisten. Die E-Mail-Adresse ist
          die Kennung des Kontos und lässt sich hier nicht ändern — daran hängen die
          Anmeldung und jede Freigabe.
        </p>
      </div>

      {fehler && <div className="fehler">{fehler}</div>}
      {meldung && !fehler && <div className="hinweis-ok">{meldung}</div>}
    </>
  );
}

/**
 * Das eigene Passwort wechseln.
 *
 * Drei Felder statt zwei: das neue zu wiederholen fängt den Tippfehler, der
 * sonst erst bei der nächsten Anmeldung auffiele -- und dann nicht mehr zu
 * beheben wäre, ohne die Verwaltung zu fragen.
 */
function Passwort() {
  const [alt, setAlt] = useState("");
  const [neu, setNeu] = useState("");
  const [nochmal, setNochmal] = useState("");
  const [fehler, setFehler] = useState("");
  const [fertig, setFertig] = useState<string | null>(null);
  const [laeuft, setLaeuft] = useState(false);

  // Dieselben Grenzen wie im Dienst (backend/internal/handlers/passwort.go).
  // Die Prüfung hier ist nur für die sofortige Rückmeldung da; entschieden wird
  // es am Server.
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
    <>
      <h3>Passwort ändern</h3>
      {fertig ? (
        <>
          <div className="hinweis-ok">{fertig}</div>
          <div className="knopfreihe">
            <button className="btn" onClick={() => setFertig(null)}>
              Noch einmal ändern
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="muted small">
            Das bisherige Passwort wird verlangt, obwohl du angemeldet bist. Es geht nicht
            darum, wer du bist, sondern um den Fall, dass jemand anders vor deinem offenen
            Browser sitzt. Nach dem Wechsel werden alle anderen Sitzungen beendet, dieses
            Gerät bleibt angemeldet.
          </p>
          <div className="fenster-felder">
            <label className="feld-breit">
              <span>Bisheriges Passwort</span>
              <input
                type="password"
                autoComplete="current-password"
                value={alt}
                onChange={(e) => setAlt(e.target.value)}
              />
            </label>
            <label>
              <span>Neues Passwort</span>
              <input
                type="password"
                autoComplete="new-password"
                value={neu}
                onChange={(e) => setNeu(e.target.value)}
              />
            </label>
            <label>
              <span>Noch einmal</span>
              <input
                type="password"
                autoComplete="new-password"
                value={nochmal}
                onChange={(e) => setNochmal(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && senden()}
              />
            </label>
          </div>
          {zuKurz && <div className="fehler">Mindestens 6 Zeichen.</div>}
          {zuLang && (
            <div className="fehler">
              Zu lang. Mehr als 72 Zeichen liest die Prüfung nicht, der Rest fiele
              stillschweigend weg.
            </div>
          )}
          {passtNicht && <div className="fehler">Die beiden Eingaben sind verschieden.</div>}
          {fehler && <div className="fehler">{fehler}</div>}
          <div className="knopfreihe">
            <button className="btn" disabled={!bereit} onClick={senden}>
              {laeuft ? "Wechselt…" : "Passwort ändern"}
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
      <h3>Zweiter Faktor</h3>
      {stand === null ? (
        <p className="muted small">Wird geladen…</p>
      ) : (
        <Zweitfaktor stand={stand} neuLaden={laden} />
      )}
    </>
  );
}

/**
 * Die eigenen Sitzungen.
 *
 * Sie standen bisher nur in der Verwaltung, also für Administratoren -- dabei
 * ist die Liste ohnehin die eigene: der Dienst gibt jedem nur seine.
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
        titel: s.diese ? "Hier abmelden" : "Sitzung beenden",
        text: s.diese
          ? "Das ist die Sitzung, mit der du gerade arbeitest. Nach dem Beenden musst du dich neu anmelden."
          : `Die Sitzung auf ${s.browser} wird sofort ungültig. Wer sie benutzt, landet auf der Anmeldeseite.`,
        bestaetigen: "Beenden",
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
        titel="Angemeldete Geräte"
        zahl={
          sitzungen === null
            ? "wird geladen"
            : `${sitzungen.length} ${sitzungen.length === 1 ? "Sitzung" : "Sitzungen"}`
        }
        filter={filter}
        setFilter={setFilter}
        platzhalter="Filtern nach Gerät oder Adresse"
      >
        {(sitzungen?.length ?? 0) > 1 && (
          <button
            className="btn"
            onClick={async () => {
              if (
                !(await frage({
                  titel: "Überall sonst abmelden",
                  text: "Alle anderen Sitzungen werden beendet. Diese hier bleibt bestehen.",
                  bestaetigen: "Alle anderen beenden",
                  gefaehrlich: true,
                }))
              )
                return;
              await api.sitzungenBeenden().catch(() => {});
              laden();
            }}
          >
            Überall sonst abmelden
          </button>
        )}
      </Listenkopf>
      <p className="muted small">
        Findest du hier ein Gerät, das dir nicht gehört, beende die Sitzung und ändere
        anschließend dein Passwort — beendet ist sie sofort, aber wer das Passwort hat, meldet
        sich sonst neu an.
      </p>
      <table className="tabelle">
        <thead>
          <tr>
            <th>Gerät</th>
            <th>Adresse</th>
            <th>Zuletzt</th>
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
                {new Date(s.zuletztAm).toLocaleString("de-DE", {
                  day: "2-digit",
                  month: "2-digit",
                  year: "numeric",
                  hour: "2-digit",
                  minute: "2-digit",
                })}
              </td>
              <td className="zeilen-aktionen">
                <button className="btn-schlicht gefaehrlich" onClick={() => beenden(s)}>
                  Beenden
                </button>
              </td>
            </tr>
          ))}
          {sichtbar.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                {sitzungen === null
                  ? "Wird geladen…"
                  : sitzungen.length === 0
                    ? "Keine gespeicherte Sitzung."
                    : "Keine Sitzung passt auf den Filter."}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}
