// The settings manager, for administrators.
//
// Laid out like a settings application rather than one long page: categories on
// the left, one topic at a time on the right. With a dozen settings and four
// times as many facts about the system, a single scroll would bury everything
// that is not at the top.
//
// One distinction runs through the whole page and matters more than any single
// field: some values can be changed while the server runs, others are fixed at
// start. Mixing them would produce switches that quietly do nothing, so the
// fixed ones are always marked as belonging to config.conf.
import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Einstellung, KonfigDatei, Sitzung, SystemZustand, api } from "../api/client";
import { useAuth } from "../auth";
import { useLizenz } from "../lizenz";
import AdminView from "./AdminView";
import GruppenView from "./GruppenView";
import { anwenden, useDesign } from "../design";
import { useRueckfrage } from "../components/Rueckfrage";

type Bereich =
  | "uebersicht"
  | "nutzer"
  | "gruppen"
  | "sicherheit"
  | "sitzungen"
  | "datenbank"
  | "suche"
  | "anhaenge"
  | "aussehen"
  | "lizenz"
  | "system"
  | "wartung";

const BEREICHE: { id: Bereich; titel: string; unter: string }[] = [
  { id: "uebersicht", titel: "Übersicht", unter: "Zahlen und Zustand auf einen Blick" },
  // Accounts and groups each used to have an entry of their own in the sidebar.
  // Both are administration and are rarely touched; they belong where one looks
  // anyway when one wants to set something.
  { id: "nutzer", titel: "Nutzer", unter: "Konten, Rollen, Zugänge" },
  { id: "gruppen", titel: "Gruppen", unter: "Konten bündeln für Space-Rechte" },
  { id: "sicherheit", titel: "Sicherheit", unter: "Registrierung, Laufzeiten, Administratoren" },
  { id: "sitzungen", titel: "Sitzungen", unter: "Angemeldete Geräte, einzeln beendbar" },
  { id: "datenbank", titel: "Datenbank", unter: "Größe, Tabellen, Belegung" },
  { id: "suche", titel: "Suche", unter: "Wörterbuch und Suchindex" },
  { id: "anhaenge", titel: "Anhänge", unter: "Größenbegrenzung und Belegung" },
  { id: "aussehen", titel: "Aussehen", unter: "Grundton und Akzentfarbe" },
  { id: "lizenz", titel: "Lizenz", unter: "Umfang und Laufzeit" },
  { id: "system", titel: "System", unter: "Was nur beim Start gilt" },
  { id: "wartung", titel: "Wartung", unter: "Konfigurationsdatei, Neustart, Aufräumen" },
];

const ZUSATZ: Record<string, string> = {
  versionen: "Versionsverlauf",
  anhaenge: "Anhänge",
  freigeben: "Teilen und öffentliche Links",
  pruefspur: "Protokoll",
  gruppen: "Gruppen und Space-Rechte",
  sso: "SSO über OIDC",
  ldap: "LDAP und Active Directory",
  anhangsuche: "Volltext in Anhängen",
  export: "Space-Export",
  vorlagen: "Vorlagen",
  kommentare: "Kommentare",
  konflikte: "Konflikterkennung",
};

const ZAHL_TITEL: Record<string, string> = {
  konten: "Konten",
  admins: "Administratoren",
  seiten: "Seiten",
  papierkorb: "im Papierkorb",
  versionen: "Versionen",
  anhaenge: "Anhänge",
  kommentare: "Kommentare",
  spureintraege: "Protokoll-Einträge",
  ohneSuchtext: "ohne Suchtext",
};

const GRUNDTOENE: { wert: string; titel: string; erklaerung: string }[] = [
  { wert: "grau", titel: "Gegrautes Weiß", erklaerung: "Vorgabe. Nimmt dem reinen Weiß die Härte, ohne dunkel zu wirken." },
  { wert: "weiss", titel: "Reines Weiß", erklaerung: "Maximaler Kontrast. Auf großen Bildschirmen auf Dauer anstrengend." },
  { wert: "dunkel", titel: "Dunkel", erklaerung: "Für dunkle Räume und lange Sitzungen." },
];

const AKZENTE = [
  { wert: "#2383e2", titel: "Blau" },
  { wert: "#2ea043", titel: "Grün" },
  { wert: "#8250df", titel: "Violett" },
  { wert: "#bf5b04", titel: "Bernstein" },
  { wert: "#cf222e", titel: "Rot" },
  { wert: "#57606a", titel: "Graphit" },
];

function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

