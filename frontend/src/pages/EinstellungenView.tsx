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
  Puls,
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
  { id: "uebersicht", titel: "Übersicht", unter: "Zahlen und Zustand auf einen Blick" },
  // Accounts and groups each used to have an entry of their own in the sidebar.
  // Both are administration and are rarely touched; they belong where one looks
  // anyway when one wants to set something.
  { id: "nutzer", titel: "Nutzer", unter: "Konten, Rollen, Zugänge" },
  { id: "gruppen", titel: "Gruppen", unter: "Konten bündeln für Ablage-Rechte" },
  { id: "sicherheit", titel: "Sicherheit", unter: "Registrierung, Laufzeiten, Administratoren" },
  { id: "anmeldungen", titel: "Anmeldungen", unter: "Jeder Versuch, mit Adresse und Herkunft" },
  { id: "sitzungen", titel: "Sitzungen", unter: "Angemeldete Geräte, einzeln beendbar" },
  { id: "ldap", titel: "Verzeichnis", unter: "LDAP und Active Directory, mit Probe" },
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
  gruppen: "Gruppen und Ablage-Rechte",
  sso: "SSO über OIDC",
  ldap: "LDAP und Active Directory",
  anhangsuche: "Volltext in Anhängen",
  export: "Ablage-Export",
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

