import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Graph, api } from "../api/client";

interface Pos {
  x: number;
  y: number;
}

// Palette used to colour nodes (and their space anchors) per space. Pages
// without a space share the last, neutral colour.
const SPACE_COLORS = [
  "#2383e2", "#e2662c", "#159a6b", "#a84be0", "#d4356b",
  "#c99700", "#0f9bb0", "#7a52d6", "#5a8f3c", "#d05a2c",
];
const NO_SPACE_COLOR = "#9b9a97";

// spaceKey groups pages the same way the sidebar does: by their space, with a
// single shared bucket for pages that live outside any space.
function spaceKey(node: { spaceId: string | null }): string {
  return node.spaceId ?? "__none__";
}

// spaceOrder returns the distinct space keys in a stable order plus a colour
// lookup, so the layout can give every space its own region and hue.
function spaceInfo(graph: Graph) {
  const keys: string[] = [];
  for (const n of graph.nodes) {
    const k = spaceKey(n);
    if (!keys.includes(k)) keys.push(k);
  }
  // Real spaces first, the "no space" bucket last.
  keys.sort((a, b) => (a === "__none__" ? 1 : b === "__none__" ? 0 : 0));
  const color: Record<string, string> = {};
  let ci = 0;
  for (const k of keys) {
    color[k] = k === "__none__" ? NO_SPACE_COLOR : SPACE_COLORS[ci++ % SPACE_COLORS.length];
  }
  return { keys, color };
}

// A force-directed layout that orients itself around the folder/space
// structure (LogSeq-style): every space gets its own anchor region, nodes are
// pulled toward their space anchor, and parent-child (hierarchy) edges pull far
// harder than loose [[wiki-links]] — so nesting drives the shape, links only
// nudge it.
function layout(graph: Graph, width: number, height: number): Record<string, Pos> {
  const w = width || 900;
  const h = height || 600;
  const { keys, color: _c } = spaceInfo(graph);
  void _c;

  // One anchor per space, spread on a circle; a lone space sits in the centre.
  const anchor: Record<string, Pos> = {};
  const radius = Math.min(w, h) / 3;
  keys.forEach((k, i) => {
    if (keys.length === 1) {
      anchor[k] = { x: w / 2, y: h / 2 };
    } else {
      const a = (i / keys.length) * Math.PI * 2;
      anchor[k] = { x: w / 2 + Math.cos(a) * radius, y: h / 2 + Math.sin(a) * radius };
    }
  });

  // Seed each node near its space anchor so clusters start apart.
  const pos: Record<string, Pos> = {};
  graph.nodes.forEach((node, i) => {
    const c = anchor[spaceKey(node)];
    const a = (i / (graph.nodes.length || 1)) * Math.PI * 2;
    pos[node.id] = { x: c.x + Math.cos(a) * 40, y: c.y + Math.sin(a) * 40 };
  });

  for (let iter = 0; iter < 260; iter++) {
    const disp: Record<string, Pos> = {};
    for (const node of graph.nodes) disp[node.id] = { x: 0, y: 0 };

    // Repulsion — softened between same-space nodes so a space stays compact.
    for (let i = 0; i < graph.nodes.length; i++) {
      for (let j = i + 1; j < graph.nodes.length; j++) {
        const na = graph.nodes[i];
        const nb = graph.nodes[j];
        const a = na.id;
        const b = nb.id;
        let dx = pos[a].x - pos[b].x;
        let dy = pos[a].y - pos[b].y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
        const same = spaceKey(na) === spaceKey(nb);
        const force = (same ? 2600 : 5200) / (dist * dist);
        dx /= dist;
        dy /= dist;
        disp[a].x += dx * force;
        disp[a].y += dy * force;
        disp[b].x -= dx * force;
        disp[b].y -= dy * force;
      }
    }

    // Edge springs: hierarchy pulls tight and short, wiki-links pull gently.
    for (const e of graph.edges) {
      if (!pos[e.source] || !pos[e.target]) continue;
      let dx = pos[e.source].x - pos[e.target].x;
      let dy = pos[e.source].y - pos[e.target].y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const parent = e.kind === "parent";
      const rest = parent ? 62 : 150;
      const k = parent ? 0.09 : 0.015;
      const force = (dist - rest) * k;
      dx /= dist;
      dy /= dist;
      disp[e.source].x -= dx * force;
      disp[e.source].y -= dy * force;
      disp[e.target].x += dx * force;
      disp[e.target].y += dy * force;
    }

    // Gravity toward each node's space anchor keeps folders as clusters.
    for (const node of graph.nodes) {
      const c = anchor[spaceKey(node)];
      disp[node.id].x += (c.x - pos[node.id].x) * 0.06;
      disp[node.id].y += (c.y - pos[node.id].y) * 0.06;
    }

    for (const node of graph.nodes) {
      pos[node.id].x = Math.max(30, Math.min(w - 30, pos[node.id].x + disp[node.id].x * 0.05));
      pos[node.id].y = Math.max(30, Math.min(h - 30, pos[node.id].y + disp[node.id].y * 0.05));
    }
  }
  return pos;
}

