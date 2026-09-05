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
import { useNavigate, useParams } from "react-router";

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
  ZweitfaktorStand,
  api,
} from "../api/client";
import { useAuth } from "../auth";
import Fenster from "../components/Fenster";
import KurzeZeilen from "../components/Kurzliste";
import Listenkopf from "../components/Listenkopf";
import Zweitfaktor from "../components/Zweitfaktor";
import { useLizenz } from "../lizenz";
import AdminView from "./AdminView";
import GruppenView from "./GruppenView";
import PruefspurView from "./PruefspurView";
import { useDesign } from "../design";
import { useRueckfrage } from "../components/Rueckfrage";

// An area is an entry in the sidebar, a part is a topic group inside it.
// Fifteen entries were no longer a structure but a list one had to search,
// because some of them consisted of a single setting. The parts have stayed,
// several of them now sit one below the other on one page.
type Bereich =
  | "uebersicht"
  | "konten"
  | "zugang"
  | "inhalte"
  | "datenbank"
  | "lizenz"
  | "protokoll"
  | "system";

type Teil =
  | "uebersicht"
  | "nutzer"
  | "gruppen"
  | "zusammen"
  | "zweitfaktor"
  | "sicherheit"
  | "anmeldungen"
  | "ldap"
  | "sitzungen"
  | "datenbank"
  | "suche"
  | "anhaenge"
  | "lizenz"
  | "protokoll"
  | "system"
  | "wartung";

const BEREICHE: { id: Bereich; titel: string; unter: string; teile: Teil[] }[] = [
  { id: "uebersicht", titel: "Overview", unter: "Figures and state", teile: ["uebersicht"] },
  {
    id: "konten",
    titel: "Accounts",
    unter: "Users, roles, groups",
    teile: ["nutzer", "gruppen"],
  },
  // Whatever stands here settles a single question: who gets in and with what.
  // Registration, directory, attempts and running sessions are four answers to
  // it and used to sit in four places.
  {
    id: "zugang",
    titel: "Access",
    unter: "Second factor, registration, directory, sessions",
    teile: ["zweitfaktor", "sicherheit", "ldap", "anmeldungen", "sitzungen"],
  },
  {
    id: "inhalte",
    titel: "Content",
    unter: "Collaboration, search, attachments",
    teile: ["zusammen", "suche", "anhaenge"],
  },
  {
    id: "datenbank",
    titel: "Database",
    unter: "PostgreSQL, tables, usage",
    teile: ["datenbank"],
  },
  { id: "lizenz", titel: "Licence", unter: "Scope, term", teile: ["lizenz"] },
  // The audit trail used to be a page of its own, with its own row in the
  // sidebar -- the one administrative matter that did not sit in the
  // administration. An area of its own and no appendix to System: whoever opens
  // it is not looking for a setting but for an event.
  {
    id: "protokoll",
    titel: "Audit log",
    unter: "Actions and changes",
    teile: ["protokoll"],
  },
  {
    id: "system",
    titel: "System",
    unter: "config.conf, restart, backup",
    teile: ["system", "wartung"],
  },
];

// The heading of a topic group, as soon as several of them sit on one page. On
// a page with only one part it would repeat the sidebar entry and is therefore
// left out.
const TEIL_TITEL: Record<Teil, string> = {
  uebersicht: "Overview",
  nutzer: "Users and roles",
  gruppen: "Groups",
  zusammen: "Collaboration",
  zweitfaktor: "Second factor",
  sicherheit: "Registration and session length",
  ldap: "Directory (LDAP / AD)",
  anmeldungen: "Sign-in attempts",
  sitzungen: "Running sessions",
  datenbank: "Database",
  suche: "Search",
  anhaenge: "Attachments",
  lizenz: "Licence",
  protokoll: "Audit log",
  system: "Configuration",
  wartung: "Maintenance",
};

// The old addresses stay valid. A bookmark on /einstellungen/ldap and the two
// redirects out of the workspace should not land on the overview but where the
// matter now sits.
const ALTE_ADRESSE: Record<string, Bereich> = {
  // The appearance is no longer an administrative matter but sits under My
  // account. A bookmark on it should still arrive somewhere.
  aussehen: "uebersicht",
  nutzer: "konten",
  gruppen: "konten",
  sicherheit: "zugang",
  ldap: "zugang",
  anmeldungen: "zugang",
  sitzungen: "zugang",
  zusammen: "inhalte",
  suche: "inhalte",
  anhaenge: "inhalte",
  wartung: "system",
};

const ZUSATZ: Record<string, string> = {
  versionen: "Version history",
  anhaenge: "Attachments",
  freigeben: "Sharing and public links",
  pruefspur: "Audit log",
  gruppen: "Groups and space permissions",
  sso: "SSO over OIDC",
  ldap: "LDAP und Active Directory",
  anhangsuche: "Full text in attachments",
  export: "Space export",
  kommentare: "Comments",
  konflikte: "Conflict detection",
  echtzeit: "Editing together",
};

const ZAHL_TITEL: Record<string, string> = {
  konten: "Accounts",
  admins: "Administrators",
  seiten: "Pages",
  papierkorb: "in the trash",
  versionen: "Versions",
  anhaenge: "Attachments",
  kommentare: "Comments",
  spureintraege: "Audit entries",
  ohneSuchtext: "without search text",
};

