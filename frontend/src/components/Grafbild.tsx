// Grafbild: a live force simulation drawn as SVG, supporting pan, zoom, and
// drag.
//
// This component lives here instead of the graph view because it is reused in
// two places: a full workspace view and a small inline graph under a page.
// The simulation runs entirely outside React. Positions are stored in refs and
// advanced in a requestAnimationFrame loop; React is only triggered to redraw
// because putting hundreds of coordinates into state would rerender the tree
// dozens of times per second.
import { useEffect, useMemo, useRef, useState } from "react";

import { Graph, GraphNode } from "../api/client";
import { farbeFuerAblage } from "../ablagefarben";

function ablageSchluessel(node: { spaceId: string | null }): string {
  return node.spaceId ?? "__none__";
}

// `ablageFarben` returns a color for each encountered space.
//
// The color is derived from the space identifier, not its index in a list
// (see src/ablagefarben.ts). That ensures a space has the same color in the
// graph and in the sidebar, even when filtering or pages change.
function ablageFarben(graph: Graph, eigene?: Record<string, string>) {
  const farbe: Record<string, string> = {};
  for (const n of graph.nodes) {
    const k = ablageSchluessel(n);
    if (!(k in farbe)) farbe[k] = farbeFuerAblage(k === "__none__" ? null : k, eigene?.[k]);
  }
  return farbe;
}

// One simulated particle per page.
interface Teilchen {
  x: number;
  y: number;
  vx: number;
  vy: number;
}

// Physics parameters. Nodes repel each other, edges act like springs
// (parent links stiffer than soft references), and a mild centering gravity
// keeps the layout overall centered.
export interface Physik {
  abstossung: number;
  federEltern: number;
  federVerweis: number;
  ruheEltern: number;
  ruheVerweis: number;
  schwerkraft: number;
  gleicheAblage: number;
}

// Physics constants for the large workspace view.
export const PHYSIK_GROSS: Physik = {
  abstossung: 13000,
  federEltern: 0.006,
  federVerweis: 0.0022,
  ruheEltern: 95,
  ruheVerweis: 210,
  schwerkraft: 0.014,
  gleicheAblage: 0.0016,
};

// Physics constants for the small inline graph: fewer nodes, less space. The
// large-view constants would cause a few nodes to fly to the edges here.
export const PHYSIK_KLEIN: Physik = {
  abstossung: 2600,
  federEltern: 0.010,
  federVerweis: 0.006,
  ruheEltern: 62,
  ruheVerweis: 105,
  schwerkraft: 0.035,
  gleicheAblage: 0,
};

const GESCHWINDIGKEIT_DAEMPFUNG = 0.82;
const ALPHA_ABFALL = 0.985;
const ALPHA_MIN = 0.02;
const ZUG_ALPHA = 0.35; // keeps the simulation warm while the user drags
const ZUG_SCHWELLE = 4; // pixels threshold above which a press is a drag, not a click

// Labels.
//
// Each node shows its title which leads to overlaps. Labels do not move nodes;
// they shift to one of a few fixed positions around their node. A freely
// floating label causes worse ambiguity than a fixed offset.
const LABEL_SCHRIFT = 12;
const LABEL_HOEHE = 14;
// Approximate width per character at this font size. Measuring precisely
// would require a canvas and per-string measurement; an overestimate is fine
// because it only makes labels shift a bit more.
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

// Candidate anchor positions for a label, ordered by preference: right, left,
// below, above, then further below.
type Anker = "start" | "end" | "middle";
const LABEL_PLAETZE: { dx: (r: number) => number; dy: (r: number) => number; anker: Anker }[] = [
  { dx: (r) => r + 5, dy: () => 4, anker: "start" },
  { dx: (r) => -(r + 5), dy: () => 4, anker: "end" },
  { dx: () => 0, dy: (r) => r + 14, anker: "middle" },
  { dx: () => 0, dy: (r) => -(r + 8), anker: "middle" },
  { dx: () => 0, dy: (r) => r + 28, anker: "middle" },
];

