// The audit trail, for admins.
//
// It answers one question: who did what, when. Everything here serves reading
// it back under pressure — during an incident or an audit — which is why the
// filters sit at the top and the newest entry is the first one.
//
// It sits inside the settings and therefore brings no frame of its own along;
// heading and spacing come from there. Before this it was a page of its own,
// reachable through a row of its own in the sidebar -- that is the one
// administrative matter that did not sit in the administration.
import { useEffect, useState } from "react";

import { Spureintrag, api } from "../api/client";
import KurzeZeilen from "../components/Kurzliste";
import Listenkopf from "../components/Listenkopf";
import { useLizenz } from "../lizenz";

// Readable German for the action names the backend records. An unknown name
// falls through to itself rather than to "unbekannt": a trail that hides what
// it does not recognise is worse than one that shows a raw string.
const BESCHRIFTUNG: Record<string, string> = {
  anmeldung: "Sign-in",
  "anmeldung.fehlgeschlagen": "Sign-in failed",
  abmeldung: "Sign-out",
  "konto.angelegt": "Account created",
  "konto.geloescht": "Account deleted",
  "konto.rolle": "Role changed",
  "konto.passwort": "Password changed",
  "konto.passwort.gesetzt": "Password reset",
  "konto.passwort.fehlgeschlagen": "Password change refused",
  "zweitfaktor.eingeschaltet": "Second factor switched on",
  "zweitfaktor.ausgeschaltet": "Second factor switched off",
  "zweitfaktor.zurueckgesetzt": "Second factor removed by an administrator",
  "zweitfaktor.ersatzcodes": "Recovery codes regenerated",
  "seite.angelegt": "Page created",
  "seite.geaendert": "Page changed",
  "seite.geloescht": "Moved to trash",
  "seite.entfernt": "Deleted permanently",
  "seite.wiederhergestellt": "Restored",
  "version.zurueckgeholt": "Version restored",
  "freigabe.erteilt": "Share granted",
  "freigabe.entzogen": "Share revoked",
  "oeffentlich.an": "Made public",
  "oeffentlich.aus": "Public link withdrawn",
  "anhang.hochgeladen": "Attachment uploaded",
  "anhang.bearbeitet": "Attachment edited",
  "anhang.entfernt": "Attachment removed",
  "kommentar.angelegt": "Comment written",
  "kommentar.geaendert": "Comment edited",
  "kommentar.geloescht": "Comment deleted",
  "kommentar.erledigt": "Thread resolved or reopened",
  "einstellung.geaendert": "Setting changed",
  "einstellung.zurueckgesetzt": "Setting reset",
  "suchindex.neu": "Search index rebuilt",
  "lizenz.geladen": "Licence loaded",
  // The templates no longer exist. The two names stay all the same: in an
  // audit trail reaching back years they still appear, and a row showing
  // nothing but its raw key is unreadable at exactly the point where somebody
  // is looking something up.
  "vorlage.gesetzt": "Marked as a template (feature removed)",
  "vorlage.aufgehoben": "Template mark lifted (feature removed)",
  "space.exportiert": "Space exported",
  "space.oeffentlich": "Space visibility changed",
  "gruppe.angelegt": "Group created",
  "gruppe.geloescht": "Group deleted",
  "gruppe.beigetreten": "Added to the group",
  "gruppe.ausgetreten": "Removed from the group",
  "spacerecht.erteilt": "Permission on a space granted",
  "spacerecht.entzogen": "Permission on a space revoked",
  "anhangindex.nachgezogen": "Attachment index caught up",
  "konfiguration.geaendert": "Configuration file changed",
  "dienst.neustart": "Service restarted",
  "papierkorb.geleert": "Instance trash emptied",
};