// The routes a sign-in comes in through. The backend writes the short names,
// the spelled-out ones stand here.
const WEG_TITEL: Record<string, string> = {
  passwort: "Password",
  ldap: "Directory",
  sso: "SSO",
};

// Turn the browser's user agent into the one word that fits in a table. The
// full string stays in the field's title: for the question "was that me" a
// "Firefox on Linux" is enough, and for anything beyond it one needs the
// original anyway.
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

// An uptime the way one says it out loud.
function laufzeit(sek: number): string {
  if (sek < 60) return `${sek} s`;
  if (sek < 3600) return `${Math.floor(sek / 60)} min`;
  if (sek < 86400) return `${Math.floor(sek / 3600)} h ${Math.floor((sek % 3600) / 60)} min`;
  return `${Math.floor(sek / 86400)} d ${Math.floor((sek % 86400) / 3600)} h`;
}

// The last minute as an area, not as a comb.
//
// Before, fifty-nine separate rectangles stood here side by side. That showed
// the same numbers but read like a barcode: with values changing every second,
// every rectangle jumps on its own, and the eye sees flicker instead of a
// course. An area has a silhouette, and that stays readable even while the
// numbers underneath change.
//
// By hand as SVG and not with a library: it is a polygon of fifty-nine points.
// Loading a charting library for that would mean growing the bundle to a
// multiple of what is being drawn.
const KURVE_B = 300; // Einheiten im viewBox, nicht Bildpunkte
const KURVE_H = 64;

