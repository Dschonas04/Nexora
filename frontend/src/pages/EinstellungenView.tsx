// The settings page, for administrators.
//
// It is laid out around one distinction that matters more than any single
// field: some settings can be changed while the server runs, others are fixed
// at start. Mixing them would produce a page where half the switches quietly do
// nothing. The changeable ones are on top and are editable; the fixed ones sit
// below, plainly marked as belonging to config.conf.
import { useCallback, useEffect, useState } from "react";

import { Einstellung, SystemZustand, api } from "../api/client";
import { useAuth } from "../auth";

// Readable names for the license features, in the order they were introduced.
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
  admins: "davon Administratoren",
  seiten: "Seiten",
  papierkorb: "im Papierkorb",
  versionen: "Versionen",
  anhaenge: "Anhänge",
  kommentare: "Kommentare",
  spureintraege: "Prüfspur-Einträge",
  ohneSuchtext: "Seiten ohne Suchtext",
};

export default function EinstellungenView() {
  const { user } = useAuth();
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
        // Der Entwurf startet auf den geltenden Werten, damit ein Feld nicht
        // leer erscheint, bevor jemand es anfasst.
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

  const speichern = async (e: Einstellung, wert: string) => {
    setLaeuft(e.schluessel);
    setMeldung(null);
    try {
      await api.einstellungSetzen(e.schluessel, wert);
      setMeldung({ text: `„${e.titel}“ gespeichert.`, art: "ok" });
      laden();
    } catch (err) {
      setMeldung({ text: (err as Error).message, art: "fehler" });
      // Entwurf auf den geltenden Wert zurück, sonst zeigt das Feld etwas an,
      // was der Server nie angenommen hat.
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

  const feld = (e: Einstellung) => {
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
                  onChange={(ev) =>
                    setEntwurf((v) => ({ ...v, [e.schluessel]: ev.target.value }))
                  }
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

        <div className="einstellung-fuss muted small">
          {e.ausDatei ? (
            <>Wert stammt aus <code>config.conf</code></>
          ) : (
            <>
              hier gesetzt
              {e.geaendertVon && <> von {e.geaendertVon}</>}
              {e.geaendertAm && <> am {e.geaendertAm}</>}
              {" · "}
              <button className="link-btn" onClick={() => zuruecksetzen(e)}>
                auf <code>config.conf</code> zurücksetzen ({e.vorgabe || "leer"})
              </button>
            </>
          )}
        </div>
      </div>
    );
  };

  const z = zustand;

  return (
    <div className="page-pad einstellungen">
      <h2>Einstellungen</h2>

      {meldung && (
        <div className={meldung.art === "ok" ? "hinweis-ok" : "fehler"}>{meldung.text}</div>
      )}

      {/* Gegen null abgesichert, nicht nur gegen undefined: eine leere Go-Liste
          kommt als null über die Leitung, und ein .length darauf hat diese Seite
          schon einmal zum Verschwinden gebracht. */}
      {z && (z.warnungen ?? []).length > 0 && (
        <div className="warnkasten">
          <strong>Beim Start bemängelt</strong>
          <ul>
            {(z.warnungen ?? []).map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
          <div className="muted small">
            Diese Punkte stehen in <code>config.conf</code> oder in der Umgebung und
            lassen sich hier nicht ändern.
          </div>
        </div>
      )}

      <section>
        <h3>Im Betrieb änderbar</h3>
        <p className="muted small">
          Diese Werte liegen in der Datenbank und greifen sofort. Sie überschreiben,
          was in <code>config.conf</code> steht.
        </p>
        {liste.map(feld)}
      </section>

      {z && (
        <>
          <section>
            <h3>Lizenz</h3>
            {z.lizenz.gueltig ? (
              <>
                <div className="kachelreihe">
                  <div className="kachel">
                    <div className="kachel-wert">{z.lizenz.inhaber}</div>
                    <div className="kachel-titel">Inhaber</div>
                  </div>
                  <div className="kachel">
                    <div className="kachel-wert">{z.lizenz.laeuftAb || "unbefristet"}</div>
                    <div className="kachel-titel">Gültig bis</div>
                  </div>
                  <div className="kachel">
                    <div className="kachel-wert">
                      {(z.lizenz.freigeschaltet ?? []).length} von {z.lizenz.alle}
                    </div>
                    <div className="kachel-titel">Zusätze frei</div>
                  </div>
                </div>
                <div className="zusatzliste">
                  {Object.entries(ZUSATZ).map(([k, titel]) => (
                    <span
                      key={k}
                      className={
                        "pill " + ((z.lizenz.freigeschaltet ?? []).includes(k) ? "frei" : "gesperrt")
                      }
                    >
                      {titel}
                    </span>
                  ))}
                </div>
              </>
            ) : (
              <p className="muted">
                Keine gültige Lizenz{z.lizenz.grund && <> — {z.lizenz.grund}</>}. Nexora läuft
                mit dem freien Umfang.
              </p>
            )}
          </section>

          <section>
            <h3>Zahlen</h3>
            <div className="kachelreihe">
              {Object.entries(z.zahlen ?? {}).map(([k, v]) => (
                <div className="kachel" key={k}>
                  <div className="kachel-wert">{v}</div>
                  <div className="kachel-titel">{ZAHL_TITEL[k] ?? k}</div>
                </div>
              ))}
              <div className="kachel">
                <div className="kachel-wert">{z.datenbankGroesse}</div>
                <div className="kachel-titel">Datenbank</div>
              </div>
            </div>
          </section>

          <section>
            <h3>Wartung</h3>
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
                    Läuft in Stapeln und blockiert nichts, kann bei vielen Seiten aber
                    einen Moment dauern.
                  </div>
                </div>
                <div className="einstellung-feld">
                  <button className="btn" disabled={laeuft === "suchindex"} onClick={indexNeu}>
                    {laeuft === "suchindex" ? "Läuft…" : "Neu aufbauen"}
                  </button>
                </div>
              </div>
            </div>
          </section>

          <section>
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
                  <td><code>{z.nurInDerDatei.datenVerzeichnis}</code></td>
                </tr>
                <tr>
                  <td>Öffentliche Adresse</td>
                  <td>{z.nurInDerDatei.oeffentlicheUrl || <span className="muted">nicht gesetzt</span>}</td>
                </tr>
                <tr>
                  <td>LDAP</td>
                  <td>
                    {z.nurInDerDatei.ldapAktiv ? (
                      <>an — <code>{z.nurInDerDatei.ldapServer || "kein Server angegeben"}</code></>
                    ) : (
                      <span className="muted">aus</span>
                    )}
                  </td>
                </tr>
                <tr>
                  <td>SSO über OIDC</td>
                  <td>
                    {z.nurInDerDatei.oidcAktiv ? (
                      <>an — <code>{z.nurInDerDatei.oidcAussteller || "kein Aussteller angegeben"}</code></>
                    ) : (
                      <span className="muted">aus</span>
                    )}
                  </td>
                </tr>
              </tbody>
            </table>
          </section>
        </>
      )}
    </div>
  );
}
