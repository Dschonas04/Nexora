// Der Graf über den ganzen Arbeitsbereich.
//
// Die Seite holt die Daten und setzt den Rahmen; gezeichnet und gerechnet wird
// im Grafbild, das sich diese Ansicht mit dem kleinen Graf unter einer Seite
// teilt. Vorher stand die ganze Simulation hier, und der kleine Graf war ein
// starrer Stern aus fest gerechneten Winkeln -- dasselbe Bild, nur ohne alles,
// was den großen brauchbar macht.
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Graph, api } from "../api/client";
import Grafbild from "../components/Grafbild";

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });

  useEffect(() => {
    api.graph().then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
  }, []);

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
      hinweis="Knoten ziehen · Hintergrund ziehen · scrollen zum Zoomen"
      legende
      zentrieren
    />
  );
}