function verlauf(p: Puls) {
  const minute = p.anfragen?.minute ?? [];
  // At least two points: with one the x calculation divided by zero, and a NaN
  // in the path does not draw wrongly but not at all. The backend always
  // delivers fifty-nine, but a curve that silently disappears at one point
  // would be the most unpleasant bug of the lot.
  if (minute.length < 2) return null;

  const hoechste = Math.max(1, ...minute.map((s) => s.anfragen));
  const still = minute.every((s) => s.anfragen === 0);
  const x = (i: number) => (i / (minute.length - 1)) * KURVE_B;
  // Two units of air at the top, so the peak does not stick to the edge.
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
        {/* A single guide line, at half height. A full grid would be more
            line than content at sixty-four units of height. */}
        <line x1="0" y1={y(hoechste / 2)} x2={KURVE_B} y2={y(hoechste / 2)}
              className="puls-hilfslinie" />
        <path d={flaeche} className="puls-flaeche" />
        <path d={linie} className="puls-linie" vectorEffect="non-scaling-stroke" />
        {/* Seconds with errors as a stroke on the base line. Colouring them
            into the area would not work, the area is one series; on the base
            line they stand where they belong, without disturbing the
            silhouette. */}
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

// One figure, set large. The value in proportional numerals, not in
// tabular ones: at this size "121" looks gappy with tabular figures. Equal-width
// numerals belong in columns that sit one below the other.
function kennzahl(titel: string, wert: React.ReactNode, unten?: React.ReactNode, art?: string) {
  return (
    <div className={"kennzahl" + (art ? " " + art : "")} key={titel}>
      <div className="kennzahl-titel">{titel}</div>
      <div className="kennzahl-wert">{wert}</div>
      <div className="kennzahl-fuss muted small">{unten ?? "\u00a0"}</div>
    </div>
  );
}

// A fill level. The colour carries the seriousness, the track behind it is a
// lighter step of the same colour, so the state can be read across the whole
// bar and not only at its end.
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
  return d.toLocaleString(undefined, {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Days remaining until the expiry date, or null when no usable date is there --
 * perpetual, empty or unreadable. A negative value does not occur: an expired
 * licence is no longer valid and the banner says so itself then.
 */
function restlaufzeit(bis: string): number | null {
  if (!bis) return null;
  const ziel = Date.parse(bis);
  if (Number.isNaN(ziel)) return null;
  const tage = Math.ceil((ziel - Date.now()) / 86400000);
  return tage > 0 ? tage : null;
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
  const bereich: Bereich = BEREICHE.some((b) => b.id === ausAdresse)
    ? (ausAdresse as Bereich)
    : (ALTE_ADRESSE[ausAdresse ?? ""] ?? "uebersicht");
  const setBereich = (b: Bereich) => nav("/einstellungen/" + b);
  const teile: Teil[] = BEREICHE.find((b) => b.id === bereich)?.teile ?? ["uebersicht"];
  // Loading still happens per topic group and not per page: the list of sign-in
  // attempts should not come along just because somebody changes the session
  // duration. They merely sit one below the other now.
  const zeigt = (t: Teil) => teile.includes(t);

  // A link to a topic group. That used to be a change of page; if the group now
  // lies further down on the same one, the page has to scroll there, otherwise
  // the click visibly does nothing.
  const [sprung, setSprung] = useState<Teil | null>(null);
  const zuTeil = (t: Teil) => {
    const b = BEREICHE.find((x) => x.teile.includes(t));
    if (!b) return;
    if (b.id !== bereich) setBereich(b.id);
    setSprung(t);
  };
  useEffect(() => {
    if (!sprung) return;
    const ziel = document.getElementById("teil-" + sprung);
    if (!ziel) return;
    ziel.scrollIntoView({ block: "start", behavior: "smooth" });
    setSprung(null);
  }, [sprung, bereich]);
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

  // The effective limit for an upload. It is measured and not read: Nexora does
  // not know what the nginx in front of it permits, see grenzprobe.go. Measuring
  // therefore happens from here, from the browser, because that is the stretch
  // things later go wrong on.
  const [grenze, setGrenze] = useState<{
    laeuft: string;
    wirksam: number | null;
    eingestellt: number;
  } | null>(null);

  const grenzeMessen = async () => {
    const eingestellt = Number(entwurf["max_anhang_mb"]) || 25;
    setGrenze({ laeuft: `${eingestellt} MB`, wirksam: null, eingestellt });

    // The configured value first. If it gets through, the question is answered
    // and no further upload is needed.
    if (await api.grenzprobe(eingestellt)) {
      setGrenze({ laeuft: "", wirksam: eingestellt, eingestellt });
      return;
    }

    // Otherwise bracket it in. Halving instead of counting up: six uploads are
    // enough to pin down half a megabyte exactly, counting upwards it would be
    // fifty.
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

  // The live figures. Polled only while the area is open: one request per
  // second is nothing, one request per second forever because somebody left the
  // tab open is background noise in every measurement.
  // Who is writing together right now. Every three seconds instead of every
  // second: a session lasts minutes, and the list should not flicker.
  const [zusammen, setZusammen] = useState<MitschriftZustand | null>(null);
  useEffect(() => {
    if (!zeigt("zusammen")) {
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

  // One's own machines. Every ten seconds: the service only re-measures every
  // fifteen anyway and answers from memory in between, asking more often would
  // bring nothing but traffic.
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

  // The size of a backup, fetched when maintenance is opened.
  const [sicherung, setSicherung] = useState<SicherungUmfang | null>(null);
  useEffect(() => {
    if (!zeigt("wartung")) return;
    api.sicherungUmfang().then(setSicherung).catch(() => setSicherung(null));
  }, [bereich]);

  const [kopiert, setKopiert] = useState("");

  const kopieren = async (text: string, was: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setKopiert(was);
      window.setTimeout(() => setKopiert(""), 2000);
    } catch {
      // Without clipboard permission the text stays there to be selected.
      setMeldung({ text: "Kopieren nicht erlaubt. Der Text lässt sich markieren.", art: "fehler" });
    }
  };

  // The filters above the administrative lists. One per list, because they can
  // sit side by side on one page and a shared filter would then empty two
  // tables at once.
  const [sitzungFilter, setSitzungFilter] = useState("");
  const [rechnerFilter, setRechnerFilter] = useState("");
  const [tabellenFilter, setTabellenFilter] = useState("");
  const [adminFilter, setAdminFilter] = useState("");
  const [funktionFilter, setFunktionFilter] = useState("");
  // The forms that are rarely needed open as dialogs instead of sitting on the
  // page: a machine is entered once, an object store is tested once, the
  // directory is asked when something jams.
  const [rechnerOffen, setRechnerOffen] = useState(false);
  const [ablageOffen, setAblageOffen] = useState(false);
  const [ldapOffen, setLdapOffen] = useState(false);

  // The second factor on one's own account. It is not one of the instance's
  // settings and is therefore fetched separately -- and only once the topic
  // group is really on the screen.
  const [zweitStand, setZweitStand] = useState<ZweitfaktorStand | null>(null);
  const zweitLaden = useCallback(() => {
    api.zweitfaktorStand().then(setZweitStand).catch(() => setZweitStand(null));
  }, []);
  useEffect(() => {
    if (zeigt("zweitfaktor") && !zweitStand) zweitLaden();
  }, [bereich, zweitStand, zweitLaden]);

  // The directory. Its setup sits in config.conf and can only be read from
  // here; what can be done from here is to try it out.
  const [ldap, setLdap] = useState<LDAPEinrichtung | null>(null);
  const [ldapProbe, setLdapProbe] = useState({ benutzer: "", passwort: "" });
  const [ldapErgebnis, setLdapErgebnis] = useState<LDAPTestErgebnis | null>(null);
  useEffect(() => {
    if (zeigt("ldap") && !ldap) {
      api.ldapEinrichtung().then(setLdap).catch(() => setLdap(null));
    }
  }, [bereich, ldap]);

  // Restoring. The chosen file sits in the state so its name is visible before
  // confirming: whoever has two archives side by side tells them apart only by
  // the timestamp in the name.
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

  // Licence: reading one in and, at the issuer, issuing one.
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
    if (zeigt("sitzungen")) sitzungenLaden();
  }, [bereich, sitzungenLaden]);

  // Sign-in attempts. Fetched on opening like the sessions, and with a filter
  // beside them: the interesting question is nearly always "only the failed
  // ones", and the list grows long enough that one does not go through it by
  // hand.
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
    if (zeigt("anmeldungen")) anmeldungenLaden();
  }, [bereich, anmeldungenLaden]);

  useEffect(() => {
    if (!zeigt("wartung") || konfig) return;
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
      // The page width comes from the same request as the appearance. Without
      // reloading it the workspace showed the old width until a page reload.
      if (e.schluessel === "seitenbreite") designNeuLaden();
      laden();
    } catch (err) {
      setMeldung({ text: (err as Error).message, art: "fehler" });
      setEntwurf((v) => ({ ...v, [e.schluessel]: e.wert }));
    } finally {
      setLaeuft(null);
    }
  };

  const zuruecksetzen = async (e: Einstellung) => {
    setLaeuft(e.schluessel);
    try {
      await api.einstellungZuruecksetzen(e.schluessel);
      setMeldung({ text: `„${e.titel}“ folgt wieder der config.conf.`, art: "ok" });
      if (e.schluessel === "seitenbreite") designNeuLaden();
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
              {e.umgebung && (
                <code className="einstellung-schluessel umgebung" title="Umgebungsvariable, gelesen beim Start">
                  {e.umgebung}
                </code>
              )}
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

  // A topic group in its own right. The switch has stayed as it was -- all that
  // has changed is how many of its branches land on one page at the same time.
  const teilInhalt = (teil: Teil) => {
    switch (teil) {
      case "sitzungen": {
        const begriff = sitzungFilter.trim().toLowerCase();
        const sichtbar = (sitzungen ?? []).filter(
          (si) => !begriff || (si.browser + " " + si.ip).toLowerCase().includes(begriff),
        );
        return (
          <>
            <Listenkopf
              titel="Angemeldete Geräte"
              zahl={
                sitzungen === null
                  ? "wird geladen"
                  : `${sitzungen.length} ${sitzungen.length === 1 ? "Sitzung" : "Sitzungen"}`
              }
              filter={sitzungFilter}
              setFilter={setSitzungFilter}
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
                    const r = await api.sitzungenBeenden().catch(() => null);
                    if (r) setMeldung({ text: `${r.beendet} Sitzung(en) beendet.`, art: "ok" });
                    sitzungenLaden();
                  }}
                >
                  Überall sonst abmelden
                </button>
              )}
            </Listenkopf>
            <p className="muted small">
              Eine Zeile je Sitzung, jede einzeln widerrufbar. Ab der halben Laufzeit
              verlängert der nächste Aufruf die Frist und setzt das Cookie neu.
            </p>
            <div className="tabelle-rollen">
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
                  <KurzeZeilen
                    alle={sichtbar}
                    spalten={5}
                    zeile={(si) => (
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
                      <td className="zeilen-aktionen">
                        <button
                          className="btn-schlicht gefaehrlich"
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
                    )}
                  />
                  {sichtbar.length === 0 && (
                    <tr>
                      <td colSpan={5} className="muted">
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
            </div>
          </>
        );
      }

      case "nutzer":
        return <AdminView />;
      case "gruppen":
        return <GruppenView />;
      case "protokoll":
        return <PruefspurView />;
      case "uebersicht": {
        // A table instead of a row of tiles. Tiles look better at first
        // glance, but fourteen of them are no longer an overview but a wall:
        // one searches it for the number one wanted to know. One below the
        // other with the label on the left reads in a single pass.
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
                <button className="btn-schlicht" onClick={() => zuTeil("anmeldungen")}>
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
                Die eingespielte Lizenz enthält echtzeit nicht. Der Schalter bleibt wirkungslos,
                bis ein Schlüssel die Funktion freischaltet.
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
                Keine aktive Sitzung. Eine entsteht, sobald jemand eine zum Bearbeiten
                geteilte Seite öffnet, und endet mit dem letzten Reiter.
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
              Zeigt nur den Moment, gespeichert wird nichts davon. Wer wann was
              geschrieben hat, steht im Versionsverlauf der Seite und, mit Lizenz, im
              Protokoll.
            </p>
          </>
        );
      }

      case "zweitfaktor":
        return (
          <>
            <h3>Am eigenen Konto</h3>
            {zweitStand === null ? (
              <p className="muted small">Wird geladen…</p>
            ) : (
              <Zweitfaktor stand={zweitStand} neuLaden={zweitLaden} />
            )}
            <p className="muted small">
              Dieselbe Einrichtung erreicht jedes Konto über das Zahnrad neben dem Namen
              unten in der Leiste, auch ohne Verwaltungsrecht.
            </p>

            <h3>Für die ganze Instanz</h3>
            <p className="muted small">
              Gilt für Konten mit Passwort. Bei SSO kommt der zweite Faktor vom
              Anbieter. Konten aus dem Verzeichnis werden hier nach dem Code gefragt,
              sobald sie einen eingerichtet haben.
            </p>
            {feld("zweitfaktor_pflicht")}
            {feld("zweitfaktor_aussteller")}
            <p className="muted small">
              Ist ein Telefon verloren, lässt sich der zweite Faktor eines Kontos unter
              Konten entfernen. Einrichten kann ihn nur das Konto selbst.
            </p>
          </>
        );

      case "sicherheit":
        return (
          <>
            <h3>Zugang</h3>
            {feld("registrierung_offen")}
            {feld("erlaubte_domaenen")}
            {feld("sitzung_stunden")}

            <Listenkopf
              titel="Administratoren"
              zahl={`${(sich?.admins ?? []).length}`}
              filter={adminFilter}
              setFilter={setAdminFilter}
              platzhalter="Filtern nach Name oder Adresse"
            />
            <p className="muted small">
              Rolle admin: Lesen und Bearbeiten auf jeder Seite, unabhängig von der
              Freigabe. Vergeben wird sie unter Konten.
            </p>
            <table className="tabelle">
              <tbody>
                <KurzeZeilen
                  alle={(sich?.admins ?? []).filter(
                    (a) =>
                      !adminFilter.trim() ||
                      (a.name + " " + a.email)
                        .toLowerCase()
                        .includes(adminFilter.trim().toLowerCase()),
                  )}
                  spalten={2}
                  zeile={(a) => (
                    <tr key={a.email}>
                      <td>{a.name}</td>
                      <td className="muted">{a.email}</td>
                    </tr>
                  )}
                />
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
              <button className="btn-schlicht" onClick={() => zuTeil("anmeldungen")}>
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

            <Listenkopf
              titel="Herkunft"
              zahl={`${(a?.herkunft ?? []).length} ${
                (a?.herkunft ?? []).length === 1 ? "Adresse" : "Adressen"
              }`}
            />
            <p className="muted small">
              Adressen der letzten sieben Tage, nach Fehlversuchen absteigend. Auffällig
              ist eine Adresse, die viele verschiedene Konten probiert.
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
                  <KurzeZeilen
                    alle={a?.herkunft ?? []}
                    spalten={6}
                    zeile={(h) => (
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
                    )}
                  />
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

            {/* The filter of this list is not a text field but the period and
                the result -- the question one asks a sign-in list is not "what
                was their name" but "what went wrong in the last few days".
                That is why it stands below the head and not inside it. */}
            <Listenkopf
              titel="Einzelne Versuche"
              zahl={`${(a?.versuche ?? []).length} angezeigt${
                (a?.versuche ?? []).length >= 300 ? ", auf 300 begrenzt" : ""
              }`}
            />
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
                  <KurzeZeilen
                    alle={a?.versuche ?? []}
                    spalten={7}
                    zeile={(v, i) => (
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
                    )}
                  />
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
              Höchstens 300 Zeilen je Abfrage. Ältere Versuche bleiben im Protokoll und
              werden nicht gelöscht.
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
              Aus <code>config.conf</code>, hier nur lesbar. Das Passwort des Dienstkontos gehört in die Datei, nicht in die Datenbank:
              ein Dump nimmt jede Datenbankzeile mit. Geändert wird es unter Wartung,
              wirksam nach einem Neustart.
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

            <h3>Verbindung prüfen</h3>
            <p className="muted small">
              Ohne Passwort wird nur gesucht: Verbindung, Dienstkonto, Filter und
              Feldnamen. Mit Passwort kommt der Bind dazu. Ein Konto wird dabei nicht
              angelegt.
            </p>
            <div className="knopfreihe">
              <button className="btn" disabled={!l.aktiv} onClick={() => setLdapOffen(true)}>
                Verzeichnis fragen
              </button>
            </div>
            {!l.aktiv && (
              <p className="muted small">
                Solange <code>ldap_aktiv</code> aus ist, gibt es nichts zu fragen.
              </p>
            )}
            {ldapOffen && (
            <Fenster
              titel="Verzeichnis fragen"
              unter="Sucht den Eintrag, legt kein Konto an"
              schliessen={() => setLdapOffen(false)}
              fuss={
                <>
                  <span className="fuss-luecke" />
                  <button className="btn" onClick={() => setLdapOffen(false)}>
                    Schließen
                  </button>
                  <button
                    className="btn btn-primary"
                    disabled={laeuft === "ldap" || !ldapProbe.benutzer.trim() || !l.aktiv}
                    onClick={ldapTesten}
                  >
                    {laeuft === "ldap" ? "Fragt…" : "Fragen"}
                  </button>
                </>
              }
            >
              <div className="fenster-felder">
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
            </Fenster>
            )}
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
              Anhänge liegen als Dateien im Datenverzeichnis, nicht in der Datenbank.
              Ein reiner Datenbank-Dump ist deshalb unvollständig.
            </p>

            <Listenkopf
              titel="Größte Tabellen"
              zahl={`${(z.datenbank?.tabellen ?? []).length}`}
              filter={tabellenFilter}
              setFilter={setTabellenFilter}
              platzhalter="Filtern nach Name"
            />
            <table className="tabelle">
              <thead>
                <tr>
                  <th>Tabelle</th>
                  <th>Zeilen</th>
                  <th>Belegt</th>
                </tr>
              </thead>
              <tbody>
                {(z.datenbank?.tabellen ?? [])
                  .filter((t) => t.name.includes(tabellenFilter.trim().toLowerCase()))
                  .map((t) => (
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
              Die Zeilenzahl kommt aus <code>pg_stat_user_tables.n_live_tup</code>, ist also
              geschätzt und nicht gezählt. Nach vielen Änderungen liegt sie daneben, bis
              ANALYZE lief.
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
              Seiten ohne Suchtext sind meist leer. Bleibt die Zahl nach einem Neuaufbau
              hoch, stimmt der Inhalt im Editor nicht.
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

            <h3>Ablage der Dateien</h3>
            <div className="kachelreihe">{kachel(ablage || "—", "Aktuelle Ablage")}</div>
            <p className="muted small">
              Vorgabe ist die lokale Platte. Objektspeicher macht den Datenbank-Dump
              vollständig und erlaubt zwei Instanzen auf denselben Dateien.
            </p>

            <div className="knopfreihe">
              <button className="btn" onClick={() => setAblageOffen(true)}>
                Objektspeicher prüfen
              </button>
            </div>
            {/* Seven fields for a test one does once a year -- they used to
                stand permanently on the page. In a dialog they are there when
                one calls them. */}
            {ablageOffen && (
            <Fenster
              titel="Objektspeicher prüfen"
              unter="Verbinden, schreiben, lesen, löschen"
              schliessen={() => setAblageOffen(false)}
              fuss={
                <>
                  <span className="fuss-luecke" />
                  <button className="btn" onClick={() => setAblageOffen(false)}>
                    Schließen
                  </button>
                  <button
                    className="btn btn-primary"
                    disabled={laeuft === "ablage" || !s3.endpunkt}
                    onClick={ablageTesten}
                  >
                    {laeuft === "ablage" ? "Prüft…" : "Verbindung prüfen"}
                  </button>
                </>
              }
            >
              <p className="muted small">
                Hier wird nichts gespeichert; die Zugangsdaten bleiben im Formular und gelten
                nur für diesen Test. Ein geheimer Schlüssel gehört in <code>config.conf</code>
                {" "}oder in die Umgebung, nicht in die Datenbank: ein Dump nimmt jede Zeile mit.
              </p>
              <div className="fenster-felder">
                <label>
                  <span>Endpunkt</span>
                  <input
                    placeholder="10.0.2.43:9010"
                    value={s3.endpunkt}
                    onChange={(e) => setS3({ ...s3, endpunkt: e.target.value })}
                  />
                </label>
                <label>
                  <span>Bucket</span>
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
              {s3Ergebnis && (
                <div className={s3Ergebnis.ok ? "hinweis-ok" : "fehler"}>{s3Ergebnis.text}</div>
              )}
              <p className="muted small">
                Geprüft wird verbinden, schreiben, lesen und löschen. Nur zu verbinden reicht
              nicht, die häufigsten Fehler zeigen sich erst beim Schreiben.
                Übernommen wird das Ergebnis nicht: dafür die Werte in{" "}
                <code>config.conf</code> eintragen und den Dienst neu starten.
              </p>
            </Fenster>
            )}
          </>
        );

      case "lizenz":      case "lizenz":
        return (
          <>
            <h3>Lizenz</h3>
            {/* A band instead of a two-column table: holder, tier and
                remaining term are what one looks for first here, and they stand
                side by side in one row instead of one below the other. */}
            <div className="lizenz-kopf">
              <div className="lizenz-felder">
                <div>
                  <span className="lizenz-feldname">Zustand</span>
                  <span className="lizenz-feldwert">
                    {z.lizenz.gueltig ? "Aktiv" : "Keine gültige Lizenz"}
                  </span>
                </div>
                <div>
                  <span className="lizenz-feldname">Inhaber</span>
                  <span className="lizenz-feldwert">{z.lizenz.inhaber || "—"}</span>
                </div>
                <div>
                  <span className="lizenz-feldname">Stufe</span>
                  <span className="lizenz-feldwert">{lizenzJetzt?.stufe || "Grundumfang"}</span>
                </div>
                <div>
                  <span className="lizenz-feldname">Laufzeit</span>
                  <span className="lizenz-feldwert">
                    {z.lizenz.laeuftAb || "unbefristet"}
                    {restlaufzeit(z.lizenz.laeuftAb) !== null && (
                      <span className="muted small">
                        {" "}
                        · noch {restlaufzeit(z.lizenz.laeuftAb)} Tage
                      </span>
                    )}
                  </span>
                </div>
                <div>
                  <span className="lizenz-feldname">Umfang</span>
                  <span className="lizenz-feldwert">
                    {(z.lizenz.freigeschaltet ?? []).length} von {z.lizenz.alle} Funktionen
                  </span>
                </div>
              </div>
            </div>
            {!z.lizenz.gueltig && (
              <div className="warnkasten">
                <strong>{z.lizenz.grund || "Kein Schlüssel hinterlegt."}</strong>
                <div className="muted small">
                  Nexora läuft im freien Umfang. Aufrufe für gesperrte Funktionen antworten
                  mit 402.
                </div>
              </div>
            )}

            {/* A list, not a matrix. A table stood here with one column per
                licence tier and a little box in every cell -- that answered the
                question of what a change would bring, and not the one asked of
                this page: what is running right now and what is not. */}
            <Listenkopf
              titel="Funktionsumfang"
              zahl={`${(z.lizenz.freigeschaltet ?? []).length} von ${
                Object.keys(ZUSATZ).length
              } frei`}
              filter={funktionFilter}
              setFilter={setFunktionFilter}
              platzhalter="Filtern nach Funktion"
            />
            <p className="muted small">
              Geprüft wird immer die einzelne Funktion, nie die Stufe. Ein Schlüssel
              kann eine Stufe und zusätzlich einzelne Funktionen enthalten.
            </p>
            <table className="tabelle">
              <thead>
                <tr>
                  <th>Funktion</th>
                  <th>Name im Schlüssel</th>
                  <th>Zustand</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(ZUSATZ)
                  .filter(
                    ([k, titel]) =>
                      !funktionFilter.trim() ||
                      (k + " " + titel)
                        .toLowerCase()
                        .includes(funktionFilter.trim().toLowerCase()),
                  )
                  .map(([k, titel]) => {
                  const frei = (z.lizenz.freigeschaltet ?? []).includes(k);
                  return (
                    <tr key={k}>
                      <td>{titel}</td>
                      <td className="muted small">
                        <code>{k}</code>
                      </td>
                      <td className={frei ? undefined : "muted"}>{frei ? "frei" : "gesperrt"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>

            <h3>Schlüssel einlesen</h3>
            <p className="muted small">
              Wird geprüft und in der Datenbank abgelegt. Sofort wirksam, übersteht den
              Neustart, hat Vorrang vor <code>config.conf</code>. Leer nimmt die Lizenz zurück.
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
                  Ohne Datum gilt ein Jahr, länger wird nicht ausgestellt. Geprüft wird
                  offline, ohne Rückfrage beim Herausgeber. Ein ausgegebener Schlüssel
                  lässt sich deshalb nicht zurückrufen, nur das Ablaufdatum begrenzt
                  ihn.
                </p>
                {ausgestellt && (
                  <textarea className="konfig-feld" rows={3} readOnly value={ausgestellt} />
                )}
              </>
            ) : (
              <p className="muted small">
                Diese Installation kann keine Schlüssel ausstellen, der private
                Signierschlüssel liegt beim Herausgeber.
              </p>
            )}
          </>
        );

      case "system":
        return (
          <>
            <h3>Letzte Minute</h3>
            {!puls ? (
              <div className="kennzahlreihe">
                {kennzahl("Anfragen je Sekunde", "—")}
                {kennzahl("Antwortzeit", "—")}
                {kennzahl("Gleichzeitig", "—")}
                {kennzahl("Verbindungen", "—")}
              </div>
            ) : (
              <>
                {/* Four numbers large, the rest small below them. The four are
                    the ones somebody looks at while things are jamming;
                    everything further is only read once one of them stands
                    out. */}
                <div className="kennzahlreihe">
                  {kennzahl(
                    "Anfragen je Sekunde",
                    (puls.anfragen?.proSekunde ?? 0).toFixed(1),
                    `${(puls.anfragen?.gesamt ?? 0).toLocaleString()} seit dem Start`,
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
                  Letzte Minute, wird alle 2 s nachgeladen, solange dieser Bereich offen
                  ist. Der eigene Abfrageweg zählt nicht mit, die laufende Sekunde
                  fehlt. Striche unter der Linie sind abgewiesene (gelb) und
                  gescheiterte (rot) Aufrufe.
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
                          im Mittel über {puls.vorrat.zugriffe.toLocaleString()} Zugriffe
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
              Die Dienste, mit denen Nexora spricht, samt Antwortzeit. Die übrigen
              Container des Verbunds fehlen: sichtbar wären sie nur über den Steuerkanal
              von Docker, und der öffnet den ganzen Wirt.
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

            <Listenkopf
              titel="Eigene Rechner"
              zahl={
                rechner === null
                  ? "wird geladen"
                  : `${rechner.rechner.length} eingetragen · ` +
                    `${rechner.rechner.filter((r) => r.zustand === "antwortet").length} antworten`
              }
              filter={rechnerFilter}
              setFilter={setRechnerFilter}
              platzhalter="Filtern nach Name oder Adresse"
            >
              <button className="btn btn-primary" onClick={() => setRechnerOffen(true)}>
                Rechner hinzufügen
              </button>
            </Listenkopf>
            <p className="muted small">
              Adressen, an denen diese Instanz selbst eine TCP-Verbindung versucht. Auf
              der Gegenseite läuft kein Agent, es gibt keinen Zugang zum fremden
              Rechner. Hier steht nur, was Nexora selbst gesehen hat.
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
                  {(rechner?.rechner ?? [])
                    .filter(
                      (r) =>
                        !rechnerFilter.trim() ||
                        (r.name + " " + r.ziel + " " + (r.notiz ?? ""))
                          .toLowerCase()
                          .includes(rechnerFilter.trim().toLowerCase()),
                    )
                    .map((r) => (
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
                        {/* Below thirty days the cell turns red: an expired
                            certificate is the most frequent reason a service in
                            one's own house suddenly stops being reachable, and
                            the only one that could be seen weeks in
                            advance. */}
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
                  {(rechner?.rechner ?? []).filter(
                    (r) =>
                      !rechnerFilter.trim() ||
                      (r.name + " " + r.ziel + " " + (r.notiz ?? ""))
                        .toLowerCase()
                        .includes(rechnerFilter.trim().toLowerCase()),
                  ).length === 0 && (
                    <tr>
                      <td colSpan={7} className="muted">
                        {(rechner?.rechner ?? []).length === 0
                          ? "Noch kein Rechner eingetragen."
                          : "Kein Rechner passt auf den Filter."}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            {rechnerFehler && <div className="fehler">{rechnerFehler}</div>}

            {rechnerOffen && (
              <Fenster
                titel="Rechner hinzufügen"
                unter="Eine Adresse, an der diese Instanz anklopft"
                schliessen={() => setRechnerOffen(false)}
                fuss={
                  <>
                    <span className="fuss-luecke" />
                    <button className="btn" onClick={() => setRechnerOffen(false)}>
                      Abbrechen
                    </button>
                    <button
                      className="btn btn-primary"
                      disabled={!neuerRechner.ziel.trim()}
                      onClick={async () => {
                        await rechnerAnlegen();
                        setRechnerOffen(false);
                      }}
                    >
                      Hinzufügen
                    </button>
                  </>
                }
              >
                <div className="fenster-felder">
                  <label>
                    <span>Name</span>
                    <input
                      placeholder="optional"
                      value={neuerRechner.name}
                      onChange={(e) => setNeuerRechner({ ...neuerRechner, name: e.target.value })}
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
                  <label className="feld-breit">
                    <span>Adresse</span>
                    <input
                      placeholder="10.0.0.5:22 oder https://10.0.0.5:8006"
                      value={neuerRechner.ziel}
                      onChange={(e) => setNeuerRechner({ ...neuerRechner, ziel: e.target.value })}
                    />
                  </label>
                </div>
                <p className="muted small">
                  Erkannt wird aus dem Banner: SSH nennt seine Fassung, HTTP die Kopfzeile{" "}
                  <code>Server</code>, TLS das Zertifikat samt Ablauf. Antwortet ein Dienst nicht, bleibt das Feld leer. Geraten wird nichts.
                </p>
              </Fenster>
            )}

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
              Datenbank und Anhänge in einem Archiv, als Strom durch den Browser. Ein
              Dump allein lässt Zeilen zurück, die auf keine Datei mehr zeigen.
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
                  die Datei <code>FERTIG</code>. Fehlt sie, ist die Sicherung abgebrochen: ein halbes ZIP bleibt ein gültiges ZIP.
                </p>
                <p className="muted small">
                  Der Suchindex fehlt im Archiv und wird nicht gebraucht. PostgreSQL
                  baut ihn beim Einspielen aus Titel und Text neu auf.
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
                  Ein Knopf im Browser sichert nur, wenn jemand ihn drückt. Für einen
                  Zeitplan braucht ein Skript einen eigenen Zugang, und ein Cookie hat
                  es nicht. Dafür ist das Losungswort da.
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
                      Fertiges Skript mit Wort und Adresse. Prüft die Marke <code>FERTIG</code> und löscht Archive nach 14 Tagen.
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
                  Zugangsdaten sind maskiert. Zeilen mit <code>********</code> bleiben beim Speichern unverändert; einen neuen Wert einfach an diese Stelle
              schreiben.
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
                    Alles, was diese Fassung auswertet. Unbekannte Schlüssel werden beim
                    Start übergangen.
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
              Nötig, damit Änderungen an der Konfigurationsdatei greifen. Gelesen wird sie nur
              beim Start. Betrifft <strong>allein diesen Dienst</strong>; Oberfläche und
              Datenbank laufen weiter. 1–2 s ohne Antwort.
            </p>
            <p className="muted small">
              Der Dienst beendet sich selbst. Gestartet wird er von dem, was ihn betreibt:
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
              Zur Bestätigung <code>neustart</code> eintippen. Ein einzelner Knopf würde den Dienst schon beim Danebenklicken abschalten.
            </p>

            <h3>Papierkorb der Instanz</h3>
            <p className="muted small">
              Löscht alle Seiten im Papierkorb endgültig, auch die anderer Konten. Es
              gibt keine Frist und keinen Aufschub, der Lauf beginnt sofort.
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
            {b.titel}
          </button>
        ))}
      </nav>

      <div className="einstellungen-inhalt">
        {/* The heading used to stand only in the sidebar on the left. An area with a
            single part thereby got by without a title at all: one read the
            first table without anything anywhere saying what one was looking
            at. The short sentence below it explains the area once and therefore
            no longer has to be repeated beside every entry of the sidebar. */}
        <header className="einstellungen-kopf">
          <h1>{BEREICHE.find((b) => b.id === bereich)?.titel}</h1>
          <p>{BEREICHE.find((b) => b.id === bereich)?.unter}</p>
        </header>
        {meldung && (
          <div className={meldung.art === "ok" ? "hinweis-ok" : "fehler"}>{meldung.text}</div>
        )}
        {teile.map((t) => (
          <Fragment key={t}>
            {teile.length > 1 && (
              <h2 className="teil-titel" id={"teil-" + t}>
                {TEIL_TITEL[t]}
              </h2>
            )}
            {teilInhalt(t)}
          </Fragment>
        ))}
      </div>
    </div>
  );
}
