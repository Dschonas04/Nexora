// The audit trail, for admins.
//
// It answers one question: who did what, when. Everything here serves reading
// it back under pressure — during an incident or an audit — which is why the
// filters sit at the top and the newest entry is the first one.
import { useEffect, useState } from "react";

import { Spureintrag, api } from "../api/client";
import { useLizenz } from "../lizenz";

// Readable German for the action names the backend records. An unknown name
// falls through to itself rather than to "unbekannt": a trail that hides what
// it does not recognise is worse than one that shows a raw string.
const BESCHRIFTUNG: Record<string, string> = {
  anmeldung: "Anmeldung",
  "anmeldung.fehlgeschlagen": "Anmeldung fehlgeschlagen",
  abmeldung: "Abmeldung",
  "konto.angelegt": "Konto angelegt",
  "konto.geloescht": "Konto gelöscht",
  "konto.rolle": "Rolle geändert",
  "seite.angelegt": "Seite angelegt",
  "seite.geaendert": "Seite geändert",
  "seite.geloescht": "In den Papierkorb",
  "seite.entfernt": "Endgültig gelöscht",
  "seite.wiederhergestellt": "Wiederhergestellt",
  "version.zurueckgeholt": "Version zurückgeholt",
  "freigabe.erteilt": "Freigabe erteilt",
  "freigabe.entzogen": "Freigabe entzogen",
  "oeffentlich.an": "Öffentlich geschaltet",
  "oeffentlich.aus": "Öffentlich zurückgenommen",
  "anhang.hochgeladen": "Anhang hochgeladen",
  "anhang.entfernt": "Anhang entfernt",
};

// Actions worth spotting at a glance. Deleting and failed sign-ins are what an
// auditor scans for; the rest is noise until it is not.
const AUFFAELLIG = new Set([
  "anmeldung.fehlgeschlagen",
  "seite.entfernt",
  "konto.geloescht",
  "konto.rolle",
  "oeffentlich.an",
]);

function zeit(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString("de-DE", {
    day: "2-digit", month: "2-digit", year: "numeric",
    hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
}

export default function PruefspurView() {
  const { frei, geladen } = useLizenz();
  const [eintraege, setEintraege] = useState<Spureintrag[]>([]);
  const [aktionen, setAktionen] = useState<{ aktion: string; anzahl: number }[]>([]);
  const [filter, setFilter] = useState("");
  const [suche, setSuche] = useState("");
  const [fehler, setFehler] = useState<string | null>(null);
  const [laedt, setLaedt] = useState(true);

  useEffect(() => {
    if (!frei("pruefspur")) return;
    setLaedt(true);
    api
      .pruefspur({ aktion: filter || undefined, limit: 500 })
      .then((e) => {
        setEintraege(e);
        setFehler(null);
      })
      .catch((e: Error & { status?: number }) =>
        setFehler(e.status === 403 ? "Nur für Administratoren." : e.message),
      )
      .finally(() => setLaedt(false));
  }, [filter, frei]);

  useEffect(() => {
    if (!frei("pruefspur")) return;
    api.pruefspurAktionen().then(setAktionen).catch(() => setAktionen([]));
  }, [frei]);

  if (!geladen) return null;

  if (!frei("pruefspur")) {
    return (
      <div className="page-pad">
        <h2>Prüfspur</h2>
        <p className="muted">
          Diese Funktion gehört zum Zusatzumfang und ist in der vorliegenden Lizenz
          nicht enthalten.
        </p>
      </div>
    );
  }

  // Filtering the loaded rows in the browser rather than asking the server
  // again: the free-text box is meant for narrowing down what is already on
  // screen, and a round trip per keystroke would make that feel sluggish.
  const begriff = suche.trim().toLowerCase();
  const sichtbar = begriff
    ? eintraege.filter((e) =>
        [e.akteurName, e.akteurEmail, e.objektTitel, e.objektId, e.ip, e.aktion]
          .join(" ")
          .toLowerCase()
          .includes(begriff),
      )
    : eintraege;

  return (
    <div className="page-pad pruefspur">
      <h2>Prüfspur</h2>
      <p className="muted small">
        Wer hat wann was getan. Die Aufzeichnung läuft unabhängig von der Lizenz mit,
        damit die Spur keine Lücken bekommt.
      </p>

      <div className="pruefspur-filter">
        <select value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="">Alle Vorgänge</option>
          {aktionen.map((a) => (
            <option key={a.aktion} value={a.aktion}>
              {(BESCHRIFTUNG[a.aktion] ?? a.aktion) + ` (${a.anzahl})`}
            </option>
          ))}
        </select>
        <input
          placeholder="In den geladenen Einträgen suchen…"
          value={suche}
          onChange={(e) => setSuche(e.target.value)}
        />
        <span className="muted small">
          {sichtbar.length} von {eintraege.length}
        </span>
      </div>

      {fehler && <div className="fehler">{fehler}</div>}
      {laedt && <div className="muted">Lädt…</div>}

      {!laedt && sichtbar.length === 0 && !fehler && (
        <div className="muted">Keine Einträge.</div>
      )}

      {sichtbar.length > 0 && (
        <table className="tabelle pruefspur-tabelle">
          <thead>
            <tr>
              <th>Zeitpunkt</th>
              <th>Wer</th>
              <th>Vorgang</th>
              <th>Betrifft</th>
              <th>Adresse</th>
            </tr>
          </thead>
          <tbody>
            {sichtbar.map((e) => (
              <tr key={e.id} className={AUFFAELLIG.has(e.aktion) ? "auffaellig" : undefined}>
                <td className="einzeilig">{zeit(e.zeitpunkt)}</td>
                <td>
                  {e.akteurName || <span className="muted">unbekannt</span>}
                  {e.akteurEmail && <div className="muted small">{e.akteurEmail}</div>}
                </td>
                <td>{BESCHRIFTUNG[e.aktion] ?? e.aktion}</td>
                <td>
                  {e.objektTitel || <span className="muted">{e.objektArt || "—"}</span>}
                  {/* Details sind je Vorgang verschieden -- etwa an wen freigegeben
                      wurde. Roh anzuzeigen ist ehrlicher, als sie zu verbergen. */}
                  {e.details && Object.keys(e.details).length > 0 && (
                    <div className="muted small">
                      {Object.entries(e.details)
                        .map(([k, v]) => `${k}: ${String(v)}`)
                        .join(", ")}
                    </div>
                  )}
                </td>
                <td className="muted small einzeilig">{e.ip || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
