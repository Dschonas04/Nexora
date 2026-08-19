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

import { Einstellung, SystemZustand, api } from "../api/client";
import { useAuth } from "../auth";
import { anwenden, useDesign } from "../design";

type Bereich = "uebersicht" | "sicherheit" | "datenbank" | "suche" | "anhaenge" | "aussehen" | "lizenz" | "system";

const BEREICHE: { id: Bereich; titel: string; unter: string }[] = [
  { id: "uebersicht", titel: "Übersicht", unter: "Zahlen und Zustand auf einen Blick" },
  { id: "sicherheit", titel: "Sicherheit", unter: "Registrierung, Sitzungen, Administratoren" },
  { id: "datenbank", titel: "Datenbank", unter: "Größe, Tabellen, Belegung" },
  { id: "suche", titel: "Suche", unter: "Wörterbuch und Suchindex" },
  { id: "anhaenge", titel: "Anhänge", unter: "Größenbegrenzung und Belegung" },
  { id: "aussehen", titel: "Aussehen", unter: "Grundton und Akzentfarbe" },
  { id: "lizenz", titel: "Lizenz", unter: "Umfang und Laufzeit" },
  { id: "system", titel: "System", unter: "Was nur beim Start gilt" },
];

const ZUSATZ: Record<string, string> = {
  versionen: "Versionsverlauf",
  anhaenge: "Anhänge",
  freigeben: "Teilen und öffentliche Links",
  pruefspur: "Prüfspur",
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
  spureintraege: "Prüfspur-Einträge",
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

export default function EinstellungenView() {
  const { user } = useAuth();
  const { neuLaden: designNeuLaden } = useDesign();

  const [bereich, setBereich] = useState<Bereich>("uebersicht");
  const [liste, setListe] = useState<Einstellung[]>([]);
  const [zustand, setZustand] = useState<SystemZustand | null>(null);
  const [entwurf, setEntwurf] = useState<Record<string, string>>({});
  const [meldung, setMeldung] = useState<{ text: string; art: "ok" | "fehler" } | null>(null);
  const [laeuft, setLaeuft] = useState<string | null>(null);
  const [laedt, setLaedt] = useState(true);

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
      // Vorschau zurücknehmen, sonst zeigt die Oberfläche eine Farbe, die der
      // Server nie angenommen hat.
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
        <h2>Einstellungen</h2>
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
            {feld("sitzung_tage")}

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
              Vollständig nachlesbar unter <strong>Prüfspur</strong>. Passwörter werden dort
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
                    // Sofort anwenden, dann speichern: eine Farbe erst nach der
                    // Antwort des Servers zu sehen macht das Auswählen zur Qual.
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
                <h3>Umfang</h3>
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
            <p className="muted small">
              Der Schlüssel wird beim Start gelesen (<code>NEXORA_LIZENZ</code> oder{" "}
              <code>lizenz</code> in <code>config.conf</code>) und lässt sich hier bewusst
              nicht ändern — ein Geheimnis gehört nicht in ein Formular, das jeder
              Administrator öffnen kann.
            </p>
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
    }
  };

  return (
    <div className="einstellungen-manager">
      <nav className="einstellungen-nav">
        <div className="einstellungen-nav-titel">Einstellungen</div>
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
