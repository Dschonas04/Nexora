// The graph for the whole workspace.
//
// This page fetches the data and provides the frame; rendering and physics
// are handled by Grafbild, which this view shares with the small inline
// graph under a page. Previously the whole simulation lived here and the
// small graph was a static star layout with fixed angles — the same picture
// but missing all the features that make the large one useful.
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Graph, Space, api } from "../api/client";
import Grafbild from "../components/Grafbild";

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  // Spaces are fetched only for their colors and to know who may change them.
  // If you cannot manage a space you still see its color but cannot set it —
  // a color applies to everyone, so it is not decided by each user.
  const [ablagen, setAblagen] = useState<Space[]>([]);

  useEffect(() => {
    api.graph().then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
    api.listSpaces().then(setAblagen).catch(() => setAblagen([]));
  }, []);

  const eigeneFarben: Record<string, string> = {};
  for (const a of ablagen) if (a.farbe) eigeneFarben[a.id] = a.farbe;
  // Per-space, not blanket: managing one space does not imply managing every
  // space that appears in the graph — foreign spaces appear when a page from
  // them is shared.
  const faerbbar = new Set(ablagen.filter((a) => a.darfVerwalten).map((a) => a.id));

  // Apply color immediately in the UI and then persist it to the database:
  // a color that toggles a fraction of a second later feels like a glitch.
  const farbeSetzen = (spaceId: string, farbe: string) => {
    setAblagen((vorher) => vorher.map((a) => (a.id === spaceId ? { ...a, farbe } : a)));
    api.spaceFarbe(spaceId, farbe).catch(() => {
      api.listSpaces().then(setAblagen).catch(() => undefined);
    });
  };

  if (graph.nodes.length === 0) {
    return (
      <div className="graph-wrap">
        <div className="empty-state">Noch keine Seiten für den Graf.</div>
      </div>
    );
  }

  return (
    <Grafbild
      graph={graph}
      onOeffnen={(id) => nav(`/page/${id}`)}
      hinweis="Knoten ziehen · Hintergrund ziehen · scrollen zum Zoomen · Punkt in der Legende färbt die Ablage"
      legende
      zentrieren
      eigeneFarben={eigeneFarben}
      onFarbe={faerbbar.size > 0 ? farbeSetzen : undefined}
      faerbbar={faerbbar}
    />
  );
}
