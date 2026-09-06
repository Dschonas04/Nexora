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
import { Fragment, useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import {
  Anmeldungen,
  Einstellung,
  KonfigDatei,
  LDAPEinrichtung,
  LDAPTestErgebnis,
  SicherungUmfang,
  MitschriftZustand,
  Puls,
  Rechner,
  RechnerListe,
  Sitzung,
  SystemZustand,
  api,
} from "../api/client";
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
  | "zusammen"
  | "sicherheit"
  | "anmeldungen"
  | "ldap"
  | "sitzungen"
  | "datenbank"
  | "suche"
  | "anhaenge"
  | "aussehen"
  | "lizenz"
  | "system"
  | "wartung";

const BEREICHE: { id: Bereich; titel: string; unter: string }[] = [
  { id: "uebersicht", titel: "Übersicht", unter: "Zustand, Kennzahlen, Puls" },
  // Accounts and groups each used to have an entry of their own in the sidebar.
  // Both are administration and are rarely touched; they belong where one looks
  // anyway when one wants to set something.
  { id: "nutzer", titel: "Nutzer", unter: "Konten, Rollen" },
  { id: "gruppen", titel: "Gruppen", unter: "Ablage-Rechte" },
  { id: "zusammen", titel: "Zusammenarbeit", unter: "echtzeit" },
  { id: "sicherheit", titel: "Sicherheit", unter: "Registrierung, Sitzungsdauer" },
  { id: "anmeldungen", titel: "Anmeldungen", unter: "Versuche, Adressen" },
  { id: "sitzungen", titel: "Sitzungen", unter: "Geräte, einzeln beendbar" },
  { id: "ldap", titel: "Verzeichnis", unter: "LDAP / AD" },
  { id: "datenbank", titel: "Datenbank", unter: "PostgreSQL, Tabellen, Belegung" },
  { id: "suche", titel: "Suche", unter: "Wörterbuch, Index" },
  { id: "anhaenge", titel: "Anhänge", unter: "Grenze, Ablage, Belegung" },
  { id: "aussehen", titel: "Aussehen", unter: "Grundton, Akzent" },
  { id: "lizenz", titel: "Lizenz", unter: "Umfang, Laufzeit" },
  { id: "system", titel: "System", unter: "config.conf, nur beim Start" },
  { id: "wartung", titel: "Wartung", unter: "Datei, Neustart, Sicherung" },
];

const ZUSATZ: Record<string, string> = {
  versionen: "Versionsverlauf",
  anhaenge: "Anhänge",
  freigeben: "Teilen und öffentliche Links",
  pruefspur: "Protokoll",
  gruppen: "Gruppen und Ablage-Rechte",
  sso: "SSO über OIDC",
  ldap: "LDAP und Active Directory",
  anhangsuche: "Volltext in Anhängen",
  export: "Ablage-Export",
  kommentare: "Kommentare",
  konflikte: "Konflikterkennung",
  echtzeit: "Gemeinsames Bearbeiten",
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

// Die Wege, über die eine Anmeldung hereinkommt. Das Backend schreibt die
// kurzen Namen, hier stehen die ausgeschriebenen.
const WEG_TITEL: Record<string, string> = {
  passwort: "Passwort",
  ldap: "Verzeichnis",
  sso: "SSO",
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

// Aus der Kennung des Browsers das eine Wort machen, das in einer Tabelle Platz
// hat. Die volle Zeichenkette bleibt im title des Feldes stehen: für die Frage
// "war ich das selbst" reicht "Firefox auf Linux", für alles darüber hinaus
// braucht man ohnehin das Original.
function geraet(ua: string): string {
  if (!ua) return "";
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /OPR\//.test(ua)
      ? "Opera"
      : /Firefox\//.test(ua)
        ? "Firefox"
        : /Chrome\//.test(ua)
          ? "Chrome"
          : /Safari\//.test(ua)
            ? "Safari"
            : /curl|wget|python|go-http/i.test(ua)
              ? "Skript"
              : "unbekannt";
  const system = /Android/.test(ua)
    ? "Android"
    : /iPhone|iPad/.test(ua)
      ? "iOS"
      : /Windows/.test(ua)
        ? "Windows"
        : /Macintosh/.test(ua)
          ? "macOS"
          : /Linux/.test(ua)
            ? "Linux"
            : "";
  return system ? `${browser} auf ${system}` : browser;
}

// Eine Laufzeit, wie man sie ausspricht.
function laufzeit(sek: number): string {
  if (sek < 60) return `${sek} s`;
  if (sek < 3600) return `${Math.floor(sek / 60)} min`;
  if (sek < 86400) return `${Math.floor(sek / 3600)} h ${Math.floor((sek % 3600) / 60)} min`;
  return `${Math.floor(sek / 86400)} d ${Math.floor((sek % 86400) / 3600)} h`;
}

// Die letzte Minute als Fläche, nicht als Kamm.
//
// Vorher standen hier neunundfünfzig einzelne Rechtecke nebeneinander. Das
// zeigte zwar dieselben Zahlen, las sich aber wie ein Strichcode: bei Werten,
// die sich im Sekundentakt ändern, springt jedes Rechteck für sich, und das
// Auge sieht Flimmern statt Verlauf. Eine Fläche hat eine Silhouette, und die
// bleibt auch dann lesbar, wenn sich die Zahlen darunter ändern.
//
// Von Hand als SVG und nicht mit einer Bibliothek: es ist ein Polygon aus
// neunundfünfzig Punkten. Eine Diagrammbibliothek dafür zu laden hieße, das
// Bündel um ein Vielfaches dessen zu vergrößern, was gezeichnet wird.
const KURVE_B = 300; // Einheiten im viewBox, nicht Bildpunkte
const KURVE_H = 64;

function verlauf(p: Puls) {
  const minute = p.anfragen?.minute ?? [];
  // Mindestens zwei Punkte: bei einem teilte die x-Berechnung durch null, und
  // ein NaN im Pfad zeichnet nicht etwa falsch, sondern gar nichts. Das Backend
  // liefert immer neunundfünfzig, aber eine Kurve, die bei einem Punkt still
  // verschwindet, wäre der unangenehmste Fehler von allen.
  if (minute.length < 2) return null;

  const hoechste = Math.max(1, ...minute.map((s) => s.anfragen));
  const still = minute.every((s) => s.anfragen === 0);
  const x = (i: number) => (i / (minute.length - 1)) * KURVE_B;
  // Zwei Einheiten Luft oben, damit die Spitze nicht am Rand klebt.
  const y = (v: number) => KURVE_H - 2 - (v / hoechste) * (KURVE_H - 6);

  const punkte = minute.map((sek, i) => `${x(i).toFixed(1)},${y(sek.anfragen).toFixed(1)}`);
  const linie = "M" + punkte.join(" L");
  const flaeche = `${linie} L${KURVE_B},${KURVE_H} L0,${KURVE_H} Z`;

  return (
    <div className="puls">
      <svg
        className="puls-kurve"
        viewBox={`0 0 ${KURVE_B} ${KURVE_H}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={
          still
            ? "Keine Anfragen in der letzten Minute"
            : `Anfragen je Sekunde in der letzten Minute, Spitze ${hoechste}`
        }
      >
        {/* Eine einzige Hilfslinie, auf halber Höhe. Ein volles Gitter wäre bei
            vierundsechzig Einheiten Höhe mehr Linie als Inhalt. */}
        <line x1="0" y1={y(hoechste / 2)} x2={KURVE_B} y2={y(hoechste / 2)}
              className="puls-hilfslinie" />
        <path d={flaeche} className="puls-flaeche" />
        <path d={linie} className="puls-linie" vectorEffect="non-scaling-stroke" />
        {/* Sekunden mit Fehlern als Strich auf der Grundlinie. Sie in die Fläche
            zu färben ginge nicht, die Fläche ist eine Reihe; auf der Grundlinie
            stehen sie dort, wo sie hingehören, ohne die Silhouette zu stören. */}
        {minute.map((sek, i) =>
          sek.fehler > 0 || sek.abgelehnt > 0 ? (
            <rect
              key={sek.vorSekunden}
              x={x(i) - 1}
              y={KURVE_H - 3}
              width={2}
              height={3}
              className={sek.fehler > 0 ? "puls-marke fehler" : "puls-marke abgelehnt"}
            >
              <title>
                {`vor ${sek.vorSekunden} s: ${sek.abgelehnt} abgewiesen, ${sek.fehler} gescheitert`}
              </title>
            </rect>
          ) : null,
        )}
      </svg>
      <div className="puls-fuss muted small">
        <span>vor einer Minute</span>
        <span>{still ? "nichts los" : `Spitze ${hoechste}/s`}</span>
        <span>jetzt</span>
      </div>
    </div>
  );
}

// Eine Kennzahl, groß gesetzt. Der Wert in proportionalen Ziffern, nicht in
// gleich breiten: bei dieser Größe sieht "121" mit Tabellenziffern lückenhaft
// aus. Gleich breite Ziffern gehören in Spalten, die untereinander stehen.
function kennzahl(titel: string, wert: React.ReactNode, unten?: React.ReactNode, art?: string) {
  return (
    <div className={"kennzahl" + (art ? " " + art : "")} key={titel}>
      <div className="kennzahl-titel">{titel}</div>
      <div className="kennzahl-wert">{wert}</div>
      <div className="kennzahl-fuss muted small">{unten ?? "\u00a0"}</div>
    </div>
  );
}

// Ein Füllstand. Die Farbe trägt den Ernst, die Bahn dahinter ist eine hellere
// Stufe derselben Farbe, damit der Zustand über den ganzen Balken zu lesen ist
// und nicht nur an seinem Ende.
function fuellstand(anteil: number) {
  const stufe = anteil >= 0.95 ? "eng" : anteil >= 0.7 ? "knapp" : "gut";
  return (
    <div className={"fuellstand " + stufe}>
      <div className="fuellstand-fuellung" style={{ width: `${Math.min(100, anteil * 100)}%` }} />
    </div>
  );
}

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

  // Die wirksame Grenze für eine Übertragung. Sie wird gemessen und nicht
  // gelesen: was der nginx davor erlaubt, weiß Nexora nicht, siehe
  // grenzprobe.go. Gemessen wird deshalb von hier aus, vom Browser, denn das
  // ist die Strecke, auf der es später schiefgeht.
  const [grenze, setGrenze] = useState<{
    laeuft: string;
    wirksam: number | null;
    eingestellt: number;
  } | null>(null);

  const grenzeMessen = async () => {
    const eingestellt = Number(entwurf["max_anhang_mb"]) || 25;
    setGrenze({ laeuft: `${eingestellt} MB`, wirksam: null, eingestellt });

    // Erst der eingestellte Wert. Kommt er durch, ist die Frage beantwortet und
    // es braucht keine weitere Übertragung.
    if (await api.grenzprobe(eingestellt)) {
      setGrenze({ laeuft: "", wirksam: eingestellt, eingestellt });
      return;
    }

    // Sonst einschachteln. Halbieren statt hochzählen: sechs Übertragungen
    // reichen für ein halbes Megabyte genau, aufsteigend wären es fünfzig.
    let unten = 0;
    let oben = eingestellt;
    while (oben - unten > 0.5) {
      const mitte = Math.round(((unten + oben) / 2) * 10) / 10;
      setGrenze({ laeuft: `${mitte} MB`, wirksam: null, eingestellt });
      if (await api.grenzprobe(mitte)) unten = mitte;
      else oben = mitte;
    }
    setGrenze({ laeuft: "", wirksam: unten, eingestellt });
  };

  // Der Live-Stand. Wird nur abgefragt, solange der Bereich offen ist: eine
  // Abfrage je Sekunde ist nichts, eine Abfrage je Sekunde für immer, weil
  // jemand den Reiter offen gelassen hat, ist Grundrauschen in jeder Messung.
  // Wer gerade gemeinsam schreibt. Alle drei Sekunden statt jede Sekunde: eine
  // Sitzung dauert Minuten, und die Liste soll nicht flackern.
  const [zusammen, setZusammen] = useState<MitschriftZustand | null>(null);
  useEffect(() => {
    if (bereich !== "zusammen") {
      setZusammen(null);
      return;
    }
    let lebt = true;
    const holen = () =>
      api
        .mitschriftZustand()
        .then((z) => lebt && setZusammen(z))
        .catch(() => lebt && setZusammen(null));
    holen();
    const takt = window.setInterval(holen, 3000);
    return () => {
      lebt = false;
      window.clearInterval(takt);
    };
  }, [bereich]);

  // Die eigenen Rechner. Alle zehn Sekunden: der Dienst misst ohnehin nur alle
  // fünfzehn neu und antwortet dazwischen aus dem Gedächtnis, häufiger zu
  // fragen brächte nichts als Verkehr.
  const [rechner, setRechner] = useState<RechnerListe | null>(null);
  const rechnerLaden = useCallback(() => {
    api
      .rechner()
      .then(setRechner)
      .catch(() => setRechner(null));
  }, []);
  useEffect(() => {
    if (bereich !== "system") {
      setRechner(null);
      return;
    }
    let lebt = true;
    const holen = () =>
      api
        .rechner()
        .then((l) => lebt && setRechner(l))
        .catch(() => lebt && setRechner(null));
    holen();
    const takt = window.setInterval(holen, 10000);
    return () => {
      lebt = false;
      window.clearInterval(takt);
    };
  }, [bereich]);

  const [neuerRechner, setNeuerRechner] = useState({ name: "", ziel: "", notiz: "" });
  const [rechnerFehler, setRechnerFehler] = useState("");

  const rechnerAnlegen = async () => {
    setRechnerFehler("");
    try {
      await api.rechnerAnlegen({
        name: neuerRechner.name.trim(),
        ziel: neuerRechner.ziel.trim(),
        notiz: neuerRechner.notiz.trim(),
      });
      setNeuerRechner({ name: "", ziel: "", notiz: "" });
      rechnerLaden();
    } catch (e) {
      setRechnerFehler((e as Error).message);
    }
  };

  const rechnerEntfernen = async (r: Rechner) => {
    if (
      !(await frage({
        titel: "Rechner entfernen",
        text: `${r.name} verschwindet aus der Übersicht. Der Rechner selbst merkt davon nichts.`,
        bestaetigen: "Entfernen",
      }))
    )
      return;
    setRechnerFehler("");
    try {
      await api.rechnerLoeschen(r.id);
      rechnerLaden();
    } catch (e) {
      setRechnerFehler((e as Error).message);
    }
  };

  const [puls, setPuls] = useState<Puls | null>(null);
  useEffect(() => {
    if (bereich !== "system") {
      setPuls(null);
      return;
    }
    let lebt = true;
    const holen = () => {
      api
        .puls()
        .then((p) => lebt && setPuls(p))
        .catch(() => lebt && setPuls(null));
    };
    holen();
    const takt = window.setInterval(holen, 2000);
    return () => {
      lebt = false;
      window.clearInterval(takt);
    };
  }, [bereich]);

  // Der Umfang einer Sicherung, beim Öffnen der Wartung geholt.
  const [sicherung, setSicherung] = useState<SicherungUmfang | null>(null);
  useEffect(() => {
    if (bereich !== "wartung") return;
    api.sicherungUmfang().then(setSicherung).catch(() => setSicherung(null));
  }, [bereich]);

  const [kopiert, setKopiert] = useState("");

  const kopieren = async (text: string, was: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setKopiert(was);
      window.setTimeout(() => setKopiert(""), 2000);
    } catch {
      // Ohne Erlaubnis für die Zwischenablage bleibt der Text zum Markieren da.
      setMeldung({ text: "Kopieren nicht erlaubt. Der Text lässt sich markieren.", art: "fehler" });
    }
  };

  // Das Verzeichnis. Die Einrichtung steht in config.conf und ist von hier aus
  // nur zu lesen; was sich von hier aus tun lässt, ist sie auszuprobieren.
  const [ldap, setLdap] = useState<LDAPEinrichtung | null>(null);
  const [ldapProbe, setLdapProbe] = useState({ benutzer: "", passwort: "" });
  const [ldapErgebnis, setLdapErgebnis] = useState<LDAPTestErgebnis | null>(null);
  useEffect(() => {
    if (bereich === "ldap" && !ldap) {
      api.ldapEinrichtung().then(setLdap).catch(() => setLdap(null));
    }
  }, [bereich, ldap]);

  // Einspielen. Die gewählte Datei steht im Zustand, damit der Name vor dem
  // Bestätigen sichtbar ist: wer zwei Archive nebeneinander hat, unterscheidet
  // sie nur am Zeitstempel im Namen.
  const [einspielDatei, setEinspielDatei] = useState<File | null>(null);
  const [einspielErgebnis, setEinspielErgebnis] = useState<string>("");

  const einspielen = async () => {
    if (!einspielDatei) return;
    if (
      !(await frage({
        titel: "Sicherung einspielen",
        text:
          `Der gesamte Bestand wird durch „${einspielDatei.name}“ ersetzt. Alles, was seit ` +
          `dieser Sicherung entstanden ist, geht verloren. Vorher wird der jetzige Stand ` +
          `automatisch als Rückweg im Datenverzeichnis abgelegt. Der Dienst startet danach neu, ` +
          `und du musst dich neu anmelden.`,
        bestaetigen: "Bestand ersetzen",
        gefaehrlich: true,
      }))
    )
      return;
    setLaeuft("einspielen");
    setEinspielErgebnis("");
    try {
      const e = await api.wiederherstellen(einspielDatei);
      setEinspielErgebnis(
        `Eingespielt. ${e.anhaenge} Anhänge geschrieben` +
          (e.misslungen > 0 ? `, ${e.misslungen} misslungen` : "") +
          `. Rückweg: ${e.rueckweg}. ${e.hinweis}`,
      );
      setMeldung({ text: "Eingespielt. Der Dienst startet neu.", art: "ok" });
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const sicherungTokenNeu = async () => {
    setLaeuft("sicherung");
    try {
      setSicherung(await api.sicherungTokenNeu());
      setMeldung({ text: "Losungswort erzeugt. Das Skript unten enthält es bereits.", art: "ok" });
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const sicherungTokenWeg = async () => {
    if (
      !(await frage({
        titel: "Losungswort entfernen",
        text: "Ein Skript, das damit sichert, bekommt danach 401 und sichert ins Leere. Prüfe vorher, dass keines mehr läuft.",
        bestaetigen: "Entfernen",
        gefaehrlich: true,
      }))
    )
      return;
    setLaeuft("sicherung");
    try {
      setSicherung(await api.sicherungTokenWeg());
      setMeldung({ text: "Losungswort entfernt.", art: "ok" });
    } catch (e) {
      setMeldung({ text: (e as Error).message, art: "fehler" });
    } finally {
      setLaeuft(null);
    }
  };

  const ldapTesten = async () => {
    setLaeuft("ldap");
    setLdapErgebnis(null);
    try {
      setLdapErgebnis(await api.ldapTesten(ldapProbe.benutzer.trim(), ldapProbe.passwort));
    } catch (e) {
      setLdapErgebnis({ ok: false, fehler: (e as Error).message });
    } finally {
      setLaeuft(null);
    }
  };

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

  // Anmeldeversuche. Wie die Sitzungen erst beim Öffnen geholt, und mit einem
  // Filter daneben: die interessante Frage ist fast immer "nur die
  // fehlgeschlagenen", und die Liste wird lang genug, dass man sie nicht von
  // Hand durchsieht.
  const [anmeldungen, setAnmeldungen] = useState<Anmeldungen | null>(null);
  const [anmeldeFilter, setAnmeldeFilter] = useState<{ nur: string; ip: string; tage: number }>({
    nur: "",
    ip: "",
    tage: 30,
  });
  const anmeldungenLaden = useCallback(() => {
    api
      .anmeldungen({ ...anmeldeFilter, limit: 300 })
      .then(setAnmeldungen)
      .catch(() => setAnmeldungen(null));
  }, [anmeldeFilter]);
  useEffect(() => {
    if (bereich === "anmeldungen") anmeldungenLaden();
  }, [bereich, anmeldungenLaden]);

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
            <div className="einstellung-titel">
              {e.titel}
              <code className="einstellung-schluessel">{e.schluessel}</code>
            </div>
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
              Eine Zeile je Sitzung, einzeln beendbar. Verlängerung ab der halben Laufzeit.
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
      case "uebersicht": {
        // Eine Tabelle statt einer Reihe von Kacheln. Kacheln sehen auf dem
        // ersten Blick besser aus, aber vierzehn Stück davon sind keine
        // Übersicht mehr, sondern eine Wand: man sucht darin nach der Zahl,
        // die man wissen wollte. Untereinander mit Beschriftung links liest
        // sich das in einem Durchgang.
        const bestand: [string, React.ReactNode, string][] = [
          ...Object.entries(z.zahlen ?? {}).map(
            ([k, v]) => [ZAHL_TITEL[k] ?? k, v, k] as [string, React.ReactNode, string],
          ),
          ["Datenbank", z.datenbank?.groesse || "unbekannt", "db"],
          ["Anhänge auf Platte", bytes(z.anhaengeBytes ?? 0), "anh"],
        ];

        const zustaende: [string, React.ReactNode, string][] = [
          [
            "Lizenz",
            z.lizenz.gueltig ? (
              `${z.lizenz.inhaber}, ${(z.lizenz.freigeschaltet ?? []).length} von ${z.lizenz.alle} Zusätzen frei`
            ) : (
              <span className="muted">keine gültige Lizenz, freier Umfang</span>
            ),
            "lizenz",
          ],
          ["Selbstregistrierung", sich?.registrierungOffen ? "offen" : "geschlossen", "reg"],
          [
            "Letzte Anmeldung",
            sich?.letzteAnmeldung || <span className="muted">keine verzeichnet</span>,
            "anm",
          ],
          [
            "Fehlversuche in 24 Stunden",
            sich?.fehlversuche24h ? (
              <>
                {sich.fehlversuche24h}{" "}
                <button className="btn-schlicht" onClick={() => setBereich("anmeldungen")}>
                  ansehen
                </button>
              </>
            ) : (
              "0"
            ),
            "fehl",
          ],
          ["PostgreSQL", z.datenbank?.version || "unbekannt", "pg"],
          ["Ablage", ablage || <span className="muted">wird geladen</span>, "ablage"],
          [
            "Beim Start bemängelt",
            (z.warnungen ?? []).length === 0 ? (
              "nichts"
            ) : (
              <>
                {(z.warnungen ?? []).length} Punkt(e){" "}
                <button className="btn-schlicht" onClick={() => setBereich("system")}>
                  ansehen
                </button>
              </>
            ),
            "warn",
          ],
        ];

        const zeilen = (daten: [string, React.ReactNode, string][]) => (
          <table className="tabelle uebersicht-tabelle">
            <tbody>
              {daten.map(([titel, wert, key]) => (
                <tr key={key}>
                  <td>{titel}</td>
                  <td className="zahl">{wert}</td>
                </tr>
              ))}
            </tbody>
          </table>
        );

        return (
          <>
            <h3>Bestand</h3>
            {zeilen(bestand)}
            <h3>Zustand</h3>
            {zeilen(zustaende)}
          </>
        );
      }

      case "zusammen": {
        const raeume = zusammen?.raeume ?? [];
        const leute = raeume.reduce((summe, r) => summe + r.anzahl, 0);
        return (
          <>
            <h3>Einstellung</h3>
            {feld("echtzeit")}
            {zusammen && !zusammen.lizenziert && (
              <p className="muted small">
                Nicht in der Lizenz enthalten. Der Schalter greift, sobald ein Schlüssel ihn
                freischaltet.
              </p>
            )}

            <h3>Gerade offen</h3>
            <div className="kennzahlreihe">
              {kennzahl("Seiten", raeume.length)}
              {kennzahl("Personen", leute)}
              {kennzahl("Höchstens je Seite", zusammen?.hoechstens ?? "—")}
            </div>
            {raeume.length === 0 ? (
              <p className="muted small">
                Keine aktive Sitzung. Eine entsteht beim Öffnen einer zum Bearbeiten geteilten
                Seite und endet mit dem letzten Reiter.
              </p>
            ) : (
              <table className="tabelle">
                <thead>
                  <tr>
                    <th>Seite</th>
                    <th>Wer</th>
                    <th className="zahl">Anzahl</th>
                  </tr>
                </thead>
                <tbody>
                  {raeume.map((r) => (
                    <tr key={r.seite}>
                      <td>{r.titel}</td>
                      <td className="muted">{r.wer.join(", ")}</td>
                      <td className="zahl">{r.anzahl}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            <p className="muted small">
              Momentaufnahme. Wer wann was geschrieben hat: Versionsverlauf der Seite, mit
              Lizenz zusätzlich das Protokoll.
            </p>
          </>
        );
      }

      case "sicherheit":
        return (
          <>
            <h3>Zugang</h3>
            {feld("registrierung_offen")}
            {feld("erlaubte_domaenen")}
            {feld("sitzung_stunden")}

            <h3>Administratoren</h3>
            <p className="muted small">
              Lesen und Bearbeiten auf allen Seiten, auch fremden.
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
            <table className="tabelle">
              <tbody>
                <tr>
                  <td>Fehlversuche in 24 Stunden</td>
                  <td className="zahl">{sich?.fehlversuche24h ?? 0}</td>
                </tr>
                <tr>
                  <td>Letzte Anmeldung</td>
                  <td className="zahl">
                    {sich?.letzteAnmeldung || <span className="muted">keine verzeichnet</span>}
                  </td>
                </tr>
                <tr>
                  <td>Letzter Fehlversuch</td>
                  <td className="zahl">
                    {sich?.letzterFehlversuch || <span className="muted">keiner verzeichnet</span>}
                  </td>
                </tr>
              </tbody>
            </table>
            <p className="muted small">
              Jeder Versuch mit Adresse und Grund unter{" "}
              <button className="btn-schlicht" onClick={() => setBereich("anmeldungen")}>
                Anmeldungen
              </button>
              . Passwörter werden nicht festgehalten.
            </p>
          </>
        );

      case "anmeldungen": {
        const a = anmeldungen;
        const zs = a?.zusammenfassung;
        return (
          <>
            <h3>Letzte Woche</h3>
            <table className="tabelle">
              <tbody>
                <tr>
                  <td>Angemeldet, 24 Stunden</td>
                  <td className="zahl">{zs?.erfolge24h ?? 0}</td>
                </tr>
                <tr>
                  <td>Fehlgeschlagen, 24 Stunden</td>
                  <td className="zahl">{zs?.fehl24h ?? 0}</td>
                </tr>
                <tr>
                  <td>Angemeldet, 7 Tage</td>
                  <td className="zahl">{zs?.erfolge7t ?? 0}</td>
                </tr>
                <tr>
                  <td>Fehlgeschlagen, 7 Tage</td>
                  <td className="zahl">{zs?.fehl7t ?? 0}</td>
                </tr>
                <tr>
                  <td>Verschiedene Adressen, 24 Stunden</td>
                  <td className="zahl">{zs?.adressen24h ?? 0}</td>
                </tr>
              </tbody>
            </table>

            <h3>Herkunft</h3>
            <p className="muted small">
              Adressen der letzten 7 Tage, meiste Fehlversuche zuerst. Muster: eine Adresse
              gegen viele Konten.
            </p>
            <div className="tabelle-rollen">
              <table className="tabelle">
                <thead>
                  <tr>
                    <th>Adresse</th>
                    <th>Versuche</th>
                    <th>davon fehl</th>
                    <th>Konten</th>
                    <th>zuletzt</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {(a?.herkunft ?? []).map((h) => (
                    <tr key={h.ip} className={h.fehl >= 10 ? "auffaellig" : undefined}>
                      <td>
                        <code>{h.ip}</code>
                      </td>
                      <td className="zahl">{h.versuche}</td>
                      <td className="zahl">{h.fehl}</td>
                      <td className="zahl">{h.konten}</td>
                      <td className="einzeilig">{zeitpunkt(h.letzter)}</td>
                      <td>
                        <button
                          className="btn-schlicht"
                          onClick={() => setAnmeldeFilter({ ...anmeldeFilter, ip: h.ip })}
                        >
                          nur diese
                        </button>
                      </td>
                    </tr>
                  ))}
                  {(a?.herkunft ?? []).length === 0 && (
                    <tr>
                      <td colSpan={6} className="muted">
                        Kein Versuch in den letzten sieben Tagen.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <h3>Einzelne Versuche</h3>
            <div className="pruefspur-filter">
              <select
                value={anmeldeFilter.nur}
                onChange={(e) => setAnmeldeFilter({ ...anmeldeFilter, nur: e.target.value })}
              >
                <option value="">Alle Versuche</option>
                <option value="fehl">Nur fehlgeschlagene</option>
                <option value="erfolg">Nur gelungene</option>
              </select>
              <select
                value={anmeldeFilter.tage}
                onChange={(e) =>
                  setAnmeldeFilter({ ...anmeldeFilter, tage: Number(e.target.value) })
                }
              >
                <option value={1}>Letzte 24 Stunden</option>
                <option value={7}>Letzte 7 Tage</option>
                <option value={30}>Letzte 30 Tage</option>
                <option value={365}>Letztes Jahr</option>
                <option value={0}>Alles</option>
              </select>
              <input
                placeholder="Adresse, etwa 10.0.2.43"
                value={anmeldeFilter.ip}
                onChange={(e) => setAnmeldeFilter({ ...anmeldeFilter, ip: e.target.value })}
              />
              <button className="btn" onClick={anmeldungenLaden}>
                Neu laden
              </button>
            </div>

            <div className="tabelle-rollen">
              <table className="tabelle anmelde-tabelle">
                <thead>
                  <tr>
                    <th>Zeitpunkt</th>
                    <th>Ergebnis</th>
                    <th>Kennung</th>
                    <th>Adresse</th>
                    <th>Weg</th>
                    <th>Grund</th>
                    <th>Gerät</th>
                  </tr>
                </thead>
                <tbody>
                  {(a?.versuche ?? []).map((v, i) => (
                    <tr key={i} className={v.erfolg ? undefined : "fehl"}>
                      <td className="einzeilig">{zeitpunkt(v.zeitpunkt)}</td>
                      <td className="einzeilig">{v.erfolg ? "angemeldet" : "abgewiesen"}</td>
                      <td>
                        {v.kennung || <span className="muted">ohne Angabe</span>}
                        {v.name && v.name !== v.kennung && (
                          <div className="muted small">{v.name}</div>
                        )}
                      </td>
                      <td>
                        <code>{v.ip || "?"}</code>
                      </td>
                      <td className="muted">{WEG_TITEL[v.weg] ?? v.weg ?? ""}</td>
                      <td className="muted">{v.grund}</td>
                      <td className="muted small" title={v.browser}>
                        {geraet(v.browser)}
                      </td>
                    </tr>
                  ))}
                  {(a?.versuche ?? []).length === 0 && (
                    <tr>
                      <td colSpan={7} className="muted">
                        {a ? "Kein Versuch im gewählten Zeitraum." : "Wird geladen…"}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            <p className="muted small">
              Höchstens 300 Zeilen. Ältere Einträge bleiben im Protokoll.
            </p>
          </>
        );
      }

      case "ldap": {
        const l = ldap;
        if (!l) return <p className="muted">Wird geladen…</p>;

        const zeile = (titel: string, wert: React.ReactNode) => (
          <tr key={titel}>
            <td>{titel}</td>
            <td className="zahl">{wert}</td>
          </tr>
        );
        const oderNichts = (v: string) =>
          v ? <code>{v}</code> : <span className="muted">nicht gesetzt</span>;

        return (
          <>
            <h3>Einrichtung</h3>
            <p className="muted small">
              Aus <code>config.conf</code>, hier nur lesbar. Geändert wird unter Wartung,
              wirksam nach Neustart.
            </p>
            <table className="tabelle uebersicht-tabelle">
              <tbody>
                {zeile("Eingeschaltet", l.aktiv ? "ja" : <span className="muted">nein</span>)}
                {zeile(
                  "In der Lizenz enthalten",
                  l.lizenziert ? "ja" : <span className="muted">nein</span>,
                )}
                {zeile("Server", oderNichts(l.server))}
                {zeile(
                  "Verbindung verschlüsselt",
                  l.verschluesselt ? (
                    l.startTLS ? "ja, StartTLS" : "ja, ldaps"
                  ) : (
                    <span className="fehler-text">nein</span>
                  ),
                )}
                {zeile(
                  "Zertifikat wird geprüft",
                  l.tlsPruefen ? "ja" : <span className="fehler-text">nein</span>,
                )}
                {zeile("Dienstkonto", oderNichts(l.bindDN))}
                {zeile(
                  "Passwort dazu",
                  l.bindPasswortDa ? "hinterlegt" : <span className="muted">keins</span>,
                )}
                {zeile("Suchwurzel", oderNichts(l.basisDN))}
                {zeile("Filter", <code>{l.benutzerFilter}</code>)}
                {zeile("Feld für den Namen", oderNichts(l.feldName))}
                {zeile("Feld für die Adresse", oderNichts(l.feldEmail))}
                {zeile("Gruppe für Administratoren", oderNichts(l.gruppeAdmin))}
              </tbody>
            </table>

            {l.aktiv && !l.verschluesselt && (
              <div className="warnkasten">
                <strong>Die Verbindung ist unverschlüsselt</strong>
                <div className="muted small">
                  Ohne StartTLS und ohne <code>ldaps://</code> gehen die Zugangsdaten jedes
                  Anmeldenden im Klartext über das Netz. Wer mitliest, liest sie mit.
                </div>
              </div>
            )}
            {l.aktiv && !l.lizenziert && (
              <div className="warnkasten">
                <strong>Eingeschaltet, aber nicht freigeschaltet</strong>
                <div className="muted small">
                  Die Einrichtung steht, die Anmeldung über das Verzeichnis antwortet aber mit
                  402. So sieht es für den Benutzer nach einem Defekt aus.
                </div>
              </div>
            )}

            <h3>Probe</h3>
            <p className="muted small">
              Ohne Passwort nur Suche: prüft Verbindung, Dienstkonto, Filter und Feldnamen.
              Mit Passwort zusätzlich Bind. Legt kein Konto an.
            </p>
            <div className="einstellung">
              <div className="s3-felder">
                <label>
                  <span>Benutzer</span>
                  <input
                    placeholder="Anmeldename oder Adresse"
                    value={ldapProbe.benutzer}
                    onChange={(e) => setLdapProbe({ ...ldapProbe, benutzer: e.target.value })}
                  />
                </label>
                <label>
                  <span>Passwort</span>
                  <input
                    type="password"
                    placeholder="leer lassen: nur suchen"
                    value={ldapProbe.passwort}
                    onChange={(e) => setLdapProbe({ ...ldapProbe, passwort: e.target.value })}
                  />
                </label>
              </div>
              <div className="einstellung-aktionen">
                <button
                  className="btn"
                  disabled={laeuft === "ldap" || !ldapProbe.benutzer.trim() || !l.aktiv}
                  onClick={ldapTesten}
                >
                  {laeuft === "ldap" ? "Fragt…" : "Fragen"}
                </button>
              </div>

              {!l.aktiv && (
                <div className="einstellung-fuss muted small">
                  Solange <code>ldap_aktiv</code> aus ist, gibt es nichts zu fragen.
                </div>
              )}

              {ldapErgebnis && !ldapErgebnis.ok && (
                <div className="fehler">{ldapErgebnis.fehler || ldapErgebnis.hinweis}</div>
              )}
              {ldapErgebnis?.ok && (
                <div className="hinweis-ok">
                  {ldapErgebnis.befund?.passwortGeprueft
                    ? "Gefunden, und das Passwort wurde angenommen."
                    : "Gefunden. Das Passwort wurde nicht geprüft."}
                </div>
              )}
              {ldapErgebnis?.befund?.dn && (
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    {zeile("Eintrag", <code>{ldapErgebnis.befund.dn}</code>)}
                    {zeile(
                      "Name",
                      ldapErgebnis.befund.name || <span className="muted">leer</span>,
                    )}
                    {zeile(
                      "Adresse",
                      ldapErgebnis.befund.email || <span className="muted">leer</span>,
                    )}
                    {zeile(
                      "Würde Administrator",
                      ldapErgebnis.befund.admin ? "ja" : <span className="muted">nein</span>,
                    )}
                    {zeile(
                      "Gruppen",
                      ldapErgebnis.befund.gruppen.length === 0 ? (
                        <span className="muted">keine</span>
                      ) : (
                        <span className="ldap-gruppen">
                          {ldapErgebnis.befund.gruppen.join(", ")}
                        </span>
                      ),
                    )}
                  </tbody>
                </table>
              )}
            </div>
          </>
        );
      }

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
              Anhänge liegen als Dateien im Datenverzeichnis, nicht in der Datenbank. Ein
              reiner Datenbank-Dump ist unvollständig.
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
              Zeilenzahl ist eine Schätzung aus der PostgreSQL-Statistik, nicht gezählt.
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
                    Zieht den Fließtext aller Seiten neu aus dem Editor-Inhalt. Nach einem
                    Wechsel des Wörterbuchs nötig.
                  </div>
                  <div className="einstellung-warnung">Läuft in Stapeln, blockiert nichts.</div>
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
                    Holt den Suchtext für Anhänge nach, die noch keinen haben. Gelesen werden
                    Textdateien und PDF mit Textebene.
                  </div>
                  <div className="einstellung-warnung">
                    Jede Datei wird einmal aus der Ablage zurückgeholt, bei Objektspeicher übers
                    Netz. Bis zu 500 Anhänge je Durchlauf.
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
              Seiten ohne Suchtext sind meist leer. Bleibt die Zahl nach einem Neuaufbau hoch,
              stimmt der Editor-Inhalt nicht.
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
            <div className="einstellung">
              <div className="einstellung-kopf">
                <div>
                  <div className="einstellung-titel">Wirksame Grenze messen</div>
                  <div className="einstellung-erklaerung">
                    Schickt eine Übertragung der eingestellten Größe los und schachtelt die
                    Grenze auf 0,5 MB genau ein. Gemessen wird die ganze Strecke, jeder nginx
                    eingeschlossen.
                  </div>
                  <div className="einstellung-warnung">
                    Überträgt dabei einige Dutzend MB und verwirft sie sofort.
                  </div>
                </div>
                <div className="einstellung-feld">
                  <button
                    className="btn"
                    disabled={!!grenze?.laeuft}
                    onClick={grenzeMessen}
                  >
                    {grenze?.laeuft ? `Prüft ${grenze.laeuft}…` : "Messen"}
                  </button>
                </div>
              </div>
              {grenze && !grenze.laeuft && grenze.wirksam !== null && (
                <div
                  className={
                    grenze.wirksam >= grenze.eingestellt ? "hinweis-ok" : "fehler"
                  }
                >
                  {grenze.wirksam >= grenze.eingestellt
                    ? `${grenze.eingestellt} MB kommen durch. Der eingestellte Wert gilt wirklich.`
                    : `Nur ${grenze.wirksam} MB kommen durch, eingestellt sind ${grenze.eingestellt} MB. ` +
                      `Etwas auf der Strecke bricht vorher ab, in aller Regel client_max_body_size in einem nginx davor.`}
                </div>
              )}
            </div>

            <h3>Wo die Dateien liegen</h3>
            <div className="kachelreihe">{kachel(ablage || "—", "Aktuelle Ablage")}</div>
            <p className="muted small">
              Vorgabe: lokale Platte. Objektspeicher macht den Datenbank-Dump vollständig und
              erlaubt zwei Instanzen auf denselben Dateien.
            </p>

            <h3>Objektspeicher prüfen</h3>
            <p className="muted small">
              Wird nicht gespeichert, nur für den Test benutzt. Geheime Schlüssel gehören in{" "}
              <code>config.conf</code> oder in die Umgebung.
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
              Instanzweit. Auswahl wird sofort angezeigt, beim Anklicken gespeichert.
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
            <p className="muted small">Verknüpfungen, ausgewählte Einträge, Knöpfe.</p>
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
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    <tr>
                      <td>Inhaber</td>
                      <td className="zahl">{z.lizenz.inhaber}</td>
                    </tr>
                    <tr>
                      <td>Gültig bis</td>
                      <td className="zahl">{z.lizenz.laeuftAb || "unbefristet"}</td>
                    </tr>
                    <tr>
                      <td>Freigeschaltete Funktionen</td>
                      <td className="zahl">
                        {(z.lizenz.freigeschaltet ?? []).length} von {z.lizenz.alle}
                      </td>
                    </tr>
                  </tbody>
                </table>

                <h3>Funktionen</h3>
                {/* Frei oder gesperrt steht als Wort in der Spalte. Vorher war
                    es ein Schild, das Gesperrte zusaetzlich durchgestrichen und
                    halb durchsichtig: drei Mittel fuer eine Angabe, von denen
                    zwei die Zeile nur schlechter lesbar machten. */}
                <table className="tabelle">
                  <thead>
                    <tr>
                      <th>Funktion</th>
                      <th>Zustand</th>
                      <th>Name im Schlüssel</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Object.entries(ZUSATZ).map(([k, titel]) => {
                      const frei = (z.lizenz.freigeschaltet ?? []).includes(k);
                      return (
                        <tr key={k}>
                          <td>{titel}</td>
                          <td className={frei ? undefined : "muted"}>
                            {frei ? "frei" : "gesperrt"}
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
                  {z.lizenz.grund || "Kein Schlüssel hinterlegt."} Nexora läuft im freien
                  Umfang. Aufrufe für gesperrte Funktionen antworten mit 402.
                </div>
              </div>
            )}
            <h3>Stufen</h3>
            <p className="muted small">
              Jede Stufe enthält die kleineren. Geprüft wird die einzelne Funktion, nie die
              Stufe.
            </p>
            <table className="tabelle">
              <thead>
                <tr>
                  <th>Stufe</th>
                  <th>Enthält</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {(lizenzJetzt?.stufen ?? []).map((st) => (
                  <tr key={st.name}>
                    <td>{st.name}</td>
                    <td className="muted small">
                      {st.funktionen.length === 0
                        ? "Grundumfang"
                        : st.funktionen.map((f) => ZUSATZ[f] ?? f).join(", ")}
                    </td>
                    <td className="einzeilig">
                      {lizenzJetzt?.stufe === st.name && "in Betrieb"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            <h3>Schlüssel einlesen</h3>
            <p className="muted small">
              Wird geprüft und in der Datenbank abgelegt: sofort wirksam, übersteht den
              Neustart, Vorrang vor <code>config.conf</code>. Leer nimmt die Lizenz zurück.
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
                  Möglich, weil hier ein privater Signierschlüssel liegt (
                  <code>NEXORA_SIGNIERSCHLUESSEL</code>). Ohne ihn erscheint dieser Abschnitt nicht.
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
                  Ohne Datum ein Jahr, mehr wird nicht ausgestellt. Prüfung offline, also kein
                  Rückruf — nur das Ablaufdatum.
                </p>
                {ausgestellt && (
                  <textarea className="konfig-feld" rows={3} readOnly value={ausgestellt} />
                )}
              </>
            ) : (
              <p className="muted small">
                Ausstellen kann diese Installation nicht: der private Signierschlüssel liegt beim
                Herausgeber.
              </p>
            )}
          </>
        );

      case "system":
        return (
          <>
            <h3>Gerade eben</h3>
            {!puls ? (
              <div className="kennzahlreihe">
                {kennzahl("Anfragen je Sekunde", "—")}
                {kennzahl("Antwortzeit", "—")}
                {kennzahl("Gleichzeitig", "—")}
                {kennzahl("Verbindungen", "—")}
              </div>
            ) : (
              <>
                {/* Vier Zahlen groß, der Rest klein darunter. Die vier sind
                    die, nach denen jemand sieht, während es hakt; alles
                    Weitere liest man erst, wenn eine davon auffällt. */}
                <div className="kennzahlreihe">
                  {kennzahl(
                    "Anfragen je Sekunde",
                    (puls.anfragen?.proSekunde ?? 0).toFixed(1),
                    `${(puls.anfragen?.gesamt ?? 0).toLocaleString("de-DE")} seit dem Start`,
                  )}
                  {kennzahl(
                    "Antwortzeit",
                    <>
                      {(puls.anfragen?.mittelMs ?? 0).toFixed(0)}
                      <span className="kennzahl-einheit">ms</span>
                    </>,
                    `längste ${(puls.anfragen?.spitzeMs ?? 0).toFixed(0)} ms`,
                  )}
                  {kennzahl(
                    "Gleichzeitig",
                    puls.anfragen?.laufend ?? 0,
                    "gerade in Bearbeitung",
                  )}
                  {kennzahl(
                    "Verbindungen",
                    <>
                      {puls.vorrat.inBenutzung}
                      <span className="kennzahl-einheit">von {puls.vorrat.hoechstens}</span>
                    </>,
                    fuellstand(
                      puls.vorrat.hoechstens > 0
                        ? puls.vorrat.inBenutzung / puls.vorrat.hoechstens
                        : 0,
                    ),
                  )}
                </div>

                {verlauf(puls)}

                <p className="muted small">
                  Letzte Minute, Nachladen alle 2 s, solange dieser Bereich offen ist. Der eigene
                  Abfrageweg zählt nicht mit, die laufende Sekunde fehlt. Striche auf der
                  Grundlinie: abgewiesen (gelb), gescheitert (rot).
                </p>

                {(puls.anfragen?.fehler ?? 0) > 0 && (
                  <div className="warnkasten">
                    <strong>
                      {puls.anfragen?.fehler} gescheiterte Anfragen in der letzten Minute
                    </strong>
                    <div className="muted small">
                      Antwortstatus ab 500, also nicht abgewiesen, sondern kaputt. Im Protokoll
                      des Containers steht, woran.
                    </div>
                  </div>
                )}

                <h3>Einzelheiten</h3>
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    <tr>
                      <td>Abgewiesen / gescheitert, letzte Minute</td>
                      <td className="zahl">
                        {puls.anfragen?.abgelehnt ?? 0} /{" "}
                        <span
                          className={(puls.anfragen?.fehler ?? 0) > 0 ? "fehler-text" : undefined}
                        >
                          {puls.anfragen?.fehler ?? 0}
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Läuft seit</td>
                      <td className="zahl">{laufzeit(puls.anfragen?.laufzeitSek ?? 0)}</td>
                    </tr>
                    <tr>
                      <td>Wartezeit auf eine Verbindung</td>
                      <td className="zahl">
                        <span
                          className={puls.vorrat.mittelWarteMs > 1 ? "fehler-text" : undefined}
                        >
                          {puls.vorrat.mittelWarteMs < 0.01
                            ? "unter 0,01 ms"
                            : `${puls.vorrat.mittelWarteMs.toFixed(2)} ms`}
                        </span>
                        <span className="muted">
                          {" "}
                          im Mittel über {puls.vorrat.zugriffe.toLocaleString("de-DE")} Zugriffe
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Datenbank</td>
                      <td className="zahl">
                        {puls.datenbank.groesse || "unbekannt"}
                        <span className="muted">
                          {" "}
                          {puls.datenbank.trefferquote === null
                            ? ""
                            : `· ${puls.datenbank.trefferquote} % aus dem Speicher`}
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Speicher des Dienstes</td>
                      <td className="zahl">
                        {puls.prozess.speicherMB.toFixed(1)} MB
                        <span className="muted">
                          {" "}
                          · {puls.prozess.aufgaben} Aufgaben auf {puls.prozess.kerne} Kernen
                        </span>
                      </td>
                    </tr>
                  </tbody>
                </table>

                {puls.vorrat.mittelWarteMs > 1 && (
                  <div className="warnkasten">
                    <strong>Anfragen warten auf eine Verbindung</strong>
                    <div className="muted small">
                      Im Mittel {puls.vorrat.mittelWarteMs.toFixed(1)} ms, bevor eine Anfrage
                      überhaupt mit der Datenbank sprechen darf. Der Vorrat steht auf{" "}
                      {puls.vorrat.hoechstens}; ohne Angabe nimmt pgx eine Verbindung je Kern.
                      Höher setzen mit <code>pool_max_conns</code> in <code>DATABASE_URL</code>
                      , und unter <code>max_connections</code> von PostgreSQL bleiben.
                    </div>
                  </div>
                )}
                {puls.datenbank.trefferquote !== null && puls.datenbank.trefferquote < 95 && (
                  <div className="warnkasten">
                    <strong>PostgreSQL liest von der Platte</strong>
                    <div className="muted small">
                      Unter 95 % kommt ein spürbarer Teil der Antworten nicht mehr aus dem
                      Speicher. Das ist der Punkt, ab dem sich mehr <code>shared_buffers</code>{" "}
                      lohnt, und vorher nicht.
                    </div>
                  </div>
                )}
              </>
            )}

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
              Dienste, mit denen Nexora spricht, samt Antwortzeit. Die übrigen Container des
              Verbunds fehlen: dafür bräuchte es den Steuerkanal von Docker.
            </p>
            <div className="tabelle-rollen">
              <table className="tabelle verbund-tabelle">
                <thead>
                  <tr>
                    <th>Dienst</th>
                    <th>Rolle</th>
                    <th>Adresse</th>
                    <th>Zustand</th>
                    <th>Fassung</th>
                    <th>Antwort</th>
                  </tr>
                </thead>
                <tbody>
                  {(z.verbund ?? []).map((d) => (
                    <Fragment key={d.name}>
                      <tr
                        className={
                          d.zustand === "läuft" ? "laeuft" : d.zustand === "fehlt" ? "fehlt" : "aus"
                        }
                      >
                        <td>{d.name}</td>
                        <td className="muted">{d.rolle}</td>
                        <td>
                          {d.adresse ? <code>{d.adresse}</code> : <span className="muted">keine</span>}
                        </td>
                        <td className="verbund-zustand einzeilig">
                          {d.zustand}
                          {d.zustand === "fehlt" && !d.notwendig && (
                            <span className="muted"> (nicht schlimm)</span>
                          )}
                        </td>
                        <td className="muted">{d.fassung || "—"}</td>
                        <td className="muted einzeilig">{d.antwort || "—"}</td>
                      </tr>
                      {d.hinweis && (
                        <tr className="verbund-hinweis">
                          <td colSpan={6}>{d.hinweis}</td>
                        </tr>
                      )}
                    </Fragment>
                  ))}
                  {(z.verbund ?? []).length === 0 && (
                    <tr>
                      <td colSpan={6} className="muted">
                        Kein Dienst gemeldet.
                      </td>
                    </tr>
                  )}
                  </tbody>
                </table>
            </div>

            <h3>Eigene Rechner</h3>
            <p className="muted small">
              Adressen, bei denen diese Instanz selbst anklopft. Kein Agent, kein Zugang zum
              fremden Rechner.
            </p>
            <p className="muted small">
              Erkannt wird aus dem Banner: SSH nennt seine Fassung, HTTP die Kopfzeile{" "}
              <code>Server</code>, TLS das Zertifikat samt Ablauf. Wer schweigt, bleibt leer —
              geraten wird nichts.
            </p>

            <div className="tabelle-rollen">
              <table className="tabelle verbund-tabelle">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Adresse</th>
                    <th>Zustand</th>
                    <th>Antwort</th>
                    <th>Fassung</th>
                    <th>Zertifikat</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {(rechner?.rechner ?? []).map((r) => (
                    <Fragment key={r.id}>
                      <tr
                        className={
                          r.zustand === "antwortet"
                            ? "laeuft"
                            : r.zustand === "still"
                              ? "fehlt"
                              : "aus"
                        }
                      >
                        <td>{r.name}</td>
                        <td className="muted einzeilig">{r.ziel}</td>
                        <td>{r.zustand}</td>
                        <td className="muted einzeilig">{r.antwort || "—"}</td>
                        <td className="muted">{r.fassung || "—"}</td>
                        {/* Unter dreißig Tagen wird die Zelle rot: ein
                            abgelaufenes Zertifikat ist der häufigste Grund,
                            warum ein Dienst im eigenen Haus plötzlich nicht
                            mehr erreichbar ist, und der einzige, den man
                            Wochen vorher sehen könnte. */}
                        <td
                          className={
                            r.tageBisAblauf !== undefined && r.tageBisAblauf < 30
                              ? "fehler-text einzeilig"
                              : "muted einzeilig"
                          }
                        >
                          {r.zertifikat || "—"}
                        </td>
                        <td className="zeilen-aktionen">
                          <button
                            className="btn-schlicht gefaehrlich"
                            onClick={() => rechnerEntfernen(r)}
                          >
                            Entfernen
                          </button>
                        </td>
                      </tr>
                      {(r.hinweis || r.notiz) && (
                        <tr className="verbund-hinweis">
                          <td colSpan={7}>{r.notiz ? `${r.notiz}${r.hinweis ? " · " : ""}` : ""}
                            {r.hinweis}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  ))}
                  {(rechner?.rechner ?? []).length === 0 && (
                    <tr>
                      <td colSpan={7} className="muted">
                        Noch kein Rechner eingetragen.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <div className="einstellung">
              <div className="s3-felder">
                <label>
                  <span>Name</span>
                  <input
                    placeholder="optional"
                    value={neuerRechner.name}
                    onChange={(e) => setNeuerRechner({ ...neuerRechner, name: e.target.value })}
                  />
                </label>
                <label>
                  <span>Adresse</span>
                  <input
                    placeholder="10.0.0.5:22 oder https://10.0.0.5:8006"
                    value={neuerRechner.ziel}
                    onChange={(e) => setNeuerRechner({ ...neuerRechner, ziel: e.target.value })}
                    onKeyDown={(e) => e.key === "Enter" && rechnerAnlegen()}
                  />
                </label>
                <label>
                  <span>Notiz</span>
                  <input
                    placeholder="optional"
                    value={neuerRechner.notiz}
                    onChange={(e) => setNeuerRechner({ ...neuerRechner, notiz: e.target.value })}
                  />
                </label>
              </div>
              <div className="einstellung-aktionen">
                <button
                  className="btn"
                  disabled={!neuerRechner.ziel.trim()}
                  onClick={rechnerAnlegen}
                >
                  Hinzufügen
                </button>
              </div>
              {rechnerFehler && <div className="fehler">{rechnerFehler}</div>}
            </div>

            <h3>Nur beim Start änderbar</h3>
            <p className="muted small">
              Aus <code>config.conf</code> oder der Umgebung, gelesen vor der Datenbank.
              Änderbar nur unter Wartung in der Datei.
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
                        an, <code>{z.nurInDerDatei.ldapServer || "kein Server angegeben"}</code>
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
                        an, <code>{z.nurInDerDatei.oidcAussteller || "kein Aussteller angegeben"}</code>
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
            <h3>Sicherung</h3>
            <p className="muted small">
              Datenbank und Anhänge in einem Archiv, als Strom durch den Browser. Ein Dump
              allein lässt Zeilen zurück, die auf nichts mehr zeigen.
            </p>
            {sicherung && (
              <>
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    <tr>
                      <td>Datenbank</td>
                      <td className="zahl">{bytes(sicherung.datenbankBytes)}</td>
                    </tr>
                    <tr>
                      <td>Anhänge</td>
                      <td className="zahl">
                        {sicherung.anhaenge} Dateien, {bytes(sicherung.anhaengeBytes)}{" "}
                        <span className="muted">aus {sicherung.ablage}</span>
                      </td>
                    </tr>
                    <tr>
                      <td>Archiv, geschätzt</td>
                      <td className="zahl">{bytes(sicherung.geschaetztBytes)}</td>
                    </tr>
                  </tbody>
                </table>

                {sicherung.bereit ? (
                  <div className="knopfreihe">
                    <a className="btn" href={api.sicherungAdresse}>
                      Sicherung herunterladen
                    </a>
                  </div>
                ) : (
                  <div className="warnkasten">
                    <strong>Sicherung nicht möglich</strong>
                    <div className="muted small">{sicherung.fehler}</div>
                  </div>
                )}

                <div className="warnkasten">
                  <strong>Das Archiv enthält alles</strong>
                  <div className="muted small">
                    Passwort-Hashes, Sitzungen, Freigabe-Tokens, den gesamten Inhalt. Es ist
                    die empfindlichste Datei, die diese Instanz herausgibt, und es liegt danach
                    ungeschützt im Downloadordner. <code>config.conf</code> ist{" "}
                    <strong>nicht</strong> dabei: sie steht auf dem Wirt und enthält eigene
                    Geheimnisse.
                  </div>
                </div>

                <p className="muted small">
                  Im Archiv: <code>LIESMICH.md</code> mit den Befehlen zum Zurückspielen, am Ende
                  die Datei <code>FERTIG</code>. Fehlt sie, brach die Sicherung ab — ein halbes ZIP
                  bleibt ein gültiges ZIP.
                </p>
                <p className="muted small">
                  Der Suchindex fehlt absichtlich: PostgreSQL berechnet ihn beim Einspielen neu.
                </p>

                <h3>Sicherung einspielen</h3>
                <div className="warnkasten">
                  <strong>Das ersetzt den gesamten Bestand</strong>
                  <div className="muted small">
                    Alles, was seit der gewählten Sicherung entstanden ist, geht verloren.
                    Bevor etwas überschrieben wird, legt Nexora den jetzigen Stand als
                    Rückweg im Datenverzeichnis ab — wer die falsche Datei erwischt, kommt
                    damit zurück. Ein Archiv ohne die Marke <code>FERTIG</code> wird
                    abgelehnt: es wäre ein halber Bestand über einem ganzen.
                  </div>
                </div>
                <div className="knopfreihe">
                  <input
                    type="file"
                    accept=".zip,application/zip"
                    onChange={(e) => {
                      setEinspielDatei(e.target.files?.[0] ?? null);
                      setEinspielErgebnis("");
                    }}
                  />
                  <button
                    className="btn danger"
                    disabled={!einspielDatei || laeuft === "einspielen"}
                    onClick={einspielen}
                  >
                    {laeuft === "einspielen" ? "Spielt ein…" : "Einspielen"}
                  </button>
                </div>
                {einspielDatei && (
                  <p className="muted small">
                    Gewählt: <code>{einspielDatei.name}</code>, {bytes(einspielDatei.size)}
                  </p>
                )}
                {laeuft === "einspielen" && (
                  <p className="muted small">
                    Läuft: erst Sicherung des jetzigen Standes, dann Einspielen, dann die Anhänge.
                    Fenster offen lassen.
                  </p>
                )}
                {einspielErgebnis && <div className="hinweis-ok">{einspielErgebnis}</div>}

                <h3>Regelmäßig sichern</h3>
                <p className="muted small">
                  Für einen Zeitplan braucht ein Skript einen Weg herein, und einen Keks hat es
                  nicht. Das Losungswort ist dieser Weg.
                </p>
                <div className="warnkasten">
                  <strong>Dieses Wort wiegt schwerer als jedes andere hier</strong>
                  <div className="muted small">
                    Es gibt den gesamten Bestand heraus, ohne Anmeldung. Jeder Abruf damit
                    steht mit seiner Adresse im Protokoll.
                  </div>
                </div>
                <div className="knopfreihe">
                  <button
                    className="btn"
                    disabled={laeuft === "sicherung"}
                    onClick={sicherungTokenNeu}
                  >
                    {sicherung.tokenGesetzt ? "Neues Losungswort" : "Losungswort erzeugen"}
                  </button>
                  {sicherung.tokenGesetzt && (
                    <button
                      className="btn"
                      disabled={laeuft === "sicherung"}
                      onClick={sicherungTokenWeg}
                    >
                      Entfernen
                    </button>
                  )}
                </div>

                {sicherung.tokenGesetzt && (
                  <>
                    <p className="muted small">
                      Fertiges Skript mit Wort und Adresse. Prüft die Marke <code>FERTIG</code> und
                      räumt Archive nach 14 Tagen weg.
                    </p>
                    <textarea className="konfig-feld" rows={14} readOnly value={sicherung.skript} />
                    <div className="knopfreihe">
                      <button
                        className="btn"
                        onClick={() => kopieren(sicherung.skript, "skript")}
                      >
                        {kopiert === "skript" ? "Kopiert" : "Skript kopieren"}
                      </button>
                      <button className="btn" onClick={() => kopieren(sicherung.token, "sicherungswort")}>
                        {kopiert === "sicherungswort" ? "Kopiert" : "Nur das Losungswort"}
                      </button>
                    </div>
                  </>
                )}
              </>
            )}

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
                  Zugangsdaten maskiert. Zeilen mit <code>********</code> bleiben beim Speichern
                  unverändert; neuen Wert an diese Stelle schreiben.
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
                    Alles, was diese Fassung auswertet. Unbekannte Schlüssel werden beim Start
                    übergangen.
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
              Nötig, damit Änderungen an der Konfigurationsdatei greifen — gelesen wird sie nur
              beim Start. Betrifft <strong>allein diesen Dienst</strong>; Oberfläche und
              Datenbank laufen weiter. 1–2 s ohne Antwort.
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
              Zur Bestätigung <code>neustart</code> eintippen.
            </p>

            <h3>Papierkorb der Instanz</h3>
            <p className="muted small">
              Löscht alle Seiten im Papierkorb endgültig, auch die anderer Konten.
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