// fitView centres the whole graph in the viewport (LogSeq-style): it measures
// the nodes' bounding box and returns the pan/zoom that puts its middle in the
// middle of the canvas, scaled to fit with a margin.
function fitView(pos: Record<string, Pos>, width: number, height: number): { x: number; y: number; scale: number } {
  const pts = Object.values(pos);
  if (pts.length === 0) return { x: 0, y: 0, scale: 1 };
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const p of pts) {
    if (p.x < minX) minX = p.x;
    if (p.x > maxX) maxX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.y > maxY) maxY = p.y;
  }
  const pad = 80; // room for node labels
  const gw = Math.max(maxX - minX, 1);
  const gh = Math.max(maxY - minY, 1);
  const scale = Math.max(0.3, Math.min(1.4, Math.min((width - 2 * pad) / gw, (height - 2 * pad) / gh)));
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  return { x: width / 2 - cx * scale, y: height / 2 - cy * scale, scale };
}

const DRAG_THRESHOLD = 4; // px before a press counts as a drag, not a click

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const wrapRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [size, setSize] = useState({ w: 900, h: 600 });
  const [positions, setPositions] = useState<Record<string, Pos>>({});
  const [view, setView] = useState({ x: 0, y: 0, scale: 1 });
  const [hover, setHover] = useState<string | null>(null);

  // Colour lookup + legend entries, grouped by space like the sidebar.
  const { color: spaceColor } = spaceInfo(graph);
  const legend = (() => {
    const seen = new Set<string>();
    const out: { key: string; label: string; color: string }[] = [];
    for (const n of graph.nodes) {
      const k = spaceKey(n);
      if (seen.has(k)) continue;
      seen.add(k);
      out.push({ key: k, label: k === "__none__" ? "Kein Space" : n.space || "Space", color: spaceColor[k] });
    }
    return out;
  })();

  // Live view kept in a ref so pointer math always reads current pan/zoom.
  const viewRef = useRef(view);
  viewRef.current = view;

  const drag = useRef<{
    kind: "node" | "pan" | null;
    id?: string;
    offX: number;
    offY: number;
    startX: number;
    startY: number;
    panX: number;
    panY: number;
    moved: boolean;
  }>({ kind: null, offX: 0, offY: 0, startX: 0, startY: 0, panX: 0, panY: 0, moved: false });

  useEffect(() => {
    api.graph().then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
  }, []);

  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const update = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // (Re)initialise positions only when the set of nodes changes — not on every
  // resize or hover — so dragged positions are preserved.
  const idsKey = graph.nodes.map((n) => n.id).sort().join(",");
  useEffect(() => {
    if (graph.nodes.length === 0) {
      setPositions({});
      return;
    }
    const el = wrapRef.current;
    const w = el?.clientWidth || 900;
    const h = el?.clientHeight || 600;
    const p = layout(graph, w, h);
    setPositions(p);
    setView(fitView(p, w, h)); // always start centred in the middle
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  const toGraph = (clientX: number, clientY: number): Pos => {
    const rect = svgRef.current!.getBoundingClientRect();
    const v = viewRef.current;
    return { x: (clientX - rect.left - v.x) / v.scale, y: (clientY - rect.top - v.y) / v.scale };
  };

  const onPointerDown = (e: React.PointerEvent) => {
    const hit = (e.target as Element).closest?.("[data-node]") as Element | null;
    svgRef.current?.setPointerCapture(e.pointerId);
    if (hit) {
      const id = hit.getAttribute("data-node")!;
      const g = toGraph(e.clientX, e.clientY);
      const p = positions[id] ?? { x: g.x, y: g.y };
      drag.current = {
        kind: "node",
        id,
        offX: g.x - p.x,
        offY: g.y - p.y,
        startX: e.clientX,
        startY: e.clientY,
        panX: 0,
        panY: 0,
        moved: false,
      };
    } else {
      drag.current = {
        kind: "pan",
        offX: 0,
        offY: 0,
        startX: e.clientX,
        startY: e.clientY,
        panX: view.x,
        panY: view.y,
        moved: false,
      };
    }
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d.kind) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    if (!d.moved && Math.hypot(dx, dy) > DRAG_THRESHOLD) d.moved = true;
    if (d.kind === "node" && d.id) {
      const g = toGraph(e.clientX, e.clientY);
      setPositions((prev) => ({ ...prev, [d.id!]: { x: g.x - d.offX, y: g.y - d.offY } }));
    } else if (d.kind === "pan") {
      setView((v) => ({ ...v, x: d.panX + dx, y: d.panY + dy }));
    }
  };

  const onPointerUp = (e: React.PointerEvent) => {
    const d = drag.current;
    svgRef.current?.releasePointerCapture(e.pointerId);
    if (d.kind === "node" && d.id && !d.moved) nav(`/page/${d.id}`);
    drag.current.kind = null;
  };

  const onWheel = (e: React.WheelEvent) => {
    const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    const rect = svgRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    setView((v) => {
      const scale = Math.max(0.3, Math.min(3, v.scale * factor));
      return { scale, x: mx - ((mx - v.x) / v.scale) * scale, y: my - ((my - v.y) / v.scale) * scale };
    });
  };

  const reset = () => {
    const el = wrapRef.current;
    const w = el?.clientWidth || 900;
    const h = el?.clientHeight || 600;
    if (graph.nodes.length) {
      const p = layout(graph, w, h);
      setPositions(p);
      setView(fitView(p, w, h));
    } else {
      setView({ x: 0, y: 0, scale: 1 });
    }
  };

  return (
    <div className="graph-wrap" ref={wrapRef}>
      {graph.nodes.length === 0 ? (
        <div className="empty-state">Noch keine Seiten für den Graphen.</div>
      ) : (
        <>
          <div className="graph-hint">Knoten ziehen · Hintergrund ziehen zum Verschieben · scrollen zum Zoomen</div>
          {legend.length > 1 && (
            <div className="graph-legend">
              {legend.map((l) => (
                <span key={l.key} className="graph-legend-item">
                  <span className="graph-legend-dot" style={{ background: l.color }} />
                  {l.label}
                </span>
              ))}
            </div>
          )}
          <button className="btn graph-reset" onClick={reset}>
            Zentrieren
          </button>
          <svg
            ref={svgRef}
            width={size.w}
            height={size.h}
            className="graph-svg"
            onPointerDown={onPointerDown}
            onPointerMove={onPointerMove}
            onPointerUp={onPointerUp}
            onWheel={onWheel}
          >
            <g transform={`translate(${view.x}, ${view.y}) scale(${view.scale})`}>
              {graph.edges.map((edge, i) => {
                const a = positions[edge.source];
                const b = positions[edge.target];
                if (!a || !b) return null;
                const lit = hover && (edge.source === hover || edge.target === hover);
                return (
                  <line
                    key={i}
                    x1={a.x}
                    y1={a.y}
                    x2={b.x}
                    y2={b.y}
                    stroke={lit ? "#2383e2" : edge.kind === "parent" ? "#c7c7c4" : "#dcdcda"}
                    strokeWidth={edge.kind === "parent" ? 1.5 : 1}
                    strokeDasharray={edge.kind === "link" ? "4 3" : undefined}
                  />
                );
              })}
              {graph.nodes.map((node) => {
                const p = positions[node.id];
                if (!p) return null;
                return (
                  <g
                    key={node.id}
                    data-node={node.id}
                    transform={`translate(${p.x}, ${p.y})`}
                    className="graph-node"
                    onPointerEnter={() => setHover(node.id)}
                    onPointerLeave={() => setHover(null)}
                  >
                    <circle
                  r={hover === node.id ? 9 : 7}
                  fill={spaceColor[spaceKey(node)] ?? "#2383e2"}
                  stroke={hover === node.id ? "#37352f" : "none"}
                  strokeWidth={hover === node.id ? 2 : 0}
                />
                    <text x={12} y={4} fontSize={12} fill="#37352f">
                      {node.title || "Ohne Titel"}
                    </text>
                  </g>
                );
              })}
            </g>
          </svg>
        </>
      )}
    </div>
  );
}
