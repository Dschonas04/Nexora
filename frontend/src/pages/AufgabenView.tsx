// My tasks: the checklist items of every page that name you.
//
// A task is an ordinary checklist item in some page. "@username" in its text
// says who it is for, "📅 2026-10-30" when it is due. This view gathers them
// from all pages one may read; ticking one here ticks it on its page.
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router";

import { Aufgabe, api } from "../api/client";
import { faelligkeit, ohneMarken } from "../aufgaben";

export default function AufgabenView() {
  const nav = useNavigate();
  const [filter, setFilter] = useState<"mine" | "own">("mine");
  const [erledigte, setErledigte] = useState(false);
  const [liste, setListe] = useState<Aufgabe[] | null>(null);
  const [ich, setIch] = useState("");
  const [fehler, setFehler] = useState("");

  const laden = useCallback(() => {
    api
      .aufgaben(filter, erledigte)
      .then((a) => {
        setListe(a.tasks);
        setIch(a.username);
      })
      .catch(() => setListe([]));
  }, [filter, erledigte]);

  useEffect(() => {
    laden();
  }, [laden]);

  const abhaken = async (a: Aufgabe) => {
    setFehler("");
    const neu = !a.checked;
    // Shown at once, put back if the service refuses.
    setListe((l) => l?.map((x) => (x === a ? { ...x, checked: neu } : x)) ?? null);
    try {
      await api.aufgabeAbhaken(a.pageId, a.blockId, neu);
    } catch (e) {
      setListe((l) => l?.map((x) => (x.blockId === a.blockId && x.pageId === a.pageId ? a : x)) ?? null);
      setFehler((e as Error).message);
    }
  };

  const offen = liste?.filter((a) => !a.checked).length ?? 0;

  return (
    <div className="editor-scroll">
      <div className="page wide">
        <h1 className="view-title">Tasks</h1>
        <p className="muted">
          Checklist items from all pages you can read. Write @username into a
          checklist item to give it to someone, and 📅 with a date for when it is
          due.
        </p>

        <div className="postfach-leiste">
          <div className="umschalter" role="tablist">
            <button
              role="tab"
              aria-selected={filter === "mine"}
              className={"btn" + (filter === "mine" ? " active" : "")}
              onClick={() => setFilter("mine")}
            >
              For me
            </button>
            <button
              role="tab"
              aria-selected={filter === "own"}
              className={"btn" + (filter === "own" ? " active" : "")}
              onClick={() => setFilter("own")}
            >
              On my pages
            </button>
          </div>
          <label className="postfach-schalter">
            <input type="checkbox" checked={erledigte} onChange={(e) => setErledigte(e.target.checked)} />
            Show done ones
          </label>
          <span className="wachsen" />
          {liste && <span className="muted small">{offen} open</span>}
        </div>

        {fehler && <div className="hinweis-fehler">{fehler}</div>}

        {filter === "mine" && liste && !ich && (
          <div className="postfach-leer">
            <strong>Your account has no user name yet.</strong>
            <p className="muted small">
              Tasks find you through @username. Set one under My account, then
              others can give you tasks.
            </p>
          </div>
        )}

        {liste && liste.length === 0 && (filter === "own" || ich) && (
          <div className="postfach-leer">
            <div className="postfach-leer-zeichen" aria-hidden="true">
              &#10003;
            </div>
            <strong>No open tasks.</strong>
            {filter === "mine" && (
              <p className="muted small">
                {`When someone writes @${ich} into a checklist, the task appears here.`}
              </p>
            )}
          </div>
        )}

        {liste && liste.length > 0 && (
          <div className="list">
            {liste.map((a) => {
              const f = faelligkeit(a.due);
              return (
                <div key={a.pageId + a.blockId} className={"list-row aufgabe-zeile" + (a.checked ? " erledigt" : "")}>
                  <input
                    type="checkbox"
                    className="aufgabe-haken"
                    checked={a.checked}
                    disabled={!a.canEdit}
                    title={a.canEdit ? "Tick off" : "Read only"}
                    aria-label="Tick off"
                    onChange={() => abhaken(a)}
                  />
                  <span className="aufgabe-inhalt">
                    <span className="aufgabe-text notranslate">{ohneMarken(a.text) || a.text}</span>
                    <span className="aufgabe-meta">
                      <button className="link-btn" onClick={() => nav(`/page/${a.pageId}`)}>
                        {a.pageTitle || "Untitled"}
                      </button>
                      {a.assignees.map((p) => (
                        <span key={p} className={"aufgabe-person notranslate" + (p === ich ? " ich" : "")}>
                          @{p}
                        </span>
                      ))}
                    </span>
                  </span>
                  {a.due && (
                    <span className={"aufgabe-datum " + f.art} title={a.due}>
                      {f.text}
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
