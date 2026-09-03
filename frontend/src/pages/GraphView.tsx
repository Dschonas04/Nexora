// Der Graf über den ganzen Arbeitsbereich.
//
// Die Seite holt die Daten und setzt den Rahmen; gezeichnet und gerechnet wird
// im Grafbild, das sich diese Ansicht mit dem kleinen Graf unter einer Seite
// teilt. Vorher stand die ganze Simulation hier, und der kleine Graf war ein
// starrer Stern aus fest gerechneten Winkeln -- dasselbe Bild, nur ohne alles,
// was den großen brauchbar macht.
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Graph, Space, api } from "../api/client";
import Grafbild from "../components/Grafbild";

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  // Die Ablagen nur wegen ihrer Farben und wegen der Frage, wer sie ändern
  // darf. Wer eine Ablage nicht verwalten darf, sieht ihre Farbe und kann sie
  // nicht setzen -- eine Farbe gilt für alle, also entscheidet sie nicht jeder.
  const [ablagen, setAblagen] = useState<Space[]>([]);

  useEffect(() => {
    api.graph().then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
    api.listSpaces().then(setAblagen).catch(() => setAblagen([]));
  }, []);

  const eigeneFarben: Record<string, string> = {};
  for (const a of ablagen) if (a.farbe) eigeneFarben[a.id] = a.farbe;
  // Je Ablage und nicht pauschal: wer EINE Ablage verwaltet, verwaltet darum
  // nicht jede, die im Grafen vorkommt -- fremde stehen dort mit, sobald eine
  // Seite daraus geteilt ist.
  const faerbbar = new Set(ablagen.filter((a) => a.darfVerwalten).map((a) => a.id));

  // Sofort im Bild und erst danach in der Datenbank: eine Farbe, die eine
  // Zehntelsekunde später umspringt, fühlt sich an wie ein Aussetzer.
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