// beschriften returns the readable name. If one is missing, at least the raw
// name is defused: dots to arrows, transliterations back to umlauts. A new
// action in the backend shall not appear in the trail as "seite.geaendert"
// merely because a line was forgotten here.
function beschriften(aktion: string): string {
  const bekannt = BESCHRIFTUNG[aktion];
  if (bekannt) return bekannt;
  return aktion
    .replace(/ae/g, "ä")
    .replace(/oe/g, "ö")
    .replace(/ue/g, "ü")
    .split(".")
    .map((teil, i) => (i === 0 ? teil.charAt(0).toUpperCase() + teil.slice(1) : teil))
    .join(" → ");
}

// Actions worth spotting at a glance. Deleting and failed sign-ins are what an
// auditor scans for; the rest is noise until it is not.
const AUFFAELLIG = new Set([
  "anmeldung.fehlgeschlagen",
  "seite.entfernt",
  "konto.geloescht",
  "konto.rolle",
  "konto.passwort.gesetzt",
  "konto.passwort.fehlgeschlagen",
  // Removing another account's second factor is the route a takeover runs
  // through when somebody talks an administrator into it. It belongs among the
  // things an audit looks for.
  "zweitfaktor.zurueckgesetzt",
  "oeffentlich.an",
]);

function zeit(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString(undefined, {
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
        setFehler(e.status === 403 ? "Administrators only." : e.message),
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
      <>
        <h3>Audit log</h3>
        <p className="muted small">
          This feature belongs to the paid scope and is not included in the licence
          currently installed. Recording continues; without a licence it just cannot be read.
        </p>
      </>
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
    <div className="pruefspur">
      {/* Two filters that do different things, and therefore sit in
          different places: the choice of event type asks the server again, the
          text field merely narrows down what is already loaded. If both stood
          side by side they would look like two halves of the same thing. */}
      <Listenkopf
        titel="Audit log"
        zahl={
          eintraege.length === 0
            ? undefined
            : sichtbar.length === eintraege.length
              ? `${eintraege.length} Einträge`
              : `${sichtbar.length} von ${eintraege.length} Einträgen`
        }
        filter={suche}
        setFilter={setSuche}
        platzhalter="Search the entries…"
      >
        <select value={filter} onChange={(e) => setFilter(e.target.value)}>
          <option value="">All actions</option>
          {aktionen.map((a) => (
            <option key={a.aktion} value={a.aktion}>
              {beschriften(a.aktion) + ` (${a.anzahl})`}
            </option>
          ))}
        </select>
      </Listenkopf>
      <p className="muted small">
        Who did what, and when. Recording runs regardless of the licence so the log does
        not end up with gaps. At most 500 entries per query.
      </p>

      {fehler && <div className="fehler">{fehler}</div>}
      {laedt && <div className="muted">Loading…</div>}

      {!laedt && sichtbar.length === 0 && !fehler && (
        <div className="muted">No entries.</div>
      )}

      {sichtbar.length > 0 && (
        <div className="tabelle-rollen">
        <table className="tabelle pruefspur-tabelle">
          <thead>
            <tr>
              <th>When</th>
              <th>Who</th>
              <th>Action</th>
              <th>Concerns</th>
              <th>Address</th>
            </tr>
          </thead>
          <tbody>
            <KurzeZeilen
              alle={sichtbar}
              spalten={5}
              zeile={(e) => (
              <tr key={e.id} className={AUFFAELLIG.has(e.aktion) ? "auffaellig" : undefined}>
                <td className="einzeilig">{zeit(e.zeitpunkt)}</td>
                <td>
                  {e.akteurName || <span className="muted">unknown</span>}
                  {e.akteurEmail && <div className="muted small">{e.akteurEmail}</div>}
                </td>
                <td>{beschriften(e.aktion)}</td>
                <td>
                  {e.objektTitel || <span className="muted">{e.objektArt || "—"}</span>}
                  {/* Details sind je Vorgang verschieden, etwa an wen freigegeben
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
              )}
            />
          </tbody>
        </table>
        </div>
      )}
    </div>
  );
}