// A point in time as one reads it out: date and clock time, without seconds and
// without a time zone; the question is "when was that me", not "what time was it
// in UTC".
function zeitpunkt(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString("de-DE", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function EinstellungenView() {
  const frage = useRueckfrage();
  const { user } = useAuth();
  const { neuLaden: designNeuLaden } = useDesign();

  // The open section stands in the address, not in the state. That way a section
  // can be linked, the back button leads into the previous one, and the old
  // addresses for users and groups can point here.
  const nav = useNavigate();
  const { bereich: ausAdresse } = useParams();
  const bereich: Bereich = (BEREICHE.some((b) => b.id === ausAdresse)
    ? ausAdresse
    : "uebersicht") as Bereich;
  const setBereich = (b: Bereich) => nav("/einstellungen/" + b);
  const [liste, setListe] = useState<Einstellung[]>([]);
  const [zustand, setZustand] = useState<SystemZustand | null>(null);
  const [entwurf, setEntwurf] = useState<Record<string, string>>({});
  const [meldung, setMeldung] = useState<{ text: string; art: "ok" | "fehler" } | null>(null);
  const [laeuft, setLaeuft] = useState<string | null>(null);
  const [laedt, setLaedt] = useState(true);

  // Object storage test. The credentials stay in this form exclusively, nothing
  // of it is saved; see the note in the section.
  const [ablage, setAblage] = useState<string>("");
  const [s3, setS3] = useState({
    endpunkt: "",
    bucket: "nexora",
    zugriff: "",
    geheimnis: "",
    region: "us-east-1",
    tls: false,
    pfadstil: true,
  });
  const [s3Ergebnis, setS3Ergebnis] = useState<{
    ok: boolean;
    text: string;
  } | null>(null);

  const laden = useCallback(() => {
    setLaedt(true);
    Promise.all([api.einstellungen(), api.systemZustand()])
      .then(([e, z]) => {
        setListe(e);
        setZustand(z);
        setEntwurf(Object.fromEntries(e.map((x) => [x.schluessel, x.wert])));
      })
      .catch((err: Error & { status?: number }) =>
        setMeldung({
          text: err.status === 403 ? "Nur für Administratoren." : err.message,
          art: "fehler",
        }),
      )
      .finally(() => setLaedt(false));
  }, []);

  useEffect(laden, [laden]);

  useEffect(() => {
    api.ablageZustand().then((a) => setAblage(a.ablage)).catch(() => setAblage(""));
  }, []);

  // Maintenance. The file is fetched only when the section is opened: it
  // contains credentials, masked though they are, and is none of the business of
  // somebody who only wanted to change the colours.
  const [konfig, setKonfig] = useState<KonfigDatei | null>(null);
  const [konfigEntwurf, setKonfigEntwurf] = useState("");
  const [konfigHinweise, setKonfigHinweise] = useState<string[]>([]);
  const [neustartWort, setNeustartWort] = useState("");

  // Lizenz: einlesen und, beim Herausgeber, ausstellen.
  const { lizenz: lizenzJetzt, neuLaden: lizenzNeuLaden } = useLizenz();
  const [schluesselFeld, setSchluesselFeld] = useState("");
  const [ausstellen, setAusstellen] = useState({
    inhaber: "",
    stufe: "pro",
    ablauf: "",
  });
  const [ausgestellt, setAusgestellt] = useState("");

  // Sessions. Fetched only when the section is opened: the list changes
  // constantly, and it interests only whoever is looking right now.
  const [sitzungen, setSitzungen] = useState<Sitzung[] | null>(null);
  const sitzungenLaden = useCallback(() => {
    api
      .sitzungen()
      .then(setSitzungen)
      .catch(() => setSitzungen([]));
  }, []);
  useEffect(() => {
    if (bereich === "sitzungen") sitzungenLaden();
  }, [bereich, sitzungenLaden]);

  useEffect(() => {
    if (bereich !== "wartung" || konfig) return;
    api
      .konfigLesen()
      .then((k) => {
        setKonfig(k);
        setKonfigEntwurf(k.inhalt);
        setKonfigHinweise(k.hinweise);
      })
      .catch((err: Error) => setMeldung({ text: err.message, art: "fehler" }));
  }, [bereich, konfig]);

  const konfigPruefen = async () => {
    setLaeuft("konfig-pruefen");
    try {
      const r = await api.konfigPruefen(konfigEntwurf);
      setKonfigHinweise(r.hinweise);
      setMeldung(
        r.hinweise.length === 0
          ? { text: "Der Entwurf ist in Ordnung.", art: "ok" }
          : { text: `${r.hinweise.length} Auffälligkeit(en). Sie stehen unten.`, art: "fehler" },
      );
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const konfigSpeichern = async () => {
    setLaeuft("konfig-speichern");
    try {
      const r = await api.konfigSchreiben(konfigEntwurf);
      setKonfigHinweise(r.hinweise);
      setMeldung({
        text: `Gespeichert. Sicherung: ${r.sicherung}. Wirksam wird die Änderung erst nach einem Neustart.`,
        art: "ok",
      });
      // Fetch anew: the answer does not contain the written state, and the draft
      // in the field would otherwise keep showing the asterisks that are real
      // values again by now.
      setKonfig(null);
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const schluesselEinlesen = async () => {
    setLaeuft("lizenz");
    try {
      const z = await api.lizenzEinlesen(schluesselFeld.trim());
      lizenzNeuLaden();
      laden();
      setSchluesselFeld("");
      setMeldung({
        text: z.gueltig
          ? `Lizenz für ${z.inhaber} übernommen${z.stufe ? ` (Stufe ${z.stufe})` : ""}.`
          : "Lizenz entfernt. Es gilt wieder der freie Umfang.",
        art: "ok",
      });
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const schluesselAusstellen = async () => {
    setLaeuft("ausstellen");
    setAusgestellt("");
    try {
      const r = await api.lizenzAusstellen({
        inhaber: ausstellen.inhaber.trim(),
        stufe: ausstellen.stufe,
        ablauf: ausstellen.ablauf || undefined,
      });
      setAusgestellt(r.schluessel);
      setMeldung({ text: "Schlüssel ausgestellt.", art: "ok" });
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const neustarten = async () => {
    setLaeuft("neustart");
    try {
      await api.neustarten();
      setMeldung({
        text: "Der Dienst wird beendet. Kommt er nicht von selbst wieder, startet ihn nichts neu. Dann hilft nur der Container-Verwalter.",
        art: "ok",
      });
      setNeustartWort("");
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const papierkorbLeeren = async () => {
    if (
      !(await frage({
        titel: "Papierkorb leeren",
        text: "Alle Seiten im Papierkorb dieser Instanz werden endgültig gelöscht, auch die anderer Konten. Das lässt sich nicht rückgängig machen.",
        bestaetigen: "Papierkorb leeren",
        gefaehrlich: true,
      }))
    ) {
      return;
    }
    setLaeuft("papierkorb");
    try {
      const r = await api.papierkorbLeeren();
      setMeldung({ text: `${r.geloescht} Seite(n) endgültig gelöscht.`, art: "ok" });
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const ablageTesten = async () => {
    setLaeuft("ablage");
    setS3Ergebnis(null);
    try {
      const r = await api.ablageTesten(s3);
      setS3Ergebnis(
        r.ok
          ? { ok: true, text: `Verbindung steht: ${r.ablage}${r.anmerkung ? ` (${r.anmerkung})` : ""}` }
          : { ok: false, text: `Fehlgeschlagen beim ${r.schritt}: ${r.grund}` },
      );
    } catch (err) {
      setS3Ergebnis({ ok: false, text: (err as Error).message });
    } finally {
      setLaeuft(null);
    }
  };

  const holen = (schluessel: string) => liste.find((e) => e.schluessel === schluessel);

  const speichern = async (e: Einstellung, wert: string) => {
    setLaeuft(e.schluessel);
    setMeldung(null);
    try {
      await api.einstellungSetzen(e.schluessel, wert);
      setMeldung({ text: `„${e.titel}“ gespeichert.`, art: "ok" });
      if (e.schluessel.startsWith("design_")) designNeuLaden();
      laden();
    } catch (err) {
      setMeldung({ text: (err as Error).message, art: "fehler" });
      setEntwurf((v) => ({ ...v, [e.schluessel]: e.wert }));
      // Take the preview back, otherwise the interface shows a colour the server
      // never accepted.
      if (e.schluessel.startsWith("design_")) designNeuLaden();
    } finally {
      setLaeuft(null);
    }
  };

  const zuruecksetzen = async (e: Einstellung) => {
    setLaeuft(e.schluessel);
    try {
      await api.einstellungZuruecksetzen(e.schluessel);
      setMeldung({ text: `„${e.titel}“ folgt wieder der config.conf.`, art: "ok" });
      if (e.schluessel.startsWith("design_")) designNeuLaden();
      laden();
    } catch (err) {
      setMeldung({ text: (err as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const anhangindex = async () => {
    setLaeuft("anhangindex");
    setMeldung(null);
    try {
      const r = await api.anhangindexNachziehen();
      setMeldung({
        text:
          r.betrachtet === 0
            ? "Alle Anhänge haben bereits einen Suchtext."
            : `${r.betrachtet} Anhänge betrachtet, ${r.gelesen} mit Text, ${r.ohneText} ohne — ` +
              `das sind Bilder, Archive oder gescannte PDF ohne Textebene.`,
        art: "ok",
      });
      laden();
    } catch (err) {
      setMeldung({ text: (err as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const indexNeu = async () => {
    setLaeuft("suchindex");
    setMeldung(null);
    try {
      const r = await api.suchindexNeu();
      setMeldung({
        text:
          r.ohneSuchtext === 0
            ? "Suchindex neu aufgebaut, alle Seiten erfasst."
            : `Suchindex neu aufgebaut. ${r.ohneSuchtext} Seiten ohne Text — das sind in aller Regel leere Seiten.`,
        art: "ok",
      });
      laden();
    } catch (err) {
      setMeldung({ text: (err as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  if (user?.role !== "admin") {
    return (
      <div className="page-pad">
        <h2>Verwaltung</h2>
        <p className="muted">Diese Seite ist Administratoren vorbehalten.</p>
      </div>
    );
  }
  if (laedt && !zustand) return <div className="page-pad muted">Lädt…</div>;

  // ── Bausteine ───────────────────────────────────────────────────────────

  const feld = (schluessel: string) => {
    const e = holen(schluessel);
    if (!e) return null;
    const w = entwurf[e.schluessel] ?? e.wert;
    const geaendert = w !== e.wert;

    return (
      <div className="einstellung" key={e.schluessel}>
        <div className="einstellung-kopf">
          <div>
            <div className="einstellung-titel">{e.titel}</div>
            <div className="einstellung-erklaerung">{e.erklaerung}</div>
            {e.warnung && <div className="einstellung-warnung">{e.warnung}</div>}
          </div>

          <div className="einstellung-feld">
            {e.art === "janein" ? (
              <label className="schalter">
                <input
                  type="checkbox"
                  checked={w === "ja"}
                  disabled={laeuft === e.schluessel}
                  onChange={(ev) => speichern(e, ev.target.checked ? "ja" : "nein")}
                />
                <span>{w === "ja" ? "an" : "aus"}</span>
              </label>
            ) : (
              <>
                <input
                  type={e.art === "zahl" ? "number" : "text"}
                  value={w}
                  disabled={laeuft === e.schluessel}
                  placeholder={e.art === "liste" ? "leer = keine Einschränkung" : undefined}
                  onChange={(ev) => setEntwurf((v) => ({ ...v, [e.schluessel]: ev.target.value }))}
                  onKeyDown={(ev) => {
                    if (ev.key === "Enter" && geaendert) speichern(e, w);
                  }}
                />
                {geaendert && (
                  <button className="btn" onClick={() => speichern(e, w)}>
                    Speichern
                  </button>
                )}
              </>
            )}
          </div>
        </div>
        {herkunft(e)}
      </div>
    );
  };

  const herkunft = (e: Einstellung) => (
    <div className="einstellung-fuss muted small">
      {e.ausDatei ? (
        <>
          Wert stammt aus <code>config.conf</code>
        </>
      ) : (
        <>
          hier gesetzt
          {e.geaendertVon && <> von {e.geaendertVon}</>}
          {e.geaendertAm && <> am {e.geaendertAm}</>}
          {" · "}
          <button className="link-btn" onClick={() => zuruecksetzen(e)}>
            zurücksetzen auf {e.vorgabe || "leer"}
          </button>
        </>
      )}
    </div>
  );

  const kachel = (wert: React.ReactNode, titel: string, key?: string) => (
    <div className="kachel" key={key ?? titel}>
      <div className="kachel-wert">{wert}</div>
      <div className="kachel-titel">{titel}</div>
    </div>
  );

  const z = zustand!;
  const sich = z.sicherheit;

  // ── Bereiche ────────────────────────────────────────────────────────────

  const inhalt = () => {
    switch (bereich) {
      case "sitzungen":
        return (
          <>
            <h3>Angemeldete Geräte</h3>
            <p className="muted small">
              Jede Anmeldung steht als Zeile in der Datenbank. Deshalb lässt sich eine
              einzelne beenden, ein verlorenes Gerät auszusperren, ohne alle anderen
              mitzunehmen, geht nur so. Wer täglich arbeitet, bleibt angemeldet: eine
              Sitzung, die mehr als die Hälfte ihrer Zeit hinter sich hat, wird beim
              nächsten Aufruf verlängert.
            </p>
            {sitzungen === null ? (
              <p className="muted">Wird geladen…</p>
            ) : sitzungen.length === 0 ? (
              <p className="muted">Keine gespeicherte Sitzung.</p>
            ) : (
              <table className="tabelle">
                <thead>
                  <tr>
                    <th>Gerät</th>
                    <th>Adresse</th>
                    <th>Zuletzt</th>
                    <th>Läuft ab</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {sitzungen.map((si) => (
                    <tr key={si.id}>
                      <td>
                        {si.browser}
                        {si.diese && (
                          <span className="pill frei" style={{ marginLeft: 8 }}>
                            dieses Gerät
                          </span>
                        )}
                      </td>
                      <td className="muted small">{si.ip || "—"}</td>
                      <td className="muted small">{zeitpunkt(si.zuletztAm)}</td>
                      <td className="muted small">{zeitpunkt(si.laeuftAb)}</td>
                      <td>
                        <button
                          className="btn"
                          onClick={async () => {
                            if (
                              !(await frage({
                                titel: si.diese ? "Hier abmelden" : "Sitzung beenden",
                                text: si.diese
                                  ? "Diese Sitzung ist die, mit der du gerade arbeitest. Nach dem Beenden musst du dich neu anmelden."
                                  : `Die Sitzung auf ${si.browser} wird sofort ungültig. Wer sie benutzt, landet auf der Anmeldeseite.`,
                                bestaetigen: "Beenden",
                                gefaehrlich: !si.diese,
                              }))
                            )
                              return;
                            await api.sitzungBeenden(si.id).catch(() => {});
                            if (si.diese) window.location.href = "/login";
                            else sitzungenLaden();
                          }}
                        >
                          Beenden
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            {(sitzungen?.length ?? 0) > 1 && (
              <div className="knopfreihe">
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
                    const r = await api.sitzungenBeenden().catch(() => null);
                    if (r) setMeldung({ text: `${r.beendet} Sitzung(en) beendet.`, art: "ok" });
                    sitzungenLaden();
                  }}
                >
                  Überall sonst abmelden
                </button>
              </div>
            )}
          </>
        );

      case "nutzer":
        return <AdminView />;
      case "gruppen":
        return <GruppenView />;
      case "uebersicht":
        return (
          <>
            <h3>Übersicht</h3>
            <div className="kachelreihe">
              {Object.entries(z.zahlen ?? {}).map(([k, v]) => kachel(v, ZAHL_TITEL[k] ?? k, k))}
              {kachel(z.datenbank?.groesse ?? "—", "Datenbank")}
              {kachel(bytes(z.anhaengeBytes ?? 0), "Anhänge auf Platte")}
            </div>

            <h3>Kurz zusammengefasst</h3>
            <table className="tabelle">
              <tbody>
                <tr>
                  <td>Lizenz</td>
                  <td>
                    {z.lizenz.gueltig ? (
                      <>
                        {z.lizenz.inhaber} — {(z.lizenz.freigeschaltet ?? []).length} von {z.lizenz.alle} Zusätzen
                      </>
                    ) : (
                      <span className="muted">keine gültige Lizenz</span>
                    )}
                  </td>
                </tr>
                <tr>
                  <td>Selbstregistrierung</td>
                  <td>{sich?.registrierungOffen ? "offen" : "geschlossen"}</td>
                </tr>
                <tr>
                  <td>Letzte Anmeldung</td>
                  <td>{sich?.letzteAnmeldung || <span className="muted">keine verzeichnet</span>}</td>
                </tr>
                <tr>
                  <td>Fehlversuche letzte 24 h</td>
                  <td>{sich?.fehlversuche24h ?? 0}</td>
                </tr>
                <tr>
                  <td>PostgreSQL</td>
                  <td>{z.datenbank?.version || "—"}</td>
                </tr>
              </tbody>
            </table>
          </>
        );

      case "sicherheit":
        return (
          <>
            <h3>Zugang</h3>
            {feld("registrierung_offen")}
            {feld("erlaubte_domaenen")}
            {feld("sitzung_stunden")}

            <h3>Administratoren</h3>
            <p className="muted small">
              Diese Konten dürfen jede Seite lesen und bearbeiten, auch fremde. Das ist keine
              Nebenwirkung, sondern Absicht — es sollte trotzdem jeder hier stehen, von dem
              du es erwartest.
            </p>
            <table className="tabelle">
              <tbody>
                {(sich?.admins ?? []).map((a) => (
                  <tr key={a.email}>
                    <td>{a.name}</td>
                    <td className="muted">{a.email}</td>
                  </tr>
                ))}
              </tbody>
            </table>

            <h3>Anmeldungen</h3>
            <div className="kachelreihe">
              {kachel(sich?.fehlversuche24h ?? 0, "Fehlversuche 24 h")}
              {kachel(sich?.letzteAnmeldung || "—", "Letzte Anmeldung")}
              {kachel(sich?.letzterFehlversuch || "—", "Letzter Fehlversuch")}
            </div>
            <p className="muted small">
              Vollständig nachlesbar unter <strong>Protokoll</strong>. Passwörter werden dort
              nie festgehalten, auch nicht bei einem Fehlversuch.
            </p>
          </>
        );

      case "datenbank":
        return (
          <>
            <h3>Datenbank</h3>
            <div className="kachelreihe">
              {kachel(z.datenbank?.groesse ?? "—", "Gesamtgröße")}
              {kachel(z.datenbank?.version || "—", "PostgreSQL")}
              {kachel(bytes(z.anhaengeBytes ?? 0), "Anhänge (auf Platte)")}
            </div>
            <p className="muted small">
              Anhänge liegen als Dateien im Datenverzeichnis, nicht in der Datenbank. Eine
              Sicherung nur der Datenbank ist deshalb keine vollständige.
            </p>

            <h3>Größte Tabellen</h3>
            <table className="tabelle">
              <thead>
                <tr>
                  <th>Tabelle</th>
                  <th>Zeilen</th>
                  <th>Belegt</th>
                </tr>
              </thead>
              <tbody>
                {(z.datenbank?.tabellen ?? []).map((t) => (
                  <tr key={t.name}>
                    <td>
                      <code>{t.name}</code>
                    </td>
                    <td>{t.zeilen}</td>
                    <td>{t.platz}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="muted small">
              Die Zeilenzahl ist eine Schätzung aus der Statistik von PostgreSQL, nicht
              gezählt — deshalb kann sie kurz nach vielen Änderungen etwas danebenliegen.
            </p>
          </>
        );

      case "suche":
        return (
          <>
            <h3>Volltextsuche</h3>
            {feld("such_woerterbuch")}

            <div className="einstellung">
              <div className="einstellung-kopf">
                <div>
                  <div className="einstellung-titel">Suchindex neu aufbauen</div>
                  <div className="einstellung-erklaerung">
                    Zieht den Fließtext aller Seiten neu aus dem Editor-Inhalt. Nötig nach
                    einem Wechsel des Wörterbuchs, und sinnvoll, wenn die Suche etwas nicht
                    findet, das offensichtlich dasteht.
                  </div>
                  <div className="einstellung-warnung">
                    Läuft in Stapeln und blockiert nichts, kann bei vielen Seiten aber einen
                    Moment dauern.
                  </div>
                </div>
                <div className="einstellung-feld">
                  <button className="btn" disabled={laeuft === "suchindex"} onClick={indexNeu}>
                    {laeuft === "suchindex" ? "Läuft…" : "Neu aufbauen"}
                  </button>
                </div>
              </div>
            </div>

            <div className="einstellung">
              <div className="einstellung-kopf">
                <div>
                  <div className="einstellung-titel">Volltext aus Anhängen nachziehen</div>
                  <div className="einstellung-erklaerung">
                    Liest Anhänge, die noch keinen Suchtext haben, und gewinnt ihn nach.
                    Betrifft alles, was vor dieser Funktion hochgeladen wurde. Gelesen
                    werden Textdateien und PDF mit Textebene.
                  </div>
                  <div className="einstellung-warnung">
                    Holt jede Datei einmal aus der Ablage zurück — bei einem Objektspeicher
                    also übers Netz. Deshalb ein bewusster Knopf und nichts, was beim Start
                    von allein läuft. Je Durchlauf werden bis zu 500 Anhänge bearbeitet.
                  </div>
                </div>
                <div className="einstellung-feld">
                  <button
                    className="btn"
                    disabled={laeuft === "anhangindex"}
                    onClick={anhangindex}
                  >
                    {laeuft === "anhangindex" ? "Läuft…" : "Nachziehen"}
                  </button>
                </div>
              </div>
            </div>

            <div className="kachelreihe">
              {kachel(z.zahlen?.seiten ?? 0, "Seiten im Index")}
              {kachel(z.zahlen?.ohneSuchtext ?? 0, "ohne Suchtext")}
            </div>
            <p className="muted small">
              Seiten ohne Suchtext sind in aller Regel leere Seiten. Bleibt die Zahl nach
              einem Neuaufbau hoch, stimmt etwas mit dem Editor-Inhalt nicht.
            </p>
          </>
        );

      case "anhaenge":
        return (
          <>
            <h3>Anhänge</h3>
            {feld("max_anhang_mb")}
            <div className="kachelreihe">
              {kachel(z.zahlen?.anhaenge ?? 0, "Dateien")}
              {kachel(bytes(z.anhaengeBytes ?? 0), "Belegt")}
            </div>
            <div className="warnkasten">
              <strong>Der nginx davor hat eine eigene Grenze</strong>
              <div className="muted small">
                Ein hier erhöhter Wert bringt nichts, solange <code>client_max_body_size</code>{" "}
                im vorgeschalteten nginx kleiner ist — der bricht die Übertragung dann schon
                ab, bevor Nexora sie überhaupt sieht.
              </div>
            </div>

            <h3>Wo die Dateien liegen</h3>
            <div className="kachelreihe">{kachel(ablage || "—", "Aktuelle Ablage")}</div>
            <p className="muted small">
              Vorgabe ist die lokale Platte. Ein Objektspeicher löst zwei Dinge: ein
              Datenbank-Dump wird zur vollständigen Sicherung, und zwei Instanzen können
              sich dieselben Dateien teilen — ein lokales Verzeichnis können sie nicht.
            </p>

            <h3>Objektspeicher prüfen</h3>
            <p className="muted small">
              Hier wird nichts gespeichert. Die Zugangsdaten bleiben in diesem Formular und
              werden nur für den Test benutzt — ein geheimer Schlüssel gehört in{" "}
              <code>config.conf</code> oder in die Umgebung, nicht in eine Datenbankzeile,
              die jeder Dump mitnimmt.
            </p>
            <div className="einstellung">
              <div className="s3-felder">
                <label>
                  <span>Endpunkt</span>
                  <input
                    placeholder="10.0.2.43:9010"
                    value={s3.endpunkt}
                    onChange={(e) => setS3({ ...s3, endpunkt: e.target.value })}
                  />
                </label>
                <label>
                  <span>Eimer</span>
                  <input value={s3.bucket} onChange={(e) => setS3({ ...s3, bucket: e.target.value })} />
                </label>
                <label>
                  <span>Zugriffsschlüssel</span>
                  <input value={s3.zugriff} onChange={(e) => setS3({ ...s3, zugriff: e.target.value })} />
                </label>
                <label>
                  <span>Geheimnis</span>
                  <input
                    type="password"
                    value={s3.geheimnis}
                    onChange={(e) => setS3({ ...s3, geheimnis: e.target.value })}
                  />
                </label>
                <label>
                  <span>Region</span>
                  <input value={s3.region} onChange={(e) => setS3({ ...s3, region: e.target.value })} />
                </label>
                <label className="schalter">
                  <input
                    type="checkbox"
                    checked={s3.tls}
                    onChange={(e) => setS3({ ...s3, tls: e.target.checked })}
                  />
                  <span>HTTPS</span>
                </label>
                <label className="schalter">
                  <input
                    type="checkbox"
                    checked={s3.pfadstil}
                    onChange={(e) => setS3({ ...s3, pfadstil: e.target.checked })}
                  />
                  <span>Pfadstil (MinIO, Garage)</span>
                </label>
              </div>
              <div className="einstellung-aktionen">
                <button className="btn" disabled={laeuft === "ablage" || !s3.endpunkt} onClick={ablageTesten}>
                  {laeuft === "ablage" ? "Prüft…" : "Verbindung prüfen"}
                </button>
              </div>
              {s3Ergebnis && (
                <div className={s3Ergebnis.ok ? "hinweis-ok" : "fehler"}>{s3Ergebnis.text}</div>
              )}
              <div className="einstellung-fuss muted small">
                Geprüft wird verbinden, schreiben, lesen und löschen — nur zu verbinden würde
                zu wenig verraten. Die häufigsten Fehler zeigen sich erst beim Schreiben.
                Übernommen wird das Ergebnis nicht: dafür die Werte in{" "}
                <code>config.conf</code> eintragen und den Dienst neu starten.
              </div>
            </div>
          </>
        );

      case "aussehen": {
        const grundton = holen("design_grundton");
        const akzent = holen("design_akzent");
        const aktuellerTon = entwurf["design_grundton"] ?? grundton?.wert ?? "grau";
        const aktuellerAkzent = entwurf["design_akzent"] ?? akzent?.wert ?? "#2383e2";

        return (
          <>
            <h3>Grundton</h3>
            <p className="muted small">
              Gilt für alle Konten dieser Instanz. Die Auswahl wird sofort angezeigt und erst
              beim Anklicken gespeichert.
            </p>
            <div className="tonauswahl">
              {GRUNDTOENE.map((g) => (
                <button
                  key={g.wert}
                  className={"tonkachel" + (aktuellerTon === g.wert ? " gewaehlt" : "")}
                  onClick={() => {
                    // Apply right away, then save: seeing a colour only after
                    // the server's answer makes picking one a torment.
                    anwenden({ grundton: g.wert, akzent: aktuellerAkzent });
                    setEntwurf((v) => ({ ...v, design_grundton: g.wert }));
                    if (grundton) speichern(grundton, g.wert);
                  }}
                >
                  <span className={"tonprobe ton-" + g.wert} />
                  <span className="tonkachel-titel">{g.titel}</span>
                  <span className="tonkachel-text">{g.erklaerung}</span>
                </button>
              ))}
            </div>
            {grundton && herkunft(grundton)}

            <h3>Akzentfarbe</h3>
            <p className="muted small">
              Wird für Verknüpfungen, ausgewählte Einträge und Knöpfe benutzt.
            </p>
            <div className="farbauswahl">
              {AKZENTE.map((a) => (
                <button
                  key={a.wert}
                  title={a.titel}
                  className={"farbknopf" + (aktuellerAkzent.toLowerCase() === a.wert ? " gewaehlt" : "")}
                  style={{ background: a.wert }}
                  onClick={() => {
                    anwenden({ grundton: aktuellerTon, akzent: a.wert });
                    setEntwurf((v) => ({ ...v, design_akzent: a.wert }));
                    if (akzent) speichern(akzent, a.wert);
                  }}
                />
              ))}
              <input
                type="color"
                className="farbwaehler"
                value={aktuellerAkzent}
                onChange={(ev) => {
                  anwenden({ grundton: aktuellerTon, akzent: ev.target.value });
                  setEntwurf((v) => ({ ...v, design_akzent: ev.target.value }));
                }}
                onBlur={() => {
                  if (akzent && aktuellerAkzent !== akzent.wert) speichern(akzent, aktuellerAkzent);
                }}
              />
              <code className="muted">{aktuellerAkzent}</code>
            </div>
            {akzent && herkunft(akzent)}
          </>
        );
      }

      case "lizenz":
        return (
          <>
            <h3>Lizenz</h3>
            {z.lizenz.gueltig ? (
              <>
                <div className="kachelreihe">
                  {kachel(z.lizenz.inhaber, "Inhaber")}
                  {kachel(z.lizenz.laeuftAb || "unbefristet", "Gültig bis")}
                  {kachel(`${(z.lizenz.freigeschaltet ?? []).length} von ${z.lizenz.alle}`, "Zusätze frei")}
                </div>
                <h3>Funktionen</h3>
                <table className="tabelle">
                  <tbody>
                    {Object.entries(ZUSATZ).map(([k, titel]) => {
                      const frei = (z.lizenz.freigeschaltet ?? []).includes(k);
                      return (
                        <tr key={k}>
                          <td>{titel}</td>
                          <td>
                            <span className={"pill " + (frei ? "frei" : "gesperrt")}>
                              {frei ? "frei" : "gesperrt"}
                            </span>
                          </td>
                          <td className="muted small">
                            <code>{k}</code>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </>
            ) : (
              <div className="warnkasten">
                <strong>Keine gültige Lizenz</strong>
                <div className="muted small">
                  {z.lizenz.grund || "Kein Schlüssel hinterlegt."} Nexora läuft mit dem freien
                  Umfang; gesperrte Aufrufe antworten mit 402.
                </div>
              </div>
            )}
            <h3>Stufen</h3>
            <p className="muted small">
              Jede Stufe enthält die kleineren mit. Geprüft wird im Code immer gegen die
              einzelne Funktion, nie gegen die Stufe — deshalb kann ein Schlüssel auch eine
              Stufe plus einzelne Zusätze tragen.
            </p>
            <table className="tabelle">
              <tbody>
                {(lizenzJetzt?.stufen ?? []).map((st) => (
                  <tr key={st.name}>
                    <td>
                      <strong>{st.name}</strong>
                      {lizenzJetzt?.stufe === st.name && (
                        <span className="pill frei" style={{ marginLeft: 8 }}>
                          in Betrieb
                        </span>
                      )}
                    </td>
                    <td className="muted small">
                      {st.funktionen.length === 0
                        ? "Grundumfang"
                        : st.funktionen.map((f) => ZUSATZ[f] ?? f).join(", ")}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            <h3>Schlüssel einlesen</h3>
            <p className="muted small">
              Wirkt sofort und überdauert den Neustart: der Schlüssel wird geprüft und in der
              Datenbank abgelegt, wo er Vorrang vor <code>config.conf</code> hat. Ein leeres
              Feld nimmt die Lizenz zurück.
            </p>
            <textarea
              className="konfig-feld"
              rows={3}
              placeholder="<Daten>.<Signatur>"
              value={schluesselFeld}
              onChange={(e) => setSchluesselFeld(e.target.value)}
            />
            <div className="knopfreihe">
              <button className="btn" disabled={laeuft !== null} onClick={schluesselEinlesen}>
                {laeuft === "lizenz" ? "Wird geprüft…" : "Einlesen"}
              </button>
            </div>

            {lizenzJetzt?.ausstellbar ? (
              <>
                <h3>Schlüssel ausstellen</h3>
                <p className="muted small">
                  Möglich, weil auf dieser Installation ein privater Signierschlüssel
                  hinterlegt ist (<code>NEXORA_SIGNIERSCHLUESSEL</code>). Auf einer
                  gewöhnlichen Installation fehlt er, und dieser Abschnitt erscheint nicht —
                  sonst könnte sich jeder seine Lizenz selbst schreiben.
                </p>
                <div className="knopfreihe">
                  <input
                    placeholder="Inhaber"
                    value={ausstellen.inhaber}
                    onChange={(e) => setAusstellen({ ...ausstellen, inhaber: e.target.value })}
                  />
                  <select
                    value={ausstellen.stufe}
                    onChange={(e) => setAusstellen({ ...ausstellen, stufe: e.target.value })}
                  >
                    {(lizenzJetzt?.stufen ?? []).map((st) => (
                      <option key={st.name} value={st.name}>
                        {st.name}
                      </option>
                    ))}
                  </select>
                  <input
                    type="date"
                    value={ausstellen.ablauf}
                    onChange={(e) => setAusstellen({ ...ausstellen, ablauf: e.target.value })}
                    aria-label="Gültig bis, leer heißt ein Jahr"
                  />
                  <button
                    className="btn"
                    disabled={laeuft !== null || ausstellen.inhaber.trim() === ""}
                    onClick={schluesselAusstellen}
                  >
                    {laeuft === "ausstellen" ? "Wird signiert…" : "Ausstellen"}
                  </button>
                </div>
                <p className="muted small">
                  Ohne Datum gilt ein Jahr; länger wird nicht ausgestellt. Geprüft wird
                  offline, ein ausgegebener Schlüssel lässt sich also nicht zurückrufen — das
                  Ablaufdatum ist der einzige Hebel.
                </p>
                {ausgestellt && (
                  <textarea className="konfig-feld" rows={3} readOnly value={ausgestellt} />
                )}
              </>
            ) : (
              <p className="muted small">
                Schlüssel <strong>ausstellen</strong> kann diese Installation nicht: dafür
                braucht es den privaten Signierschlüssel, und der gehört zum Herausgeber.
              </p>
            )}
          </>
        );

      case "system":
        return (
          <>
            {(z.warnungen ?? []).length > 0 && (
              <div className="warnkasten">
                <strong>Beim Start bemängelt</strong>
                <ul>
                  {(z.warnungen ?? []).map((w) => (
                    <li key={w}>{w}</li>
                  ))}
                </ul>
              </div>
            )}

            <h3>Verbund</h3>
            <p className="muted small">
              Die Dienste, mit denen dieser hier spricht, samt Antwortzeit. Der
              Docker-Verbund selbst steht nicht dabei: dafür bräuchte der Container den
              Steuerkanal von Docker, und wer den hat, ist auf dem Wirt allmächtig. Eine
              Liste von Containern ist das nicht wert.
            </p>
            <div className="verbund">
              {(z.verbund ?? []).map((d) => (
                <div
                  key={d.name}
                  className={
                    "verbund-karte" +
                    (d.zustand === "läuft" ? " laeuft" : d.zustand === "fehlt" ? " fehlt" : " aus")
                  }
                >
                  <div className="verbund-kopf">
                    <span className="verbund-name">{d.name}</span>
                    <span className="verbund-zustand">{d.zustand}</span>
                  </div>
                  <div className="verbund-rolle">{d.rolle}</div>
                  <div className="verbund-adresse">{d.adresse || "keine Adresse"}</div>
                  <div className="verbund-werte">
                    {d.fassung && <span>Fassung {d.fassung}</span>}
                    {d.antwort && <span>{d.antwort}</span>}
                    {d.zustand === "fehlt" && !d.notwendig && <span>nicht schlimm</span>}
                  </div>
                  {d.hinweis && <div className="verbund-hinweis">{d.hinweis}</div>}
                </div>
              ))}
            </div>

            <h3>Nur beim Start änderbar</h3>
            <p className="muted small">
              Diese Werte stehen in <code>config.conf</code> oder in der Umgebung. Sie werden
              gebraucht, bevor die Datenbank offen ist, und lassen sich deshalb nicht aus dem
              Browser ändern.
            </p>
            <table className="tabelle">
              <tbody>
                <tr>
                  <td>Port</td>
                  <td>{z.nurInDerDatei.port}</td>
                </tr>
                <tr>
                  <td>Datenverzeichnis</td>
                  <td>
                    <code>{z.nurInDerDatei.datenVerzeichnis}</code>
                  </td>
                </tr>
                <tr>
                  <td>Öffentliche Adresse</td>
                  <td>
                    {z.nurInDerDatei.oeffentlicheUrl || <span className="muted">nicht gesetzt</span>}
                  </td>
                </tr>
                <tr>
                  <td>LDAP</td>
                  <td>
                    {z.nurInDerDatei.ldapAktiv ? (
                      <>
                        an — <code>{z.nurInDerDatei.ldapServer || "kein Server angegeben"}</code>
                      </>
                    ) : (
                      <span className="muted">aus</span>
                    )}
                  </td>
                </tr>
                <tr>
                  <td>SSO über OIDC</td>
                  <td>
                    {z.nurInDerDatei.oidcAktiv ? (
                      <>
                        an — <code>{z.nurInDerDatei.oidcAussteller || "kein Aussteller angegeben"}</code>
                      </>
                    ) : (
                      <span className="muted">aus</span>
                    )}
                  </td>
                </tr>
              </tbody>
            </table>
          </>
        );

      case "wartung":
        return (
          <>
            <h3>Konfigurationsdatei</h3>
            {konfig === null ? (
              <p className="muted">Wird geladen…</p>
            ) : !konfig.gefunden ? (
              <p className="muted">
                Diese Instanz läuft ohne <code>config.conf</code> — aus Umgebungsvariablen und
                Vorgaben. Es gibt hier nichts zu bearbeiten.
              </p>
            ) : (
              <>
                <p className="muted small">
                  <code>{konfig.pfad}</code>
                  {!konfig.schreibbar && " — für den Dienst nur lesbar"}
                </p>
                {/* The sentence stands here and not in the small print: whoever
                    looks for credentials and finds asterisks otherwise takes
                    them for lost and writes them anew, of all things the ones
                    that are correct. */}
                <p className="muted small">
                  Zugangsdaten sind unkenntlich gemacht. Zeilen mit{" "}
                  <code>********</code> bleiben beim Speichern unverändert; wer einen Wert ändern
                  will, schreibt den neuen an diese Stelle.
                </p>
                <textarea
                  className="konfig-feld"
                  spellCheck={false}
                  value={konfigEntwurf}
                  disabled={!konfig.schreibbar}
                  onChange={(e) => setKonfigEntwurf(e.target.value)}
                />
                <div className="knopfreihe">
                  <button className="btn" disabled={laeuft !== null} onClick={konfigPruefen}>
                    {laeuft === "konfig-pruefen" ? "Prüft…" : "Prüfen"}
                  </button>
                  <button
                    className="btn btn-primary"
                    disabled={laeuft !== null || !konfig.schreibbar || konfigEntwurf === konfig.inhalt}
                    onClick={konfigSpeichern}
                  >
                    {laeuft === "konfig-speichern" ? "Speichert…" : "Speichern"}
                  </button>
                  <button
                    className="btn"
                    disabled={konfigEntwurf === konfig.inhalt}
                    onClick={() => {
                      setKonfigEntwurf(konfig.inhalt);
                      setKonfigHinweise(konfig.hinweise);
                    }}
                  >
                    Änderungen verwerfen
                  </button>
                </div>
                {konfigHinweise.length > 0 && (
                  <div className="warnkasten">
                    <strong>Auffälligkeiten</strong>
                    <ul>
                      {konfigHinweise.map((h) => (
                        <li key={h}>{h}</li>
                      ))}
                    </ul>
                  </div>
                )}
                <details className="konfig-schluessel">
                  <summary>
                    Bekannte Schlüssel ({konfig.schluessel.length})
                  </summary>
                  <p className="muted small">
                    Alles, was diese Fassung auswertet. Was hier nicht steht, wird beim Start
                    stillschweigend übergangen.
                  </p>
                  <div className="schluesselliste">
                    {konfig.schluessel.map((k) => (
                      <code key={k}>{k}</code>
                    ))}
                  </div>
                </details>
              </>
            )}

            <h3>Dienst neu starten</h3>
            <p className="muted small">
              Nötig, damit Änderungen an der Konfigurationsdatei greifen: gelesen wird sie nur beim
              Start. Betroffen ist <strong>allein dieser Dienst</strong> — die Oberfläche und die
              Datenbank laufen in eigenen Containern weiter und werden nicht angefasst. Für die
              Dauer des Starts, ein bis zwei Sekunden, antwortet die Anwendung nicht.
            </p>
            <p className="muted small">
              Der Dienst beendet sich selbst; hochgefahren wird er von dem, was ihn betreibt —
              Docker mit <code>restart: unless-stopped</code>, systemd, Kubernetes.{" "}
              <strong>Gibt es nichts davon, bleibt er aus.</strong>
            </p>
            <div className="knopfreihe">
              <input
                placeholder="neustart"
                value={neustartWort}
                onChange={(e) => setNeustartWort(e.target.value)}
                aria-label="Zur Bestätigung das Wort neustart eingeben"
              />
              <button
                className="btn"
                disabled={neustartWort.trim() !== "neustart" || laeuft !== null}
                onClick={neustarten}
              >
                {laeuft === "neustart" ? "Beendet…" : "Neu starten"}
              </button>
            </div>
            <p className="muted small">
              Zur Bestätigung <code>neustart</code> eintippen. Ein Knopf, der beim
              Danebenklicken den Dienst abschaltet, wäre an dieser Stelle falsch.
            </p>

            <h3>Papierkorb der Instanz</h3>
            <p className="muted small">
              Löscht alle Seiten im Papierkorb endgültig — auch die anderer Konten. Der Papierkorb
              eines Kontos bleibt davon unberührt nur, solange niemand ihn benutzt hat.
            </p>
            <button className="btn" disabled={laeuft !== null} onClick={papierkorbLeeren}>
              {laeuft === "papierkorb" ? "Löscht…" : "Papierkorb endgültig leeren"}
            </button>
          </>
        );
    }
  };

  return (
    <div className="einstellungen-manager">
      <nav className="einstellungen-nav">
        <div className="einstellungen-nav-titel">Verwaltung</div>
        {BEREICHE.map((b) => (
          <button
            key={b.id}
            className={"einstellungen-nav-eintrag" + (bereich === b.id ? " aktiv" : "")}
            onClick={() => setBereich(b.id)}
          >
            <span className="nav-titel">{b.titel}</span>
            <span className="nav-unter">{b.unter}</span>
          </button>
        ))}
      </nav>

      <div className="einstellungen-inhalt">
        {meldung && (
          <div className={meldung.art === "ok" ? "hinweis-ok" : "fehler"}>{meldung.text}</div>
        )}
        {inhalt()}
      </div>
    </div>
  );
}
