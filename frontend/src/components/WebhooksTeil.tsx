// Webhooks in the administration: an address that is called when pages are
// created, changed, deleted or commented on.
//
// The secret signs every delivery and is shown once, when the hook is made.
// The receiver checks X-Nexora-Signature against it; n8n does that with a
// Crypto node, see the README.
import { useEffect, useState } from "react";
import { Webhook, api } from "../api/client";
import { useRueckfrage } from "./Rueckfrage";

const EREIGNIS_NAMEN: Record<string, string> = {
  "page.created": "Page created",
  "page.updated": "Page changed",
  "page.deleted": "Page deleted",
  "page.restored": "Page restored",
  "comment.created": "New comment",
};

export default function WebhooksTeil() {
  const frage = useRueckfrage();
  const [hooks, setHooks] = useState<Webhook[]>([]);
  const [ereignisse, setEreignisse] = useState<string[]>([]);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [gewaehlt, setGewaehlt] = useState<string[]>([]);
  const [neu, setNeu] = useState<Webhook | null>(null);
  const [fehler, setFehler] = useState("");
  const [probe, setProbe] = useState<Record<string, string>>({});

  const laden = () =>
    api
      .webhooks()
      .then((a) => {
        setHooks(a.hooks);
        setEreignisse(a.events);
      })
      .catch((e) => setFehler(e instanceof Error ? e.message : "Not loaded."));
  useEffect(() => {
    laden();
  }, []);

  const anlegen = async () => {
    setFehler("");
    try {
      const h = await api.webhookAnlegen({ name: name.trim(), url: url.trim(), events: gewaehlt });
      setNeu(h);
      setName("");
      setUrl("");
      setGewaehlt([]);
      laden();
    } catch (e) {
      setFehler(e instanceof Error ? e.message : "Not saved.");
    }
  };

  const umschalten = async (h: Webhook) => {
    await api.webhookAendern(h.id, { active: !h.active }).catch(() => {});
    laden();
  };

  const loeschen = async (h: Webhook) => {
    if (
      !(await frage({
        titel: "Delete webhook",
        text: "This address is no longer called.",
        bestaetigen: "Delete",
        gefaehrlich: true,
      }))
    )
      return;
    await api.webhookLoeschen(h.id).catch(() => {});
    if (neu?.id === h.id) setNeu(null);
    laden();
  };

  const testen = async (h: Webhook) => {
    setProbe((p) => ({ ...p, [h.id]: "…" }));
    try {
      const r = await api.webhookTesten(h.id);
      setProbe((p) => ({ ...p, [h.id]: r.error ? r.error : `HTTP ${r.status}` }));
    } catch (e) {
      setProbe((p) => ({ ...p, [h.id]: e instanceof Error ? e.message : "failed" }));
    }
    laden();
  };

  const ereignisUmschalten = (e: string) =>
    setGewaehlt((g) => (g.includes(e) ? g.filter((x) => x !== e) : [...g, e]));

  return (
    <>
      <h3>Webhooks</h3>
      <p className="muted small">
        Nexora calls these addresses with a POST and a JSON body when something happens. "Page changed" is sent once a page has been quiet for 30 seconds, not on every save. Every delivery carries X-Nexora-Signature: sha256= and the HMAC-SHA256 of the body with the secret.
      </p>

      {neu?.secret && (
        <div className="hinweis-ok schluessel-neu">
          <strong>The secret for checking signatures – shown only this once.</strong>
          <div className="schluessel-zeile">
            <input className="input notranslate" readOnly value={neu.secret} onFocus={(e) => e.target.select()} />
          </div>
        </div>
      )}

      <div className="einstellung">
        <div className="schluessel-form">
          <input className="input" placeholder="Name, e.g. n8n" value={name} onChange={(e) => setName(e.target.value)} />
          <input
            className="input breit"
            placeholder="https://…"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
        </div>
        <div className="ereignis-wahl">
          {ereignisse.map((e) => (
            <label key={e} className="postfach-schalter">
              <input type="checkbox" checked={gewaehlt.includes(e)} onChange={() => ereignisUmschalten(e)} />
              {EREIGNIS_NAMEN[e] ?? e}
            </label>
          ))}
          <span className="muted small">None ticked means all.</span>
        </div>
        <button className="btn btn-primary" disabled={!url.trim()} onClick={anlegen}>
          Add webhook
        </button>
        {fehler && <div className="fehler">{fehler}</div>}
      </div>

      {hooks.length === 0 ? (
        <p className="muted small">No webhooks yet.</p>
      ) : (
        <div className="list">
          {hooks.map((h) => (
            <div key={h.id} className="version-row">
              <div className="webhook-angaben">
                <div className="version-title">
                  <span className="notranslate">{h.name || h.url}</span>
                  {!h.active && <span className="pill readonly">off</span>}
                </div>
                <div className="muted small notranslate">{h.url}</div>
                <div className="muted small">
                  {h.events.length === 0 ? "All events" : h.events.map((e) => EREIGNIS_NAMEN[e] ?? e).join(", ")}
                </div>
                {h.lastAt && (
                  <div className={"small " + (h.lastError ? "fehler-text" : "muted")}>
                    {`Last delivery ${new Date(h.lastAt).toLocaleString()}: ${h.lastError || "HTTP " + h.lastStatus}`}
                  </div>
                )}
                {probe[h.id] && <div className="small muted">{`Test: ${probe[h.id]}`}</div>}
              </div>
              <div className="version-knoepfe">
                <button className="btn" onClick={() => testen(h)}>
                  Test
                </button>
                <button className="btn" onClick={() => umschalten(h)}>
                  {h.active ? "Pause" : "Resume"}
                </button>
                <button className="btn" onClick={() => loeschen(h)}>
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  );
}