// `kastenFuer` computes the bounding box of a label at a chosen anchor.
function kastenFuer(x: number, y: number, breite: number, dx: number, dy: number, anker: Anker): Kasten {
  const links = anker === "start" ? x + dx : anker === "end" ? x + dx - breite : x + dx - breite / 2;
  return { x: links, y: y + dy - LABEL_HOEHE + 3, w: breite, h: LABEL_HOEHE };
}

interface Props {
  graph: Graph;
  /** A click on a node that was not a drag. */
  onOeffnen: (id: string) => void;
  /** The page to emphasize and keep near the center (used by the small graph). */
  mitte?: string;
  physik?: Physik;
  /** Fixed height in pixels; if omitted the image fills its container. */
  hoehe?: number;
  hinweis?: string;
  legende?: boolean;
  zentrieren?: boolean;
  /** User-selected colors per space; key is the spaceId. */
  eigeneFarben?: Record<string, string>;
    /** Callback to change a space color. If provided, the legend shows a color
      picker; otherwise the legend is read-only. */
  onFarbe?: (spaceId: string, farbe: string) => void;
  /**
   * Welche Ablagen dieses Konto färben darf. Ohne die Angabe darf es alle, die
   * in der Legende stehen.
   *
   * Nötig, weil der Dienst nur den Eigentümer und eine Verwaltung durchlässt:
   * ein Wähler an einer fremden Ablage nähme die Farbe an, der Dienst wiese sie
   * ab, und sie spränge kommentarlos zurück. Ein Regler, der zurückschnappt,
   * sieht kaputt aus -- also gibt es ihn dort gar nicht erst.
   */
  faerbbar?: Set<string>;
    /** Whether the mouse wheel zooms. In a page view only with Ctrl to avoid
      interfering with normal scrolling. */
  radZoom?: "immer" | "mit-strg";
}

