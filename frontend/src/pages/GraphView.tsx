// The Graf over the whole workspace: a live force simulation drawn as SVG,
// pannable, zoomable and draggable.
//
// The simulation runs entirely outside React. Positions live in refs and are
// integrated in a requestAnimationFrame loop; React is only nudged to repaint,
// because putting hundreds of coordinates into state would re-render the whole
// tree sixty times a second. Everything derived from the graph is memoised on
// the set of page ids, so the layout is rebuilt when pages appear or disappear
// but not when one of them is merely renamed.
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Graph, GraphNode, api } from "../api/client";

// Palette used to colour nodes per space. Pages without a space share the last,
// neutral colour.
const SPACE_COLORS = [
  "#2383e2", "#e2662c", "#159a6b", "#a84be0", "#d4356b",
  "#c99700", "#0f9bb0", "#7a52d6", "#5a8f3c", "#d05a2c",
];
const NO_SPACE_COLOR = "#9b9a97";

function spaceKey(node: { spaceId: string | null }): string {
  return node.spaceId ?? "__none__";
}

// spaceInfo returns the distinct space keys in a stable order plus a colour
// lookup, so nodes can be tinted by the folder/space they live in.
function spaceInfo(graph: Graph) {
  const keys: string[] = [];
  for (const n of graph.nodes) {
    const k = spaceKey(n);
    if (!keys.includes(k)) keys.push(k);
  }
  const color: Record<string, string> = {};
  let ci = 0;
  for (const k of keys) {
    color[k] = k === "__none__" ? NO_SPACE_COLOR : SPACE_COLORS[ci++ % SPACE_COLORS.length];
  }
  return { keys, color };
}

// One simulated particle per page.
interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
}

// Physics constants, tuned for a small personal knowledge base. A live
// force simulation (LogSeq-style): nodes repel, edges pull like springs with
// hierarchy edges far stiffer than loose [[wiki-links]], and a gentle gravity
// keeps the whole graph centred in the canvas.
const CHARGE = 13000; // repulsion strength (spreads the graph so labels don't collide)
const SPRING_PARENT = 0.006;
const SPRING_LINK = 0.0022;
const REST_PARENT = 95;
const REST_LINK = 210;
const GRAVITY = 0.014; // mild pull toward centre → contains stray nodes
const SAME_SPACE_PULL = 0.0016; // loose grouping by folder, not rigid anchors
const VELOCITY_DECAY = 0.82;
const ALPHA_DECAY = 0.985;
const ALPHA_MIN = 0.02;
const DRAG_ALPHA = 0.35; // keep the sim warm while interacting

const DRAG_THRESHOLD = 4; // px before a press counts as a drag, not a click

// Label placement.
//
// Every node carries its name, so the names get in each other's way. Nothing
// here moves a node; only the label steps aside, and only into one of a few
// fixed places around its own node. A label that wanders freely is worse than
// one that overlaps: one no longer sees which name belongs to which dot.
const LABEL_SCHRIFT = 12;
const LABEL_HOEHE = 14;
// Rough width per character at that size in a sans face. Measuring properly
// would mean a canvas and one measurement per name per frame; for stepping
// aside, an estimate that errs on the generous side is enough -- too wide only
// means a label steps aside a little too eagerly.
const LABEL_BREITE_JE_ZEICHEN = 6.4;

interface Kasten {
  x: number;
  y: number;
  w: number;
  h: number;
}

function ueberlappen(a: Kasten, b: Kasten): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
}

// The places a label may take, in order of preference: to the right of the node
// first, because that is where one looks for it, then left, then below, then
// above, and finally further below as the last way out.
type Anker = "start" | "end" | "middle";
const LABEL_PLAETZE: { dx: (r: number) => number; dy: (r: number) => number; anker: Anker }[] = [
  { dx: (r) => r + 5, dy: () => 4, anker: "start" },
  { dx: (r) => -(r + 5), dy: () => 4, anker: "end" },
  { dx: () => 0, dy: (r) => r + 14, anker: "middle" },
  { dx: () => 0, dy: (r) => -(r + 8), anker: "middle" },
  { dx: () => 0, dy: (r) => r + 28, anker: "middle" },
];

