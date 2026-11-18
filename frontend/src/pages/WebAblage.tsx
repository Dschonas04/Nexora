// SPDX-License-Identifier: LicenseRef-Nexora-Enterprise
// Copyright (c) 2026 Jonas Groll. See LICENSE-ENTERPRISE.

// A whole space published on the web: page tree on the left, the page on the
// right, readable by anyone with the address. Built like the shared single
// page (PublicPage), with the tree as the difference: a handbook is read by
// moving through it, not by receiving one page.
import { Suspense, lazy, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { WebAblage, WebSeite, api } from "../api/client";
import Fehlergrenze from "../components/Fehlergrenze";

const Editor = lazy(() => import("../components/Editor"));

type Knoten = WebAblage["pages"][number];

function Baum({
  token,
  eltern,
  kinder,
  aktiv,
  onWahl,
}: {
  token: string;
  eltern: string | null;
  kinder: Map<string | null, Knoten[]>;
  aktiv?: string;
  onWahl: () => void;
}) {
  const liste = kinder.get(eltern) ?? [];
  if (liste.length === 0) return null;
  return (
    <ul>
      {liste.map((p) => (
        <li key={p.id}>
          <Link
            to={`/web/${token}/${p.id}`}
            className={p.id === aktiv ? "aktiv" : ""}
            onClick={onWahl}
            title={p.title || "Untitled"}
          >
            {p.icon ? `${p.icon} ` : ""}
            {p.title || "Untitled"}
          </Link>
          <Baum token={token} eltern={p.id} kinder={kinder} aktiv={aktiv} onWahl={onWahl} />
        </li>
      ))}
    </ul>
  );
}

export default function WebAblageView() {
  const { token, pageId } = useParams();
  const nav = useNavigate();
  const [ablage, setAblage] = useState<WebAblage | null>(null);
  const [seite, setSeite] = useState<WebSeite | null>(null);
  const [err, setErr] = useState(false);
  const [seiteFehlt, setSeiteFehlt] = useState(false);
  const [menueOffen, setMenueOffen] = useState(false);

  useEffect(() => {
    if (!token) return;
    api
      .getWebAblage(token)
      .then(setAblage)
      .catch(() => setErr(true));
  }, [token]);

  const kinder = useMemo(() => {
    const m = new Map<string | null, Knoten[]>();
    for (const p of ablage?.pages ?? []) {
      const l = m.get(p.parentId) ?? [];
      l.push(p);
      m.set(p.parentId, l);
    }
    return m;
  }, [ablage]);

  // Without a page in the address the first page of the tree is the start page.
  const ziel = pageId ?? kinder.get(null)?.[0]?.id;

  useEffect(() => {
    if (!token || !ziel) return;
    setSeiteFehlt(false);
    api
      .getWebSeite(token, ziel)
      .then((s) => {
        setSeite(s);
        document.querySelector(".webablage-rumpf .editor-scroll")?.scrollTo(0, 0);
      })
      .catch(() => setSeiteFehlt(true));
  }, [token, ziel]);

  useEffect(() => {
    if (!ablage) return;
    const vorher = document.title;
    document.title = seite?.title ? `${seite.title} · ${ablage.name}` : ablage.name;
    return () => {
      document.title = vorher;
    };
  }, [ablage, seite]);

  // [[Title]] links to another page of the same space lead there; anything
  // else stays plain text, it is not published.
  const nachTitel = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of ablage?.pages ?? []) m.set(p.title.trim().toLowerCase(), p.id);
    return m;
  }, [ablage]);

  if (err) return <div className="empty-state">This space is not available.</div>;
  if (!ablage) return <div className="empty-state spaet">Loading…</div>;

  return (
    <div className="oeffentlich">
      <div className="oeffentlich-kopf">
        <span className="oeffentlich-marke">{ablage.name}</span>
        <span className="oeffentlich-hinweis">Published with Nexora, read only</span>
        <button className="btn webablage-menue" onClick={() => setMenueOffen((v) => !v)}>
          Pages
        </button>
      </div>
      <div className="webablage-rumpf">
        <nav className={"webablage-baum" + (menueOffen ? " offen" : "")} aria-label="Pages">
          {token && (
            <Baum token={token} eltern={null} kinder={kinder} aktiv={ziel} onWahl={() => setMenueOffen(false)} />
          )}
        </nav>
        <div className="editor-scroll">
          {ablage.pages.length === 0 ? (
            <div className="empty-state">This space has no pages yet.</div>
          ) : seiteFehlt ? (
            <div className="empty-state">This page is not available.</div>
          ) : !seite || seite.id !== ziel ? (
            <div className="empty-state spaet">Loading…</div>
          ) : (
            <div className={"page" + (seite.width && seite.width !== "normal" ? " " + seite.width : "")}>
              <h1 className="page-title" style={{ cursor: "default" }}>
                {seite.icon && <span className="oeffentlich-symbol">{seite.icon}</span>}
                {seite.title || "Untitled"}
              </h1>
              <Fehlergrenze text="The content of this page could not be shown.">
                <Suspense fallback={<div className="qv-none">Loading…</div>}>
                  <Editor
                    key={seite.id}
                    initialContent={seite.content}
                    editable={false}
                    linkResolver={(titel) => nachTitel.get(titel.trim().toLowerCase()) ?? null}
                    onOpenLink={(id) => nav(`/web/${token}/${id}`)}
                  />
                </Suspense>
              </Fehlergrenze>
              <div className="oeffentlich-fuss">
                Stand:{" "}
                {new Date(seite.updatedAt).toLocaleDateString(undefined, {
                  day: "2-digit",
                  month: "long",
                  year: "numeric",
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