export default function Grafbild({
  graph,
  onOeffnen,
  mitte,
  physik = PHYSIK_GROSS,
  hoehe,
  hinweis,
  legende = false,
  zentrieren = false,
  eigeneFarben,
  onFarbe,
  faerbbar,
  radZoom = "immer",
}: Props) {
  const rahmenRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [groesse, setGroesse] = useState({ w: 900, h: hoehe ?? 600 });
  const [blick, setBlick] = useState({ x: 0, y: 0, scale: 1 });
  const [zeiger, setZeiger] = useState<string | null>(null);
  const [, neuZeichnen] = useState(0);

  const groesseRef = useRef(groesse);
  groesseRef.current = groesse;
  const blickRef = useRef(blick);
  blickRef.current = blick;
  const physikRef = useRef(physik);
  physikRef.current = physik;
  const mitteRef = useRef(mitte);
  mitteRef.current = mitte;

  const farbe = ablageFarben(graph, eigeneFarben);

  // Derived values that depend only on the node set: degree (for size),
  // neighbor sets (for highlighting), and the legend entries.
  const idsKey = graph.nodes.map((n) => n.id).sort().join(",");
  const abgeleitet = useMemo(() => {
    const grad: Record<string, number> = {};
    const nachbarn: Record<string, Set<string>> = {};
    for (const n of graph.nodes) {
      grad[n.id] = 0;
      nachbarn[n.id] = new Set();
    }
    for (const e of graph.edges) {
      if (grad[e.source] === undefined || grad[e.target] === undefined) continue;
      grad[e.source]++;
      grad[e.target]++;
      nachbarn[e.source].add(e.target);
      nachbarn[e.target].add(e.source);
    }
    const eintraege: { key: string; label: string; color: string }[] = [];
    const gesehen = new Set<string>();
    for (const n of graph.nodes) {
      const k = ablageSchluessel(n);
      if (gesehen.has(k)) continue;
      gesehen.add(k);
      eintraege.push({ key: k, label: k === "__none__" ? "Keine Ablage" : n.space || "Ablage", color: farbe[k] });
    }
    return { grad, nachbarn, legende: eintraege };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  // The center node is slightly larger: it is the reason the graph exists.
  const radius = (id: string) =>
    (id === mitte ? 3 : 0) + 6 + Math.min(7, Math.sqrt(abgeleitet.grad[id] || 0) * 2.2);

  const teilchen = useRef<Record<string, Teilchen>>({});
  const beschriftung = useRef<Record<string, { dx: number; dy: number; anker: Anker }>>({});
  const alpha = useRef(0);
  const raf = useRef<number | null>(null);
  const gehalten = useRef<{ id: string; x: number; y: number } | null>(null);
  const ablageVon = useRef<Record<string, string>>({});

  const zug = useRef<{
    art: "knoten" | "flaeche" | null;
    id?: string;
    offX: number;
    offY: number;
    startX: number;
    startY: number;
    panX: number;
    panY: number;
    bewegt: boolean;
  }>({ art: null, offX: 0, offY: 0, startX: 0, startY: 0, panX: 0, panY: 0, bewegt: false });

  useEffect(() => {
    const el = rahmenRef.current;
    if (!el) return;
    const messen = () => setGroesse({ w: el.clientWidth, h: hoehe ?? el.clientHeight });
    messen();
    const ro = new ResizeObserver(messen);
    ro.observe(el);
    return () => ro.disconnect();
  }, [hoehe]);

  // One simulation step over the current particle set.
  const schritt = () => {
    const nodes = graph.nodes;
    const p = teilchen.current;
    const a = alpha.current;
    const { w, h } = groesseRef.current;
    const ph = physikRef.current;

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
        const rep = (ph.abstossung * a) / l;
        const ux = dx / dist;
        const uy = dy / dist;
        pa.vx += ux * rep;
        pa.vy += uy * rep;
        pb.vx -= ux * rep;
        pb.vy -= uy * rep;
        // Mild attraction within the same space: folders cluster without
        // being pinned.
        if (ph.gleicheAblage && ablageVon.current[nodes[i].id] === ablageVon.current[nodes[j].id]) {
          pa.vx -= ux * ph.gleicheAblage * dist * a;
          pa.vy -= uy * ph.gleicheAblage * dist * a;
          pb.vx += ux * ph.gleicheAblage * dist * a;
          pb.vy += uy * ph.gleicheAblage * dist * a;
        }
      }
    }

    // Spring forces along edges: parents act as short stiff springs, refs as
    // longer softer springs.
    for (const e of graph.edges) {
      const ps = p[e.source];
      const pt = p[e.target];
      if (!ps || !pt) continue;
      const dx = pt.x - ps.x;
      const dy = pt.y - ps.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const eltern = e.kind === "parent";
      const ruhe = eltern ? ph.ruheEltern : ph.ruheVerweis;
      const k = eltern ? ph.federEltern : ph.federVerweis;
      const spr = k * (dist - ruhe) * a;
      const ux = dx / dist;
      const uy = dy / dist;
      ps.vx += ux * spr;
      ps.vy += uy * spr;
      pt.vx -= ux * spr;
      pt.vy -= uy * spr;
    }

    // Gravity toward the center. The emphasized node is held more strongly so
    // that the focused page remains near the center in the small graph.
    for (const n of nodes) {
      const pp = p[n.id];
      if (!pp) continue;
      const stark = n.id === mitteRef.current ? 6 : 1;
      pp.vx += (w / 2 - pp.x) * ph.schwerkraft * stark * a;
      pp.vy += (h / 2 - pp.y) * ph.schwerkraft * stark * a;
    }

    // Integrate velocities to advance positions.
    for (const n of nodes) {
      const pp = p[n.id];
      if (!pp) continue;
      const f = gehalten.current;
      if (f && f.id === n.id) {
        pp.x = f.x;
        pp.y = f.y;
        pp.vx = 0;
        pp.vy = 0;
        continue;
      }
      pp.vx *= GESCHWINDIGKEIT_DAEMPFUNG;
      pp.vy *= GESCHWINDIGKEIT_DAEMPFUNG;
      pp.x += pp.vx;
      pp.y += pp.vy;
    }

    // Keep the whole layout centered by translating nodes so their centroid is
    // at the center. Skip while dragging so the grabbed node remains under the
    // cursor.
    if (!gehalten.current) {
      let cx = 0;
      let cy = 0;
      for (const n of nodes) {
        const pp = p[n.id];
        if (pp) {
          cx += pp.x;
          cy += pp.y;
        }
      }
      const anzahl = nodes.length || 1;
      const schiebeX = w / 2 - cx / anzahl;
      const schiebeY = h / 2 - cy / anzahl;
      for (const n of nodes) {
        const pp = p[n.id];
        if (pp) {
          pp.x += schiebeX;
          pp.y += schiebeY;
        }
      }
    }

    beschriftungenVerteilen();
  };

  // Assign each label a position that minimizes overlap.
  //
  // Greedy single-pass algorithm: nodes with highest degree get preference and
  // keep the best spot on the right; others yield. A true optimum is not worth
  // the CPU time because layout changes frequently.
  const beschriftungenVerteilen = () => {
    const p = teilchen.current;
    const reihe = [...graph.nodes].sort(
      (a, b) => (abgeleitet.grad[b.id] || 0) - (abgeleitet.grad[a.id] || 0),
    );
    const belegt: Kasten[] = [];
    for (const n of reihe) {
      const pp = p[n.id];
      if (!pp) continue;
      const r = radius(n.id);
      belegt.push({ x: pp.x - r, y: pp.y - r, w: r * 2, h: r * 2 });
    }
    const gewaehlt: Record<string, { dx: number; dy: number; anker: Anker }> = {};
    for (const n of reihe) {
      const pp = p[n.id];
      if (!pp) continue;
      const r = radius(n.id);
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
      // Either way it occupies space: even the last fallback reserves the
      // spot for the next label, otherwise two labels that both gave up would
      // end up overlapping.
      belegt.push(kasten);
      gewaehlt[n.id] = { dx: platz.dx(r), dy: platz.dy(r), anker: platz.anker };
    }
    beschriftung.current = gewaehlt;
  };

  const schleife = () => {
    schritt();
    alpha.current *= ALPHA_ABFALL;
    neuZeichnen((v) => v + 1);
    if (alpha.current > ALPHA_MIN || gehalten.current) {
      raf.current = requestAnimationFrame(schleife);
    } else {
      raf.current = null;
    }
  };

  const anheizen = (ziel = 1) => {
    alpha.current = Math.max(alpha.current, ziel);
    if (raf.current == null) raf.current = requestAnimationFrame(schleife);
  };

  // Reinitialize particles whenever the node set changes, then run the
  // simulation. Known nodes retain their positions across data updates.
  useEffect(() => {
    const { w, h } = groesseRef.current;
    const naechste: Record<string, Teilchen> = {};
    const ablagen: Record<string, string> = {};
    const n = graph.nodes.length || 1;
    graph.nodes.forEach((node, i) => {
      ablagen[node.id] = ablageSchluessel(node);
      const vorher = teilchen.current[node.id];
      if (vorher) {
        naechste[node.id] = vorher;
      } else if (node.id === mitte) {
        naechste[node.id] = { x: w / 2, y: h / 2, vx: 0, vy: 0 };
      } else {
        const ang = (i / n) * Math.PI * 2;
        naechste[node.id] = {
          x: w / 2 + Math.cos(ang) * (Math.min(w, h) / 3.5),
          y: h / 2 + Math.sin(ang) * (Math.min(w, h) / 3.5),
          vx: 0,
          vy: 0,
        };
      }
    });
    teilchen.current = naechste;
    ablageVon.current = ablagen;
    if (graph.nodes.length) anheizen(1);
    return () => {
      if (raf.current != null) cancelAnimationFrame(raf.current);
      raf.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  // Convert screen coordinates to simulation coordinates, accounting for
  // pan and zoom. Every pointer handler needs this because the physics loop
  // is unaware of the current viewport.
  const zuGraf = (clientX: number, clientY: number) => {
    const rect = svgRef.current!.getBoundingClientRect();
    const v = blickRef.current;
    return { x: (clientX - rect.left - v.x) / v.scale, y: (clientY - rect.top - v.y) / v.scale };
  };

  // A press either grabs a node or pans the surface depending on what lies
  // beneath the pointer. Capture the pointer so a quick drag that leaves the
  // SVG remains tracked.
  const zeigerAb = (e: React.PointerEvent) => {
    const treffer = (e.target as Element).closest?.("[data-node]") as Element | null;
    svgRef.current?.setPointerCapture(e.pointerId);
    if (treffer) {
      const id = treffer.getAttribute("data-node")!;
      const g = zuGraf(e.clientX, e.clientY);
      const p = teilchen.current[id] ?? { x: g.x, y: g.y };
      zug.current = {
        art: "knoten",
        id,
        offX: g.x - p.x,
        offY: g.y - p.y,
        startX: e.clientX,
        startY: e.clientY,
        panX: 0,
        panY: 0,
        bewegt: false,
      };
      gehalten.current = { id, x: p.x, y: p.y };
      anheizen(ZUG_ALPHA);
    } else {
      zug.current = {
        art: "flaeche",
        offX: 0,
        offY: 0,
        startX: e.clientX,
        startY: e.clientY,
        panX: blick.x,
        panY: blick.y,
        bewegt: false,
      };
    }
  };

  // A dragged node stays attached to the pointer; keep the simulation warm
  // so its neighbors move along instead of freezing.
  const zeigerBewegt = (e: React.PointerEvent) => {
    const d = zug.current;
    if (!d.art) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    if (!d.bewegt && Math.hypot(dx, dy) > ZUG_SCHWELLE) d.bewegt = true;
    if (d.art === "knoten" && d.id) {
      const g = zuGraf(e.clientX, e.clientY);
      gehalten.current = { id: d.id, x: g.x - d.offX, y: g.y - d.offY };
      anheizen(ZUG_ALPHA);
    } else if (d.art === "flaeche") {
      setBlick((v) => ({ ...v, x: d.panX + dx, y: d.panY + dy }));
    }
  };

  // On release: a press that never exceeded the drag threshold is treated as
  // a click and opens the page — so a dragged node does not accidentally
  // navigate elsewhere.
  const zeigerAuf = (e: React.PointerEvent) => {
    const d = zug.current;
    svgRef.current?.releasePointerCapture(e.pointerId);
    if (d.art === "knoten" && d.id && !d.bewegt) onOeffnen(d.id);
    gehalten.current = null;
    anheizen(0.15);
    zug.current.art = null;
  };

  // Zoom toward the pointer, not the center: the point under the pointer
  // should remain fixed, adjust the transform accordingly. Limit the scale so
  // the graph neither vanishes nor becomes a single node filling the window.
  const amRad = (e: React.WheelEvent) => {
    if (radZoom === "mit-strg" && !e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    const faktor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    const rect = svgRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    setBlick((v) => {
      const scale = Math.max(0.3, Math.min(3, v.scale * faktor));
      return { scale, x: mx - ((mx - v.x) / v.scale) * scale, y: my - ((my - v.y) / v.scale) * scale };
    });
  };

  // Reset view: restore the default viewport and nudge the simulation so it
  // recenters.
  const zurueck = () => {
    setBlick({ x: 0, y: 0, scale: 1 });
    if (graph.nodes.length) anheizen(1);
  };

  const p = teilchen.current;
  const deckkraft = (id: string) => {
    if (!zeiger) return 1;
    if (id === zeiger || abgeleitet.nachbarn[zeiger]?.has(id)) return 1;
    return 0.15;
  };

  // Every node shows its name at all zoom levels. Hiding labels until zoom
  // kept the image tidy but made it useless: a cloud of unlabeled points
  // does not convey what is related to what.
  return (
    <div className="graph-wrap" ref={rahmenRef} style={hoehe ? { height: hoehe } : undefined}>
      {hinweis && <div className="graph-hint">{hinweis}</div>}
      {/* Eine Legende aus einem einzigen Eintrag sagt nichts, was der Graf
          nicht schon zeigt -- ausser wenn an ihrem Punkt die Farbe gewaehlt
          wird. Dann ist sie kein Schild mehr, sondern der Bedienplatz, und
          fehlte sie, koennte eine Ablage in einem Arbeitsbereich mit nur
          einer Ablage ihre Farbe nirgends bekommen. */}
      {legende && abgeleitet.legende.length > (onFarbe ? 0 : 1) && (
        <div className="graph-legend">
          {abgeleitet.legende.map((l) => (
            <span key={l.key} className="graph-legend-item">
                {/* The dot itself is the color picker: click it, choose a color,
                  done. A separate button would be an extra control for the
                  same action, and the dot is precisely what the user intends
                  to change. For "No space" only the dot is shown — there is
                  no space to attach a color to. */}
              {onFarbe && l.key !== "__none__" && (!faerbbar || faerbbar.has(l.key)) ? (
                <label className="graph-legend-dot-wahl" title="Farbe dieser Ablage">
                  <span className="graph-legend-dot" style={{ background: farbe[l.key] }} />
                  <input
                    type="color"
                    value={farbe[l.key]}
                    onChange={(e) => onFarbe(l.key, e.target.value)}
                  />
                </label>
              ) : (
                <span className="graph-legend-dot" style={{ background: farbe[l.key] }} />
              )}
              {l.label}
            </span>
          ))}
        </div>
      )}
      {zentrieren && (
        <button className="btn graph-reset" onClick={zurueck}>
          Zentrieren
        </button>
      )}
      <svg
        ref={svgRef}
        width={groesse.w}
        height={groesse.h}
        className="graph-svg"
        onPointerDown={zeigerAb}
        onPointerMove={zeigerBewegt}
        onPointerUp={zeigerAuf}
        onWheel={amRad}
      >
        <g transform={`translate(${blick.x}, ${blick.y}) scale(${blick.scale})`}>
          {graph.edges.map((kante, i) => {
            const a = p[kante.source];
            const b = p[kante.target];
            if (!a || !b) return null;
            const hell = zeiger && (kante.source === zeiger || kante.target === zeiger);
            const blass = zeiger && !hell;
            return (
              <line
                key={i}
                x1={a.x}
                y1={a.y}
                x2={b.x}
                y2={b.y}
                // Colors live in the stylesheet, not as attributes: if set here
                // they'd be tied to a base tone and could make light lines sit
                // on light text in dark themes.
                className={
                  "graph-kante" +
                  (kante.kind === "parent" ? " eltern" : "") +
                  (hell ? " betont" : "")
                }
                strokeWidth={kante.kind === "parent" ? 1.6 : 1}
                strokeDasharray={kante.kind === "link" ? "4 3" : undefined}
                opacity={blass ? 0.12 : 1}
              />
            );
          })}
          {graph.nodes.map((node: GraphNode) => {
            const pp = p[node.id];
            if (!pp) return null;
            const r = radius(node.id);
            const platz = beschriftung.current[node.id] ?? { dx: r + 5, dy: 4, anker: "start" as Anker };
            const betont = zeiger === node.id || node.id === mitte;
            return (
              <g
                key={node.id}
                data-node={node.id}
                transform={`translate(${pp.x}, ${pp.y})`}
                className="graph-node"
                opacity={deckkraft(node.id)}
                onPointerEnter={() => setZeiger(node.id)}
                onPointerLeave={() => setZeiger(null)}
              >
                <circle
                  r={zeiger === node.id ? r + 2 : r}
                  fill={farbe[ablageSchluessel(node)] ?? "#2383e2"}
                  // The stroke color should match the page background so nodes
                  // stand out from the lines behind them—light on light, dark on dark.
                  className={"graph-knoten" + (betont ? " betont" : "")}
                  strokeWidth={betont ? 2 : 1}
                />
                {/* The outline around letters keeps a name readable where it
                  crosses a line — overlap avoidance manages labels with
                  respect to each other, not the edges. */}
                <text
                  x={platz.dx}
                  y={platz.dy}
                  textAnchor={platz.anker}
                  fontSize={LABEL_SCHRIFT}
                  className="graph-name"
                  fontWeight={node.id === mitte ? 600 : undefined}
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
    </div>
  );
}