// kastenFuer is where a label sits once it has taken one of those places.
function kastenFuer(x: number, y: number, breite: number, dx: number, dy: number, anker: Anker): Kasten {
  const links = anker === "start" ? x + dx : anker === "end" ? x + dx - breite : x + dx - breite / 2;
  return { x: links, y: y + dy - LABEL_HOEHE + 3, w: breite, h: LABEL_HOEHE };
}

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const wrapRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [size, setSize] = useState({ w: 900, h: 600 });
  const [view, setView] = useState({ x: 0, y: 0, scale: 1 });
  const [hover, setHover] = useState<string | null>(null);
  const [, forceRender] = useState(0);

  const sizeRef = useRef(size);
  sizeRef.current = size;
  const viewRef = useRef(view);
  viewRef.current = view;

  const { color: spaceColor } = spaceInfo(graph);

  // Derived, stable-per-node-set structures: particle store, degree (for node
  // size), neighbour sets (for hover highlighting) and the legend.
  const idsKey = graph.nodes.map((n) => n.id).sort().join(",");
  const derived = useMemo(() => {
    const degree: Record<string, number> = {};
    const neighbours: Record<string, Set<string>> = {};
    for (const n of graph.nodes) {
      degree[n.id] = 0;
      neighbours[n.id] = new Set();
    }
    for (const e of graph.edges) {
      if (degree[e.source] === undefined || degree[e.target] === undefined) continue;
      degree[e.source]++;
      degree[e.target]++;
      neighbours[e.source].add(e.target);
      neighbours[e.target].add(e.source);
    }
    const legend: { key: string; label: string; color: string }[] = [];
    const seen = new Set<string>();
    for (const n of graph.nodes) {
      const k = spaceKey(n);
      if (seen.has(k)) continue;
      seen.add(k);
      legend.push({ key: k, label: k === "__none__" ? "Kein Space" : n.space || "Space", color: spaceColor[k] });
    }
    return { degree, neighbours, legend };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  const nodeRadius = (id: string) => 6 + Math.min(7, Math.sqrt(derived.degree[id] || 0) * 2.2);

  // The mutable simulation state lives in refs so the animation loop never
  // triggers React re-creation; we only nudge React to repaint each frame.
  const particles = useRef<Record<string, Particle>>({});
  // Where each label currently sits. Recomputed with the simulation, kept in a
  // ref for the same reason the positions are: it changes sixty times a second
  // and must not drag React through it.
  const beschriftung = useRef<Record<string, { dx: number; dy: number; anker: Anker }>>({});
  const alpha = useRef(0);
  const raf = useRef<number | null>(null);
  const fixed = useRef<{ id: string; x: number; y: number } | null>(null);
  const spaceOf = useRef<Record<string, string>>({});

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

  // One simulation step over the current particle set.
  const tick = () => {
    const nodes = graph.nodes;
    const p = particles.current;
    const a = alpha.current;
    const { w, h } = sizeRef.current;

    // Repulsion between every pair.
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const pa = p[nodes[i].id];
        const pb = p[nodes[j].id];
        if (!pa || !pb) continue;
        let dx = pa.x - pb.x;
        let dy = pa.y - pb.y;
        let l = dx * dx + dy * dy;
        if (l < 0.01) {
          dx = (Math.random() - 0.5) * 0.5;
          dy = (Math.random() - 0.5) * 0.5;
          l = dx * dx + dy * dy + 0.01;
        }
        const dist = Math.sqrt(l);
        const rep = (CHARGE * a) / l;
        const ux = dx / dist;
        const uy = dy / dist;
        pa.vx += ux * rep;
        pa.vy += uy * rep;
        pb.vx -= ux * rep;
        pb.vy -= uy * rep;
        // Loose same-space attraction: folders cluster without hard anchors.
        if (spaceOf.current[nodes[i].id] === spaceOf.current[nodes[j].id]) {
          pa.vx -= ux * SAME_SPACE_PULL * dist * a;
          pa.vy -= uy * SAME_SPACE_PULL * dist * a;
          pb.vx += ux * SAME_SPACE_PULL * dist * a;
          pb.vy += uy * SAME_SPACE_PULL * dist * a;
        }
      }
    }

    // Edge springs — hierarchy stiff and short, wiki-links soft and long.
    for (const e of graph.edges) {
      const ps = p[e.source];
      const pt = p[e.target];
      if (!ps || !pt) continue;
      let dx = pt.x - ps.x;
      let dy = pt.y - ps.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const parent = e.kind === "parent";
      const rest = parent ? REST_PARENT : REST_LINK;
      const k = parent ? SPRING_PARENT : SPRING_LINK;
      const spr = k * (dist - rest) * a;
      const ux = dx / dist;
      const uy = dy / dist;
      ps.vx += ux * spr;
      ps.vy += uy * spr;
      pt.vx -= ux * spr;
      pt.vy -= uy * spr;
    }

    // Gravity toward the canvas centre keeps the graph centred.
    for (const n of nodes) {
      const pp = p[n.id];
      if (!pp) continue;
      pp.vx += (w / 2 - pp.x) * GRAVITY * a;
      pp.vy += (h / 2 - pp.y) * GRAVITY * a;
    }

    // Integrate.
    for (const n of nodes) {
      const pp = p[n.id];
      if (!pp) continue;
      const f = fixed.current;
      if (f && f.id === n.id) {
        pp.x = f.x;
        pp.y = f.y;
        pp.vx = 0;
        pp.vy = 0;
        continue;
      }
      pp.vx *= VELOCITY_DECAY;
      pp.vy *= VELOCITY_DECAY;
      pp.x += pp.vx;
      pp.y += pp.vy;
    }

    // Keep the whole graph centred: shift every node so their centroid sits at
    // the canvas centre. Skipped while dragging so the grabbed node stays put.
    if (!fixed.current) {
      let cx = 0;
      let cy = 0;
      for (const n of nodes) {
        const pp = p[n.id];
        if (pp) {
          cx += pp.x;
          cy += pp.y;
        }
      }
      const cnt = nodes.length || 1;
      const shiftX = w / 2 - cx / cnt;
      const shiftY = h / 2 - cy / cnt;
      for (const n of nodes) {
        const pp = p[n.id];
        if (pp) {
          pp.x += shiftX;
          pp.y += shiftY;
        }
      }
    }

    beschriftungenVerteilen();
  };

  // Give every label a place where it covers as little as possible.
  //
  // Greedy and in one pass: the nodes with the most edges come first and keep
  // the good place to the right, everything else steps aside around them. A
  // proper optimum would be worth neither the code nor the milliseconds -- it is
  // recomputed on every frame anyway, and one frame later the picture has moved
  // on regardless.
  //
  // The circles count as occupied as well: a name lying across a dot is no
  // better than one lying across another name.
  const beschriftungenVerteilen = () => {
    const p = particles.current;
    const reihe = [...graph.nodes].sort(
      (a, b) => (derived.degree[b.id] || 0) - (derived.degree[a.id] || 0),
    );
    const belegt: Kasten[] = [];
    for (const n of reihe) {
      const pp = p[n.id];
      if (!pp) continue;
      const r = nodeRadius(n.id);
      belegt.push({ x: pp.x - r, y: pp.y - r, w: r * 2, h: r * 2 });
    }
    const gewaehlt: Record<string, { dx: number; dy: number; anker: Anker }> = {};
    for (const n of reihe) {
      const pp = p[n.id];
      if (!pp) continue;
      const r = nodeRadius(n.id);
      const breite = (n.title || "Ohne Titel").length * LABEL_BREITE_JE_ZEICHEN;
      let platz = LABEL_PLAETZE[0];
      let kasten = kastenFuer(pp.x, pp.y, breite, platz.dx(r), platz.dy(r), platz.anker);
      for (const k of LABEL_PLAETZE) {
        const versuch = kastenFuer(pp.x, pp.y, breite, k.dx(r), k.dy(r), k.anker);
        if (!belegt.some((b) => ueberlappen(b, versuch))) {
          platz = k;
          kasten = versuch;
          break;
        }
      }
      // Taken either way: even the last resort blocks the spot for whoever comes
      // after, otherwise two labels that both had to give up would land on top
      // of one another.
      belegt.push(kasten);
      gewaehlt[n.id] = { dx: platz.dx(r), dy: platz.dy(r), anker: platz.anker };
    }
    beschriftung.current = gewaehlt;
  };

  const loop = () => {
    tick();
    alpha.current *= ALPHA_DECAY;
    forceRender((v) => v + 1);
    if (alpha.current > ALPHA_MIN || fixed.current) {
      raf.current = requestAnimationFrame(loop);
    } else {
      raf.current = null;
    }
  };

  const reheat = (target = 1) => {
    alpha.current = Math.max(alpha.current, target);
    if (raf.current == null) raf.current = requestAnimationFrame(loop);
  };

  // (Re)seed particles whenever the set of pages changes; then run the sim.
  useEffect(() => {
    const { w, h } = sizeRef.current;
    const next: Record<string, Particle> = {};
    const so: Record<string, string> = {};
    const n = graph.nodes.length || 1;
    graph.nodes.forEach((node, i) => {
      so[node.id] = spaceKey(node);
      const prev = particles.current[node.id];
      if (prev) {
        next[node.id] = prev; // keep positions across data refreshes
      } else {
        const ang = (i / n) * Math.PI * 2;
        next[node.id] = {
          x: w / 2 + Math.cos(ang) * (Math.min(w, h) / 3.5),
          y: h / 2 + Math.sin(ang) * (Math.min(w, h) / 3.5),
          vx: 0,
          vy: 0,
        };
      }
    });
    particles.current = next;
    spaceOf.current = so;
    if (graph.nodes.length) reheat(1);
    return () => {
      if (raf.current != null) cancelAnimationFrame(raf.current);
      raf.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  // Screen coordinates to simulation coordinates, undoing the current pan and
  // zoom. Needed by every pointer handler, since the physics knows nothing about
  // the viewport.
  const toGraph = (clientX: number, clientY: number) => {
    const rect = svgRef.current!.getBoundingClientRect();
    const v = viewRef.current;
    return { x: (clientX - rect.left - v.x) / v.scale, y: (clientY - rect.top - v.y) / v.scale };
  };

  // A press either grabs a node or starts a pan, decided by whether it landed on
  // one. The pointer is captured so a fast drag that leaves the canvas keeps
  // being tracked. The grab offset is remembered so the node does not jump to
  // centre itself under the cursor.
  const onPointerDown = (e: React.PointerEvent) => {
    const hit = (e.target as Element).closest?.("[data-node]") as Element | null;
    svgRef.current?.setPointerCapture(e.pointerId);
    if (hit) {
      const id = hit.getAttribute("data-node")!;
      const g = toGraph(e.clientX, e.clientY);
      const p = particles.current[id] ?? { x: g.x, y: g.y };
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
      fixed.current = { id, x: p.x, y: p.y };
      reheat(DRAG_ALPHA);
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

  // A dragged node is pinned to the cursor and the simulation is reheated, so
  // its neighbours follow along instead of freezing in place. The moved flag
  // separates a drag from a click; see onPointerUp.
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d.kind) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    if (!d.moved && Math.hypot(dx, dy) > DRAG_THRESHOLD) d.moved = true;
    if (d.kind === "node" && d.id) {
      const g = toGraph(e.clientX, e.clientY);
      fixed.current = { id: d.id, x: g.x - d.offX, y: g.y - d.offY };
      reheat(DRAG_ALPHA);
    } else if (d.kind === "pan") {
      setView((v) => ({ ...v, x: d.panX + dx, y: d.panY + dy }));
    }
  };

  // Release. A press that never exceeded the threshold counts as a click and
  // opens the page, so dragging a node does not navigate away by accident.
  const onPointerUp = (e: React.PointerEvent) => {
    const d = drag.current;
    svgRef.current?.releasePointerCapture(e.pointerId);
    if (d.kind === "node" && d.id && !d.moved) nav(`/page/${d.id}`);
    fixed.current = null; // release: node settles back into the sim
    reheat(0.15);
    drag.current.kind = null;
  };

  // Zoom toward the pointer rather than the canvas centre: the point under the
  // cursor has to stay put, which is what the coordinate correction does. The
  // scale is clamped so the graph can neither vanish nor fill the screen with a
  // single node.
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

  // Recentre: reset pan/zoom and give the sim a fresh kick so it re-settles in
  // the middle of the canvas.
  const recenter = () => {
    setView({ x: 0, y: 0, scale: 1 });
    if (graph.nodes.length) reheat(1);
  };

  const p = particles.current;
  const nodeOpacity = (id: string) => {
    if (!hover) return 1;
    if (id === hover || derived.neighbours[hover]?.has(id)) return 1;
    return 0.15;
  };

  // Every node carries its name, at every zoom level. Hiding them until one
  // zoomed in kept the picture tidy and made it useless: a graph of unnamed dots
  // says nothing about what is connected to what, and to read it one had to
  // hover over every dot one at a time.
  //
  // What was tidy about it is kept in another way: the labels fade with their
  // node, so hovering still lifts one neighbourhood out of the rest.

  return (
    <div className="graph-wrap" ref={wrapRef}>
      {graph.nodes.length === 0 ? (
        <div className="empty-state">Noch keine Seiten für den Graf.</div>
      ) : (
        <>
          <div className="graph-hint">Knoten ziehen · Hintergrund ziehen · scrollen zum Zoomen</div>
          {derived.legend.length > 1 && (
            <div className="graph-legend">
              {derived.legend.map((l) => (
                <span key={l.key} className="graph-legend-item">
                  <span className="graph-legend-dot" style={{ background: l.color }} />
                  {l.label}
                </span>
              ))}
            </div>
          )}
          <button className="btn graph-reset" onClick={recenter}>
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
                const a = p[edge.source];
                const b = p[edge.target];
                if (!a || !b) return null;
                const lit = hover && (edge.source === hover || edge.target === hover);
                const dim = hover && !lit;
                return (
                  <line
                    key={i}
                    x1={a.x}
                    y1={a.y}
                    x2={b.x}
                    y2={b.y}
                    stroke={lit ? "#2383e2" : edge.kind === "parent" ? "#c7c7c4" : "#dcdcda"}
                    strokeWidth={edge.kind === "parent" ? 1.6 : 1}
                    strokeDasharray={edge.kind === "link" ? "4 3" : undefined}
                    opacity={dim ? 0.12 : 1}
                  />
                );
              })}
              {graph.nodes.map((node: GraphNode) => {
                const pp = p[node.id];
                if (!pp) return null;
                const r = nodeRadius(node.id);
                const op = nodeOpacity(node.id);
                const platz = beschriftung.current[node.id] ?? { dx: r + 5, dy: 4, anker: "start" as Anker };
                return (
                  <g
                    key={node.id}
                    data-node={node.id}
                    transform={`translate(${pp.x}, ${pp.y})`}
                    className="graph-node"
                    opacity={op}
                    onPointerEnter={() => setHover(node.id)}
                    onPointerLeave={() => setHover(null)}
                  >
                    <circle
                      r={hover === node.id ? r + 2 : r}
                      fill={spaceColor[spaceKey(node)] ?? "#2383e2"}
                      stroke={hover === node.id ? "#37352f" : "#ffffff"}
                      strokeWidth={hover === node.id ? 2 : 1}
                    />
                    {/* The white edge around the letters is what keeps a name
                        readable where it crosses an edge -- stepping aside
                        settles the labels among themselves, not the lines. */}
                    <text
                      x={platz.dx}
                      y={platz.dy}
                      textAnchor={platz.anker}
                      fontSize={LABEL_SCHRIFT}
                      fill="#37352f"
                      stroke="#ffffff"
                      strokeWidth={3.5}
                      paintOrder="stroke"
                      strokeLinejoin="round"
                    >
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