// Die letzte Minute als Balken, einer je Sekunde.
//
// Von Hand aus <div> gebaut und nicht mit einer Bibliothek: es sind neunundfünfzig
// Rechtecke, und dafür ein Diagrammpaket zu laden hieße, das Bündel um ein
// Vielfaches dessen zu vergrößern, was hier gezeichnet wird.
//
// Die Höhe ist auf die höchste Sekunde bezogen und nicht auf einen festen Wert.
// Eine feste Achse wäre bei zwei Anfragen je Sekunde eine leere Fläche und bei
// zweitausend ein Balken, der oben abgeschnitten ist. Was hier interessiert, ist
// ohnehin die Form: gleichmäßig, eine Spitze, oder ein Loch.
function balken(p: Puls) {
  const minute = p.anfragen?.minute ?? [];
  const hoechste = Math.max(1, ...minute.map((s) => s.anfragen));
  const still = minute.every((s) => s.anfragen === 0);
  return (
    <div className="puls">
      <div className="puls-balken">
        {minute.map((s) => {
          const anteil = (s.anfragen / hoechste) * 100;
          const art = s.fehler > 0 ? "fehler" : s.abgelehnt > 0 ? "abgelehnt" : "gut";
          return (
            <div
              key={s.vorSekunden}
              className={"puls-strich " + art}
              style={{ height: `${Math.max(s.anfragen > 0 ? 4 : 0, anteil)}%` }}
              title={
                `vor ${s.vorSekunden} s: ${s.anfragen} Anfragen` +
                (s.anfragen > 0 ? `, im Mittel ${s.mittelMs.toFixed(1)} ms` : "") +
                (s.abgelehnt > 0 ? `, ${s.abgelehnt} abgewiesen` : "") +
                (s.fehler > 0 ? `, ${s.fehler} gescheitert` : "")
              }
            />
          );
        })}
      </div>
      <div className="puls-fuss muted small">
        <span>vor einer Minute</span>
        <span>{still ? "nichts los" : `Spitze ${hoechste} je Sekunde`}</span>
        <span>jetzt</span>
      </div>
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
              Jeder einzelne Versuch steht unter{" "}
              <button className="btn-schlicht" onClick={() => setBereich("anmeldungen")}>
                Anmeldungen
              </button>
              , mit Adresse und Grund. Passwörter werden nirgends festgehalten, auch nicht bei
              einem Fehlversuch.
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
              Die Adressen der letzten sieben Tage, die mit den meisten Fehlversuchen zuerst.
              Viele Fehlversuche von einer Adresse auf viele verschiedene Konten sind das
              Muster, nach dem man hier sucht.
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
              Höchstens 300 Zeilen auf einmal. Ältere Einträge stehen weiter im Protokoll und
              werden nie gelöscht. Was ein Konto danach getan hat, steht dort ebenfalls.
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
              Diese Werte stehen in <code>config.conf</code> und sind von hier aus nur zu
              lesen. Ein Passwort für das Dienstkonto gehört in die Datei und nicht in eine
              Datenbankzeile, die jeder Dump mitnimmt. Geändert wird es unter Wartung, wirksam
              nach einem Neustart.
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
              Fragt das Verzeichnis nach einem Konto. Ohne Passwort wird nur gesucht, und
              genau das ist der übliche Fall: damit lassen sich Verbindung, Dienstkonto,
              Filter und Feldnamen prüfen, ohne dass jemand sein Passwort in ein fremdes
              Formular tippt. Mit Passwort wird zusätzlich gebunden, dann ist die ganze Kette
              geprüft. Ein Konto in Nexora entsteht dabei nicht.
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
                    Zieht den Fließtext aller Seiten neu aus dem Editor-Inhalt. Nach einem
                    Wechsel des Wörterbuchs ist das nötig. Sonst hilft es, wenn die Suche
                    etwas nicht findet, das sichtbar auf der Seite steht.
                  </div>
                  <div className="einstellung-warnung">
                    Läuft in Stapeln und blockiert nichts. Bei vielen Seiten dauert es einen
                    Moment.
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
                    Liest Anhänge, die noch keinen Suchtext haben, und holt ihn nach. Das
                    betrifft alles, was vor dieser Funktion hochgeladen wurde. Gelesen werden
                    Textdateien und PDF, sofern sie eine Textebene haben.
                  </div>
                  <div className="einstellung-warnung">
                    Jede Datei muss dafür einmal aus der Ablage zurückgeholt werden, bei einem
                    Objektspeicher also übers Netz. Darum steht hier ein Knopf und läuft das
                    nicht beim Start von allein. Pro Durchlauf werden bis zu 500 Anhänge
                    bearbeitet.
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
              Seiten ohne Suchtext sind meistens einfach leere Seiten. Bleibt die Zahl nach
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
            <div className="einstellung">
              <div className="einstellung-kopf">
                <div>
                  <div className="einstellung-titel">Wirksame Grenze messen</div>
                  <div className="einstellung-erklaerung">
                    Schickt aus diesem Browser eine Übertragung von der eingestellten Größe
                    los und sieht nach, ob sie ankommt. Kommt sie nicht durch, wird die
                    Grenze eingeschachtelt, bis sie auf ein halbes Megabyte genau feststeht.
                    Gemessen wird damit die ganze Strecke, also auch jeder nginx dazwischen.
                  </div>
                  <div className="einstellung-warnung">
                    Dabei werden ein paar Dutzend Megabyte übertragen und sofort verworfen.
                    Über eine langsame Leitung dauert das entsprechend.
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
              Jede Stufe enthält die kleineren mit. Geprüft wird immer die einzelne Funktion
              und nie die Stufe. Deshalb kann ein Schlüssel eine Stufe und zusätzlich
              einzelne Funktionen tragen.
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
              Der Schlüssel wird geprüft und in der Datenbank abgelegt. Er wirkt sofort,
              übersteht einen Neustart und hat Vorrang vor <code>config.conf</code>. Ein
              leeres Feld nimmt die Lizenz wieder zurück.
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
                  Das geht hier, weil auf dieser Installation ein privater Signierschlüssel
                  hinterlegt ist (<code>NEXORA_SIGNIERSCHLUESSEL</code>). Auf einer normalen
                  Installation fehlt er und dieser Abschnitt erscheint gar nicht. Sonst könnte
                  sich jeder seine Lizenz selbst ausstellen.
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
                  Ohne Datum gilt ein Jahr, länger wird nicht ausgestellt. Geprüft wird
                  offline. Ein ausgegebener Schlüssel lässt sich deshalb nicht zurückrufen,
                  das Ablaufdatum ist der einzige Hebel.
                </p>
                {ausgestellt && (
                  <textarea className="konfig-feld" rows={3} readOnly value={ausgestellt} />
                )}
              </>
            ) : (
              <p className="muted small">
                Schlüssel <strong>ausstellen</strong> kann diese Installation nicht. Dafür
                braucht es den privaten Signierschlüssel, und der liegt beim Herausgeber.
              </p>
            )}
          </>
        );

      case "system":
        return (
          <>
            <h3>Gerade eben</h3>
            <p className="muted small">
              Die letzte Minute, im Sekundentakt nachgeladen, solange dieser Bereich offen
              ist. Der eigene Abfrageweg zählt sich nicht mit. Die laufende Sekunde fehlt in
              der Kurve: sie ist erst halb vergangen und sähe wie ein Einbruch aus.
            </p>
            {!puls ? (
              <p className="muted">Wird gemessen…</p>
            ) : (
              <>
                {balken(puls)}
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    <tr>
                      <td>Anfragen je Sekunde</td>
                      <td className="zahl">{(puls.anfragen?.proSekunde ?? 0).toFixed(1)}</td>
                    </tr>
                    <tr>
                      <td>Gerade in Bearbeitung</td>
                      <td className="zahl">{puls.anfragen?.laufend ?? 0}</td>
                    </tr>
                    <tr>
                      <td>Mittlere Dauer, letzte Minute</td>
                      <td className="zahl">{(puls.anfragen?.mittelMs ?? 0).toFixed(1)} ms</td>
                    </tr>
                    <tr>
                      <td>Längste Antwort, letzte Minute</td>
                      <td className="zahl">{(puls.anfragen?.spitzeMs ?? 0).toFixed(0)} ms</td>
                    </tr>
                    <tr>
                      <td>Abgewiesen / gescheitert, letzte Minute</td>
                      <td className="zahl">
                        {puls.anfragen?.abgelehnt ?? 0} /{" "}
                        <span className={(puls.anfragen?.fehler ?? 0) > 0 ? "fehler-text" : undefined}>
                          {puls.anfragen?.fehler ?? 0}
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Seit dem Start beantwortet</td>
                      <td className="zahl">
                        {(puls.anfragen?.gesamt ?? 0).toLocaleString("de-DE")}{" "}
                        <span className="muted">
                          in {laufzeit(puls.anfragen?.laufzeitSek ?? 0)}
                        </span>
                      </td>
                    </tr>
                  </tbody>
                </table>

                <h3>Verbindungen zur Datenbank</h3>
                <p className="muted small">
                  Die Stelle, an der eine Instanz zuerst eng wird, und zwar unauffällig: sind
                  alle Verbindungen belegt, wartet jede weitere Anfrage. Von außen sieht das
                  aus wie eine langsame Datenbank, obwohl die Datenbank Langeweile hat.
                  Anzusehen ist das der <em>mittleren Wartezeit</em>: bleibt sie unter einer
                  Millisekunde, ist der Vorrat groß genug. Steigt sie, lässt er sich mit{" "}
                  <code>pool_max_conns</code> in <code>DATABASE_URL</code> heben.
                </p>
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    <tr>
                      <td>In Benutzung</td>
                      <td className="zahl">
                        <span
                          className={
                            puls.vorrat.inBenutzung >= puls.vorrat.hoechstens
                              ? "fehler-text"
                              : undefined
                          }
                        >
                          {puls.vorrat.inBenutzung} von {puls.vorrat.hoechstens}
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Offen, davon frei</td>
                      <td className="zahl">
                        {puls.vorrat.offen}, davon {puls.vorrat.frei}
                      </td>
                    </tr>
                    <tr>
                      <td>Mittlere Wartezeit auf eine Verbindung</td>
                      <td className="zahl">
                        <span
                          className={puls.vorrat.mittelWarteMs > 1 ? "fehler-text" : undefined}
                        >
                          {puls.vorrat.mittelWarteMs < 0.01
                            ? "unter 0,01 ms"
                            : `${puls.vorrat.mittelWarteMs.toFixed(2)} ms`}
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Zugriffe ohne freie Verbindung</td>
                      <td className="zahl">
                        {puls.vorrat.ohneFreie.toLocaleString("de-DE")}{" "}
                        <span className="muted">
                          von {puls.vorrat.zugriffe.toLocaleString("de-DE")}
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Verbindungen an der Datenbank insgesamt</td>
                      <td className="zahl">{puls.datenbank.verbindungen}</td>
                    </tr>
                    <tr>
                      <td>Größe der Datenbank</td>
                      <td className="zahl">{puls.datenbank.groesse || "unbekannt"}</td>
                    </tr>
                    <tr>
                      <td>Aus dem Speicher beantwortet</td>
                      <td className="zahl">
                        {puls.datenbank.trefferquote === null ? (
                          <span className="muted">noch nichts gelesen</span>
                        ) : (
                          `${puls.datenbank.trefferquote} %`
                        )}
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

                <h3>Prozess</h3>
                <table className="tabelle uebersicht-tabelle">
                  <tbody>
                    <tr>
                      <td>Belegter Speicher</td>
                      <td className="zahl">
                        {puls.prozess.speicherMB.toFixed(1)} MB{" "}
                        <span className="muted">
                          von {puls.prozess.vomSystemMB.toFixed(0)} MB angefordert
                        </span>
                      </td>
                    </tr>
                    <tr>
                      <td>Nebenläufige Aufgaben</td>
                      <td className="zahl">{puls.prozess.aufgaben}</td>
                    </tr>
                    <tr>
                      <td>Kerne</td>
                      <td className="zahl">{puls.prozess.kerne}</td>
                    </tr>
                  </tbody>
                </table>
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
              Die Dienste, mit denen Nexora spricht, samt Antwortzeit. Die übrigen Container
              des Docker-Verbunds stehen nicht dabei. Dafür bräuchte dieser Container den
              Steuerkanal von Docker, und wer den hat, kann auf dem Wirt alles. So viel ist
              eine Liste von Containern nicht wert.
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

            <h3>Nur beim Start änderbar</h3>
            <p className="muted small">
              Diese Werte stehen in <code>config.conf</code> oder in der Umgebung. Nexora
              braucht sie schon, bevor die Datenbank offen ist. Aus dem Browser lassen sie
              sich deshalb nicht ändern, wohl aber unter Wartung in der Datei selbst.
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
