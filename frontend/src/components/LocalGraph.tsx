// Der kleine Graf unter einer Seite: die Seite in der Mitte, ihre unmittelbaren
// Nachbarn darum herum.
//
// Er benutzt dasselbe Grafbild wie die große Ansicht und ist damit ebenso zum
// Ziehen, Schieben und Zoomen. Vorher war er ein starrer Stern: Winkel fest
// gerechnet, nichts bewegte sich, und wo zwei Namen auf denselben Punkt fielen,
// blieben sie liegen.
//
// Die Daten kommen aus dem Graf, den die Seite ohnehin schon geholt hat, also
// kostet er keinen weiteren Aufruf.
import { Graph } from "../api/client";
import Grafbild, { PHYSIK_KLEIN } from "./Grafbild";

interface Props {
  graph: Graph;
  pageId: string;
  onOpen: (id: string) => void;
}

export default function LocalGraph({ graph, pageId, onOpen }: Props) {
  // Die Nachbarschaft: diese Seite und alles, was eine Kante mit ihr teilt.
  // Ungerichtet -- wer hierher verweist und wohin diese Seite verweist, ist
  // beides Nachbarschaft; für die Frage "was hängt hiermit zusammen" spielt die
  // Richtung keine Rolle.
  const dabei = new Set<string>([pageId]);
  for (const e of graph.edges) {
    if (e.source === pageId) dabei.add(e.target);
    else if (e.target === pageId) dabei.add(e.source);
  }

  const knoten = graph.nodes.filter((n) => dabei.has(n.id));
  // Ohne die eigene Seite im Graf -- etwa, solange er noch lädt -- gibt es
  // nichts zu zeigen.
  if (knoten.length <= 1 || !knoten.some((n) => n.id === pageId)) return null;

  // Auch die Kanten UNTER den Nachbarn kommen mit, nicht nur die zur Mitte:
  // dass zwei Nachbarn einander kennen, ist genau das, was ein Bild zeigen
  // kann und eine Liste nicht.
  const kanten = graph.edges.filter((e) => dabei.has(e.source) && dabei.has(e.target));

  return (
    <div className="local-graph">
      <div className="local-graph-title">Lokaler Graf</div>
      <Grafbild
        graph={{ nodes: knoten, edges: kanten }}
        onOeffnen={onOpen}
        mitte={pageId}
        physik={PHYSIK_KLEIN}
        hoehe={300}
        // Am Rad wird die Seite gescrollt, nicht gezoomt: ein Graf, der beim
        // Vorbeiscrollen den Text festhält und sich selbst zusammenzieht, ist
        // eine Falle mitten in der Seite.
        radZoom="mit-strg"
        hinweis="Knoten ziehen · Strg + Rad zoomt"
      />
    </div>
  );
}
