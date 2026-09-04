// Small inline graph under a page: the page in the center and its immediate
// neighbors.
//
// It reuses the same `Grafbild` as the larger view and therefore supports
// dragging, panning and zooming. Previously this was a static star layout
// with fixed angles; nodes could overlap and remain stuck.
//
// The data comes from the graph the page already requested, so no extra API
// call is required.
import { Graph } from "../api/client";
import Grafbild, { PHYSIK_KLEIN } from "./Grafbild";

interface Props {
  graph: Graph;
  pageId: string;
  onOpen: (id: string) => void;
}

export default function LocalGraph({ graph, pageId, onOpen }: Props) {
  // The neighborhood: this page and everything that shares an edge with it.
  // Undirected: incoming and outgoing links are both considered neighbors for
  // the question "what is related to this page?".
  const dabei = new Set<string>([pageId]);
  for (const e of graph.edges) {
    if (e.source === pageId) dabei.add(e.target);
    else if (e.target === pageId) dabei.add(e.source);
  }

  const knoten = graph.nodes.filter((n) => dabei.has(n.id));
  // Without the own page present (for example while loading) there is nothing
  // to show.
  if (knoten.length <= 1 || !knoten.some((n) => n.id === pageId)) return null;

  // Also include edges between neighbors, not only those to the center node:
  // relationships among neighbors are exactly what a graph visualization can
  // show more clearly than a list.
  const kanten = graph.edges.filter((e) => dabei.has(e.source) && dabei.has(e.target));

  return (
    <div className="local-graph">
      <div className="local-graph-title">Lokaler Graf</div>
      <Grafbild
        graph={{ nodes: knoten, edges: kanten }}
        onOeffnen={onOpen}
        mitte={pageId}
        physik={PHYSIK_KLEIN}
        // The height grows with the number of nodes: with two nodes the lower
        // half of the box looked empty, with twelve it became cramped. Cap the
        // maximum so the graph remains an inline image and not a second view.
        hoehe={Math.min(340, 150 + knoten.length * 22)}
        // Scrolling with the wheel scrolls the page, not the zoom: a graph
        // that fixed the text while shrinking itself during scroll would be a
        // trap in the middle of the page.
        radZoom="mit-strg"
        hinweis="Knoten ziehen · Strg + Rad zoomt"
      />
    </div>
  );
}
