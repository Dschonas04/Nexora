// Grafbild: a live force simulation drawn as SVG, in the manner of Obsidian's
// graph view: plain dots, names under them that fade out when zooming away,
// hovering lights up a page and its neighbours and lets the rest recede, and a
// settings panel with filters, display and forces.
//
// This component lives here instead of the graph view because it is reused in
// two places: a full workspace view and a small inline graph under a page.
// The simulation runs entirely outside React. Positions are stored in refs and
// advanced in a requestAnimationFrame loop; React is only triggered to redraw
// because putting hundreds of coordinates into state would rerender the tree
// dozens of times per second.
import { useEffect, useId, useMemo, useRef, useState } from "react";

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

// Physics parameters, modelled on d3-force:
//
// - Repulsion falls off with 1/distance (not 1/distance²) and stops at a
//   range. 1/d² clumped the middle and flung loose nodes far out; the cut-off
//   stops distant clusters from pushing each other off the screen.
// - Edges are springs whose stiffness is divided by the smaller degree of
//   their ends, and whose pull is shared by degree: a hub with twenty pages
//   does not get yanked around by each of them, a leaf follows its hub.
// - Collision keeps circles apart independent of temperature, so nodes never
//   end up on top of each other, even after the layout has cooled.
export interface Physik {
  /** Strength of the mutual repulsion (force ≈ abstossung / distance). */
  abstossung: number;
  /** Distance beyond which nodes no longer repel each other. */
  reichweite: number;
  /** Spring strength factor for parent links and for references. */
  federEltern: number;
  federVerweis: number;
  /** Rest length of the springs, measured between the circle edges. */
  ruheEltern: number;
  ruheVerweis: number;
  /** Pull toward the middle; keeps unconnected pages in view. */
  schwerkraft: number;
  /** Pull toward the centre of the own space, so spaces form clusters. */
  gleicheAblage: number;
  /** Minimum gap between two circles. */
  luecke: number;
}

// Physics constants for the large workspace view.
export const PHYSIK_GROSS: Physik = {
  abstossung: 200,
  reichweite: 700,
  federEltern: 0.9,
  federVerweis: 0.35,
  ruheEltern: 55,
  ruheVerweis: 120,
  schwerkraft: 0.035,
  gleicheAblage: 0.025,
  luecke: 10,
};

// Physics constants for the small inline graph: fewer nodes, less space. The
// large-view constants would cause a few nodes to fly to the edges here.
export const PHYSIK_KLEIN: Physik = {
  abstossung: 140,
  reichweite: 300,
  federEltern: 0.9,
  federVerweis: 0.6,
  ruheEltern: 45,
  ruheVerweis: 75,
  schwerkraft: 0.07,
  gleicheAblage: 0,
  luecke: 8,
};

// Friction per tick: the share of velocity that is lost.
const REIBUNG = 0.42;
// Upper bound per tick, against single nodes shooting off.
const MAX_TEMPO = 35;
// Below this distance repulsion no longer grows; avoids huge kicks when two
// nodes start at the same point.
const ABSTAND_MIN = 24;
// Temperature: it moves toward `alphaZiel` instead of jumping. Cooling from 1
// to ALPHA_MIN takes about 300 ticks (five seconds at 60 frames).
const ALPHA_MIN = 0.004;
const ALPHA_ABFALL = 1 - Math.pow(ALPHA_MIN, 1 / 300);
const ZUG_ALPHA = 0.3; // keeps the simulation warm while the user drags
const ZUG_SCHWELLE = 4; // pixels threshold above which a press is a drag, not a click
// The simulation ticks at a fixed rate, independent of the display: on a
// 120 Hz screen it used to run twice as fast and twice as nervous.
const TICK_MS = 1000 / 60;
// When a graph appears, the first ticks are computed before the first frame,
// so the layout unfolds from a rough shape instead of exploding from a ring.
const VORLAUF_MS = 120;
const VORLAUF_ALPHA = 0.3;

// What the settings panel controls, as in Obsidian. The forces are factors on
// the base physics of the view, 1 is the default.
export interface Ansicht {
  suche: string;
  waisen: boolean;
  pfeile: boolean;
  faerben: boolean;
  /** 0 = names stay even far out, 1 = names only when zoomed in close. */
  textSchwelle: number;
  knotenGroesse: number;
  linienStaerke: number;
  zentrum: number;
  abstossung: number;
  verbindung: number;
  abstand: number;
}

export const ANSICHT_STANDARD: Ansicht = {
  suche: "",
  waisen: true,
  pfeile: false,
  faerben: true,
  textSchwelle: 0.5,
  knotenGroesse: 1,
  linienStaerke: 1,
  zentrum: 1,
  abstossung: 1,
  verbindung: 1,
  abstand: 1,
};

const ANSICHT_SCHLUESSEL = "nexora.graph.ansicht";

function ansichtLaden(): Ansicht {
  try {
    const roh = localStorage.getItem(ANSICHT_SCHLUESSEL);
    if (roh) return { ...ANSICHT_STANDARD, ...JSON.parse(roh), suche: "" };
  } catch {
    // private window or blocked storage: defaults
  }
  return ANSICHT_STANDARD;
}

function ansichtSpeichern(a: Ansicht) {
  try {
    // The search is a passing filter, not a setting.
    localStorage.setItem(ANSICHT_SCHLUESSEL, JSON.stringify({ ...a, suche: "" }));
  } catch {
    // ignore
  }
}

// Zoom limits and label font size (in graph units, so it scales with the zoom
// like in Obsidian).
const ZOOM_MIN = 0.1;
const ZOOM_MAX = 4;
const LABEL_SCHRIFT = 11;

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
  /** Show the settings panel (filters, display, forces). */
  einstellungen?: boolean;
  /** User-selected colors per space; key is the spaceId. */
  eigeneFarben?: Record<string, string>;
  /** Callback to change a space color. If provided, the legend shows a color
      picker; otherwise the legend is read-only. */
  onFarbe?: (spaceId: string, farbe: string) => void;
  /**
   * Which spaces this account may colour. Without the field it may colour all
   * that appear in the legend.
   *
   * Necessary because the service only lets the owner and an administrator
   * through: a picker on somebody else's space would accept the colour, the
   * service would reject it, and it would jump back without comment. A control
   * that snaps back looks broken -- so it does not exist there in the first
   * place.
   */
  faerbbar?: Set<string>;
  /** Whether the mouse wheel zooms. In a page view only with Ctrl to avoid
      interfering with normal scrolling. */
  radZoom?: "immer" | "mit-strg";
}

function Regler({
  label,
  wert,
  min,
  max,
  schritt = 0.05,
  onWert,
}: {
  label: string;
  wert: number;
  min: number;
  max: number;
  schritt?: number;
  onWert: (v: number) => void;
}) {
  return (
    <label className="graph-regler">
      <span>{label}</span>
      <input type="range" min={min} max={max} step={schritt} value={wert} onChange={(e) => onWert(+e.target.value)} />
    </label>
  );
}

function Schalter({ label, wert, onWert }: { label: string; wert: boolean; onWert: (v: boolean) => void }) {
  return (
    <label className="graph-schalter">
      <span>{label}</span>
      <input type="checkbox" checked={wert} onChange={(e) => onWert(e.target.checked)} />
    </label>
  );
}

function Abschnitt({ titel, offen, onKlick, children }: { titel: string; offen: boolean; onKlick: () => void; children: React.ReactNode }) {
  return (
    <div className="graph-abschnitt">
      <button type="button" className="graph-abschnitt-kopf" onClick={onKlick} aria-expanded={offen}>
        <span className="graph-abschnitt-pfeil">{offen ? "▾" : "▸"}</span>
        {titel}
      </button>
      {offen && <div className="graph-abschnitt-inhalt">{children}</div>}
    </div>
  );
}

export default function Grafbild({
  graph: graphRoh,
  onOeffnen,
  mitte,
  physik = PHYSIK_GROSS,
  hoehe,
  hinweis,
  legende = false,
  zentrieren = false,
  einstellungen = false,
  eigeneFarben,
  onFarbe,
  faerbbar,
  radZoom = "immer",
}: Props) {
  const rahmenRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const pfeilId = "graph-pfeil-" + useId().replace(/:/g, "");
  const [groesse, setGroesse] = useState({ w: 900, h: hoehe ?? 600 });
  const [blick, setBlick] = useState({ x: 0, y: 0, scale: 1 });
  const [zeiger, setZeiger] = useState<string | null>(null);
  const [, neuZeichnen] = useState(0);
  const [ansicht, setAnsicht] = useState<Ansicht>(() =>
    // The small graph under a page has no panel and always shows its names.
    einstellungen ? ansichtLaden() : { ...ANSICHT_STANDARD, textSchwelle: 0 },
  );
  const [panelOffen, setPanelOffen] = useState(false);
  const [offeneAbschnitte, setOffeneAbschnitte] = useState<Record<string, boolean>>({ filter: true, darstellung: false, kraefte: false });

  // Filters: search and orphans narrow the set of pages, the simulation then
  // only knows the visible ones (as in Obsidian).
  const graph = useMemo<Graph>(() => {
    const q = ansicht.suche.trim().toLowerCase();
    if (!q && ansicht.waisen) return graphRoh;
    const grad: Record<string, number> = {};
    for (const e of graphRoh.edges) {
      grad[e.source] = (grad[e.source] || 0) + 1;
      grad[e.target] = (grad[e.target] || 0) + 1;
    }
    const nodes = graphRoh.nodes.filter(
      (n) => (!q || (n.title || "").toLowerCase().includes(q)) && (ansicht.waisen || grad[n.id] || n.id === mitte),
    );
    const ids = new Set(nodes.map((n) => n.id));
    return { nodes, edges: graphRoh.edges.filter((e) => ids.has(e.source) && ids.has(e.target)) };
  }, [graphRoh, ansicht.suche, ansicht.waisen, mitte]);

  const aendern = (teil: Partial<Ansicht>, heizen = 0) => {
    setAnsicht((vorher) => {
      const neu = { ...vorher, ...teil };
      if (einstellungen) ansichtSpeichern(neu);
      return neu;
    });
    if (heizen) anheizen(heizen);
  };

  const groesseRef = useRef(groesse);
  groesseRef.current = groesse;
  const blickRef = useRef(blick);
  blickRef.current = blick;
  const physikRef = useRef(physik);
  physikRef.current = {
    ...physik,
    schwerkraft: physik.schwerkraft * ansicht.zentrum,
    abstossung: physik.abstossung * ansicht.abstossung,
    federEltern: physik.federEltern * ansicht.verbindung,
    federVerweis: physik.federVerweis * ansicht.verbindung,
    ruheEltern: physik.ruheEltern * ansicht.abstand,
    ruheVerweis: physik.ruheVerweis * ansicht.abstand,
  };
  const mitteRef = useRef(mitte);
  mitteRef.current = mitte;
  const ansichtRef = useRef(ansicht);
  ansichtRef.current = ansicht;

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
      eintraege.push({ key: k, label: k === "__none__" ? "No space" : n.space || "Space", color: farbe[k] });
    }
    return { grad, nachbarn, legende: eintraege };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey, graph.edges.length]);
  const gradRef = useRef(abgeleitet.grad);
  gradRef.current = abgeleitet.grad;

  // Small dots that grow with the number of links, like in Obsidian; the
  // centre page of the small graph a little larger.
  const radius = (id: string) =>
    ((id === mitteRef.current ? 2 : 0) + 4 + Math.min(8, Math.sqrt(gradRef.current[id] || 0) * 1.7)) *
    ansichtRef.current.knotenGroesse;

  const teilchen = useRef<Record<string, Teilchen>>({});
  const alpha = useRef(0);
  const alphaZiel = useRef(0);
  const raf = useRef<number | null>(null);
  const letzterTakt = useRef<number | null>(null);
  const zeitRest = useRef(0);
  const gehalten = useRef<{ id: string; x: number; y: number } | null>(null);
  const ablageVon = useRef<Record<string, string>>({});
  // After the first unfolding the view fits itself to the graph once it has
  // come to rest.
  const einpassenNachRuhe = useRef(false);
  const blickAnimation = useRef<number | null>(null);

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

  // Keep the picture where it was when the frame changes size: shift every
  // node by the movement of the centre. Without this a cooled graph stayed
  // in the old corner after a window resize.
  const mitteVorher = useRef<{ x: number; y: number } | null>(null);
  useEffect(() => {
    const neu = { x: groesse.w / 2, y: groesse.h / 2 };
    const alt = mitteVorher.current;
    mitteVorher.current = neu;
    if (!alt) return;
    const dx = neu.x - alt.x;
    const dy = neu.y - alt.y;
    if (!dx && !dy) return;
    for (const t of Object.values(teilchen.current)) {
      t.x += dx;
      t.y += dy;
    }
    neuZeichnen((v) => v + 1);
  }, [groesse.w, groesse.h]);

  // Move the view smoothly to a target instead of jumping.
  const blickNach = (ziel: { x: number; y: number; scale: number }, ms = 350) => {
    if (blickAnimation.current != null) cancelAnimationFrame(blickAnimation.current);
    const von = blickRef.current;
    const start = performance.now();
    const weiter = (jetzt: number) => {
      const t = Math.min(1, (jetzt - start) / ms);
      const e = 1 - Math.pow(1 - t, 3);
      setBlick({
        x: von.x + (ziel.x - von.x) * e,
        y: von.y + (ziel.y - von.y) * e,
        scale: von.scale + (ziel.scale - von.scale) * e,
      });
      blickAnimation.current = t < 1 ? requestAnimationFrame(weiter) : null;
    };
    blickAnimation.current = requestAnimationFrame(weiter);
  };

  // Fit the whole graph into the frame, never closer than 1:1.
  const einpassen = (weich = true) => {
    const ts = Object.values(teilchen.current);
    if (!ts.length) return;
    let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (const t of ts) {
      x0 = Math.min(x0, t.x);
      y0 = Math.min(y0, t.y);
      x1 = Math.max(x1, t.x);
      y1 = Math.max(y1, t.y);
    }
    const { w, h } = groesseRef.current;
    const rand = 60;
    const scale = Math.max(ZOOM_MIN, Math.min(1, w / (x1 - x0 + rand * 2), h / (y1 - y0 + rand * 2)));
    const ziel = { scale, x: w / 2 - ((x0 + x1) / 2) * scale, y: h / 2 - ((y0 + y1) / 2) * scale };
    if (weich) blickNach(ziel);
    else setBlick(ziel);
  };

  // One simulation step over the current particle set.
  const schritt = () => {
    const nodes = graph.nodes;
    const p = teilchen.current;
    const a = alpha.current;
    const { w, h } = groesseRef.current;
    const ph = physikRef.current;
    const grad = abgeleitet.grad;

    const ids: string[] = [];
    const ts: Teilchen[] = [];
    const rs: number[] = [];
    for (const n of nodes) {
      const t = p[n.id];
      if (!t) continue;
      ids.push(n.id);
      ts.push(t);
      rs.push(radius(n.id));
    }
    const anzahl = ts.length;
    if (!anzahl) return;

    // Pull toward the centre of the own space. One centre per space instead
    // of an attraction between every pair: that grew with distance and
    // dragged far nodes across the whole picture.
    if (ph.gleicheAblage) {
      const summe: Record<string, { x: number; y: number; n: number }> = {};
      for (let i = 0; i < anzahl; i++) {
        const k = ablageVon.current[ids[i]];
        const s = (summe[k] ??= { x: 0, y: 0, n: 0 });
        s.x += ts[i].x;
        s.y += ts[i].y;
        s.n++;
      }
      if (Object.keys(summe).length > 1) {
        for (let i = 0; i < anzahl; i++) {
          const s = summe[ablageVon.current[ids[i]]];
          if (s.n < 2) continue;
          ts[i].vx += (s.x / s.n - ts[i].x) * ph.gleicheAblage * a;
          ts[i].vy += (s.y / s.n - ts[i].y) * ph.gleicheAblage * a;
        }
      }
    }

    // Springs along edges, on the positions the nodes are about to reach
    // (as d3 does), which damps the swinging back and forth.
    for (const e of graph.edges) {
      const s = p[e.source];
      const t = p[e.target];
      if (!s || !t || s === t) continue;
      let dx = t.x + t.vx - s.x - s.vx;
      let dy = t.y + t.vy - s.y - s.vy;
      if (!dx && !dy) {
        dx = (Math.random() - 0.5) * 1e-3;
        dy = (Math.random() - 0.5) * 1e-3;
      }
      const dist = Math.sqrt(dx * dx + dy * dy);
      const gs = Math.max(1, grad[e.source] || 0);
      const gt = Math.max(1, grad[e.target] || 0);
      const eltern = e.kind === "parent";
      const ruhe = (eltern ? ph.ruheEltern : ph.ruheVerweis) + radius(e.source) + radius(e.target);
      const k = (eltern ? ph.federEltern : ph.federVerweis) / Math.min(gs, gt);
      const f = ((dist - ruhe) / dist) * a * k;
      dx *= f;
      dy *= f;
      // The node with fewer links moves more.
      const b = gs / (gs + gt);
      t.vx -= dx * b;
      t.vy -= dy * b;
      s.vx += dx * (1 - b);
      s.vy += dy * (1 - b);
    }

    // Repulsion and collision between every pair.
    const reich2 = ph.reichweite * ph.reichweite;
    const min2 = ABSTAND_MIN * ABSTAND_MIN;
    for (let i = 0; i < anzahl; i++) {
      const pa = ts[i];
      for (let j = i + 1; j < anzahl; j++) {
        const pb = ts[j];
        let dx = pa.x - pb.x;
        let dy = pa.y - pb.y;
        let l = dx * dx + dy * dy;
        if (l < 1e-6) {
          dx = (Math.random() - 0.5) * 1e-2;
          dy = (Math.random() - 0.5) * 1e-2;
          l = dx * dx + dy * dy;
        }
        if (l < reich2) {
          const lw = l < min2 ? Math.sqrt(min2 * l) : l;
          const f = (ph.abstossung * a) / lw;
          pa.vx += dx * f;
          pa.vy += dy * f;
          pb.vx -= dx * f;
          pb.vy -= dy * f;
        }
        // Collision works on the predicted positions and does not depend
        // on the temperature: circles do not overlap even when cooled.
        const rr = rs[i] + rs[j] + ph.luecke;
        const cx = pa.x + pa.vx - pb.x - pb.vx;
        const cy = pa.y + pa.vy - pb.y - pb.vy;
        const cl = cx * cx + cy * cy;
        if (cl < rr * rr) {
          const d = Math.sqrt(cl) || 0.01;
          const m = ((rr - d) / d) * 0.6;
          // Larger circles give way less.
          const anteil = (rs[j] * rs[j]) / (rs[i] * rs[i] + rs[j] * rs[j]);
          pa.vx += cx * m * anteil;
          pa.vy += cy * m * anteil;
          pb.vx -= cx * m * (1 - anteil);
          pb.vy -= cy * m * (1 - anteil);
        }
      }
    }

    // Gravity toward the centre. The emphasized node is held more strongly so
    // that the focused page remains near the centre in the small graph.
    for (let i = 0; i < anzahl; i++) {
      const stark = ids[i] === mitteRef.current ? 6 : 1;
      ts[i].vx += (w / 2 - ts[i].x) * ph.schwerkraft * stark * a;
      ts[i].vy += (h / 2 - ts[i].y) * ph.schwerkraft * stark * a;
    }

    // Integrate velocities to advance positions.
    const f = gehalten.current;
    for (let i = 0; i < anzahl; i++) {
      const pp = ts[i];
      if (f && f.id === ids[i]) {
        pp.x = f.x;
        pp.y = f.y;
        pp.vx = 0;
        pp.vy = 0;
        continue;
      }
      pp.vx *= 1 - REIBUNG;
      pp.vy *= 1 - REIBUNG;
      const tempo = Math.hypot(pp.vx, pp.vy);
      if (tempo > MAX_TEMPO) {
        pp.vx *= MAX_TEMPO / tempo;
        pp.vy *= MAX_TEMPO / tempo;
      }
      pp.x += pp.vx;
      pp.y += pp.vy;
    }

    // Drift the whole layout toward the centre. Only a share per tick, so
    // releasing a node no longer makes the whole picture jump; skipped while
    // dragging so the grabbed node stays under the cursor.
    if (!f) {
      let cx = 0;
      let cy = 0;
      for (const t of ts) {
        cx += t.x;
        cy += t.y;
      }
      const sx = (w / 2 - cx / anzahl) * 0.1;
      const sy = (h / 2 - cy / anzahl) * 0.1;
      for (const t of ts) {
        t.x += sx;
        t.y += sy;
      }
    }
  };


  // One tick: temperature moves toward its target, then the forces act.
  const takt = () => {
    alpha.current += (alphaZiel.current - alpha.current) * ALPHA_ABFALL;
    schritt();
  };

  const laeuft = () => alpha.current > ALPHA_MIN || alphaZiel.current > 0 || gehalten.current != null;

  const schleife = (jetzt: number) => {
    const dt = letzterTakt.current == null ? TICK_MS : Math.min(100, jetzt - letzterTakt.current);
    letzterTakt.current = jetzt;
    zeitRest.current += dt;
    let n = 0;
    // A small tolerance, so a display at 59.9 Hz does not skip every other tick.
    while (zeitRest.current >= TICK_MS * 0.9 && n < 4) {
      takt();
      zeitRest.current -= TICK_MS;
      n++;
    }
    if (n === 4 || zeitRest.current < 0) zeitRest.current = 0;
    if (n > 0) neuZeichnen((v) => v + 1);
    if (laeuft()) {
      raf.current = requestAnimationFrame(schleife);
    } else {
      raf.current = null;
      letzterTakt.current = null;
      if (einpassenNachRuhe.current) {
        einpassenNachRuhe.current = false;
        einpassen();
      }
    }
  };

  const starten = () => {
    if (raf.current == null) {
      letzterTakt.current = null;
      zeitRest.current = TICK_MS;
      raf.current = requestAnimationFrame(schleife);
    }
  };

  // Raise the temperature to at least `ziel` and run.
  const anheizen = (ziel = 1) => {
    alpha.current = Math.max(alpha.current, ziel);
    starten();
  };

  // Place nodes without a position: next to a neighbour that already has a
  // place, otherwise on a sunflower spiral around the middle. A ring placed
  // everything at the same distance and the first second was an explosion
  // outward; the spiral spreads evenly without symmetries the forces cannot
  // break.
  const platzieren = (naechste: Record<string, Teilchen>) => {
    const { w, h } = groesseRef.current;
    const goldenerWinkel = Math.PI * (3 - Math.sqrt(5));
    let spirale = 0;
    let neu = 0;
    for (const node of graph.nodes) {
      if (naechste[node.id]) continue;
      neu++;
      if (node.id === mitte) {
        naechste[node.id] = { x: w / 2, y: h / 2, vx: 0, vy: 0 };
        continue;
      }
      let nachbar: Teilchen | undefined;
      for (const id of abgeleitet.nachbarn[node.id] ?? []) {
        if (naechste[id]) {
          nachbar = naechste[id];
          break;
        }
      }
      if (nachbar) {
        const ang = Math.random() * Math.PI * 2;
        naechste[node.id] = { x: nachbar.x + Math.cos(ang) * 40, y: nachbar.y + Math.sin(ang) * 40, vx: 0, vy: 0 };
      } else {
        const r = 28 * Math.sqrt(0.5 + spirale);
        const ang = spirale * goldenerWinkel;
        spirale++;
        naechste[node.id] = { x: w / 2 + Math.cos(ang) * r, y: h / 2 + Math.sin(ang) * r, vx: 0, vy: 0 };
      }
    }
    return neu;
  };

  // Compute the rough unfolding before the first frame and only animate the
  // settling; afterwards fit the view.
  const entfalten = () => {
    alpha.current = 1;
    alphaZiel.current = 0;
    const ende = performance.now() + VORLAUF_MS;
    while (alpha.current > VORLAUF_ALPHA && performance.now() < ende) takt();
    einpassen(false);
    einpassenNachRuhe.current = true;
    neuZeichnen((v) => v + 1);
    starten();
  };

  // Reinitialize particles whenever the node set changes, then run the
  // simulation. Known nodes retain their positions across data updates.
  useEffect(() => {
    const naechste: Record<string, Teilchen> = {};
    const ablagen: Record<string, string> = {};
    for (const node of graph.nodes) {
      ablagen[node.id] = ablageSchluessel(node);
      const vorher = teilchen.current[node.id];
      if (vorher) naechste[node.id] = vorher;
    }
    const neu = platzieren(naechste);
    teilchen.current = naechste;
    ablageVon.current = ablagen;
    if (graph.nodes.length) {
      if (neu > graph.nodes.length / 2) entfalten();
      else anheizen(0.5);
    }
    return () => {
      if (raf.current != null) cancelAnimationFrame(raf.current);
      raf.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  // "Animate" in Obsidian: throw everything up again and let it settle anew.
  const animieren = () => {
    teilchen.current = {};
    platzieren(teilchen.current);
    alpha.current = 1;
    alphaZiel.current = 0;
    einpassenNachRuhe.current = true;
    starten();
  };

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
    if (blickAnimation.current != null) {
      cancelAnimationFrame(blickAnimation.current);
      blickAnimation.current = null;
    }
    einpassenNachRuhe.current = false;
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
      alphaZiel.current = ZUG_ALPHA;
      // A start boost, so the neighbours react from the first movement on.
      alpha.current = Math.max(alpha.current, 0.12);
      starten();
    } else {
      zug.current = {
        art: "flaeche",
        offX: 0,
        offY: 0,
        startX: e.clientX,
        startY: e.clientY,
        panX: blickRef.current.x,
        panY: blickRef.current.y,
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
      starten();
    } else if (d.art === "flaeche") {
      setBlick((v) => ({ ...v, x: d.panX + dx, y: d.panY + dy }));
    }
  };

  // On release: a press that never exceeded the drag threshold is treated as
  // a click and opens the page, so a dragged node does not accidentally
  // navigate elsewhere.
  const zeigerAuf = (e: React.PointerEvent) => {
    const d = zug.current;
    svgRef.current?.releasePointerCapture(e.pointerId);
    if (d.art === "knoten" && d.id && !d.bewegt) onOeffnen(d.id);
    // Let go: the temperature sinks back on its own instead of jumping, so
    // the neighbours glide into place rather than freezing mid-movement.
    if (gehalten.current) {
      gehalten.current = null;
      alphaZiel.current = 0;
      starten();
    }
    zug.current.art = null;
  };

  // Zoom toward the pointer, not the center: the point under the pointer
  // should remain fixed. Proportional to the wheel distance, so a trackpad
  // zooms smoothly and a mouse wheel in steps.
  const amRad = (e: React.WheelEvent) => {
    if (radZoom === "mit-strg" && !e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    if (blickAnimation.current != null) {
      cancelAnimationFrame(blickAnimation.current);
      blickAnimation.current = null;
    }
    einpassenNachRuhe.current = false;
    const faktor = Math.exp(-Math.max(-100, Math.min(100, e.deltaY)) * 0.0015);
    const rect = svgRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    setBlick((v) => {
      const scale = Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, v.scale * faktor));
      return { scale, x: mx - ((mx - v.x) / v.scale) * scale, y: my - ((my - v.y) / v.scale) * scale };
    });
  };

  // Centre: fit the graph into the frame and nudge the simulation.
  const zurueck = () => {
    einpassen();
    if (graph.nodes.length) anheizen(0.3);
  };

  const p = teilchen.current;
  const nachbarVon = zeiger ? abgeleitet.nachbarn[zeiger] : undefined;
  const imFokus = (id: string) => !zeiger || id === zeiger || !!nachbarVon?.has(id);

  // Names fade with the zoom like in Obsidian: from a threshold on they
  // appear, below it the picture is only dots and lines. The page under the
  // pointer and its neighbours always show their names.
  const schwelle = 0.25 + ansicht.textSchwelle * 1.1;
  const textDeckkraft = Math.max(0, Math.min(1, (blick.scale - schwelle) / 0.3 + 1));
  const legendeZeigen = legende && ansicht.faerben && abgeleitet.legende.length > (onFarbe ? 0 : 1);

  return (
    <div className={"graph-wrap" + (zeiger ? " hat-fokus" : "")} ref={rahmenRef} style={hoehe ? { height: hoehe } : undefined}>
      {hinweis && <div className="graph-hint">{hinweis}</div>}
      {/* A legend of a single entry says nothing the graph does not show
          already -- except when the colour is chosen at its dot. Then it is no
          longer a sign but the control panel, and were it missing, a space in a
          workspace with only one space could get its colour nowhere. */}
      {legendeZeigen && (
        <div className="graph-legend">
          {abgeleitet.legende.map((l) => (
            <span key={l.key} className="graph-legend-item">
              {/* The dot itself is the color picker: click it, choose a color,
                  done. For "No space" only the dot is shown; there is no space
                  to attach a color to. */}
              {onFarbe && l.key !== "__none__" && (!faerbbar || faerbbar.has(l.key)) ? (
                <label className="graph-legend-dot-wahl" title="Colour of this space">
                  <span className="graph-legend-dot" style={{ background: farbe[l.key] }} />
                  <input type="color" value={farbe[l.key]} onChange={(e) => onFarbe(l.key, e.target.value)} />
                </label>
              ) : (
                <span className="graph-legend-dot" style={{ background: farbe[l.key] }} />
              )}
              {l.label}
            </span>
          ))}
        </div>
      )}
      {(zentrieren || einstellungen) && (
        <div className="graph-werkzeuge">
          {zentrieren && (
            <button className="btn graph-reset" onClick={zurueck}>
              Zentrieren
            </button>
          )}
          {einstellungen && (
            <button
              className={"btn graph-zahnrad" + (panelOffen ? " aktiv" : "")}
              onClick={() => setPanelOffen((o) => !o)}
              title="Graph settings"
              aria-label="Graph settings"
              aria-expanded={panelOffen}
            >
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
              </svg>
            </button>
          )}
        </div>
      )}
      {einstellungen && panelOffen && (
        <div className="graph-panel">
          <Abschnitt titel="Filters" offen={offeneAbschnitte.filter} onKlick={() => setOffeneAbschnitte((o) => ({ ...o, filter: !o.filter }))}>
            <input
              className="graph-suche"
              type="search"
              placeholder="Search pages…"
              value={ansicht.suche}
              onChange={(e) => aendern({ suche: e.target.value })}
            />
            <Schalter label="Orphans" wert={ansicht.waisen} onWert={(v) => aendern({ waisen: v })} />
          </Abschnitt>
          <Abschnitt titel="Display" offen={offeneAbschnitte.darstellung} onKlick={() => setOffeneAbschnitte((o) => ({ ...o, darstellung: !o.darstellung }))}>
            <Schalter label="Arrows" wert={ansicht.pfeile} onWert={(v) => aendern({ pfeile: v })} />
            <Schalter label="Colour by space" wert={ansicht.faerben} onWert={(v) => aendern({ faerben: v })} />
            <Regler label="Text fade threshold" wert={ansicht.textSchwelle} min={0} max={1} onWert={(v) => aendern({ textSchwelle: v })} />
            <Regler label="Node size" wert={ansicht.knotenGroesse} min={0.4} max={2.5} onWert={(v) => aendern({ knotenGroesse: v }, 0.2)} />
            <Regler label="Link thickness" wert={ansicht.linienStaerke} min={0.3} max={4} onWert={(v) => aendern({ linienStaerke: v })} />
            <button type="button" className="btn graph-animieren" onClick={animieren}>
              Animate
            </button>
          </Abschnitt>
          <Abschnitt titel="Forces" offen={offeneAbschnitte.kraefte} onKlick={() => setOffeneAbschnitte((o) => ({ ...o, kraefte: !o.kraefte }))}>
            <Regler label="Centre force" wert={ansicht.zentrum} min={0} max={3} onWert={(v) => aendern({ zentrum: v }, 0.3)} />
            <Regler label="Repel force" wert={ansicht.abstossung} min={0} max={3} onWert={(v) => aendern({ abstossung: v }, 0.3)} />
            <Regler label="Link force" wert={ansicht.verbindung} min={0} max={2} onWert={(v) => aendern({ verbindung: v }, 0.3)} />
            <Regler label="Link distance" wert={ansicht.abstand} min={0.2} max={3} onWert={(v) => aendern({ abstand: v }, 0.3)} />
            <button
              type="button"
              className="btn graph-animieren"
              onClick={() => aendern({ zentrum: 1, abstossung: 1, verbindung: 1, abstand: 1 }, 0.5)}
            >
              Reset
            </button>
          </Abschnitt>
        </div>
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
        <defs>
          <marker id={pfeilId} viewBox="0 0 10 10" refX="10" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" className="graph-pfeil" />
          </marker>
        </defs>
        <g transform={`translate(${blick.x}, ${blick.y}) scale(${blick.scale})`}>
          {graph.edges.map((kante, i) => {
            const a = p[kante.source];
            const b = p[kante.target];
            if (!a || !b) return null;
            const hell = !!zeiger && (kante.source === zeiger || kante.target === zeiger);
            // With arrows the line ends at the edge of the target circle,
            // otherwise the tip would hide under the dot.
            let x2 = b.x;
            let y2 = b.y;
            if (ansicht.pfeile) {
              const dx = b.x - a.x;
              const dy = b.y - a.y;
              const l = Math.hypot(dx, dy) || 1;
              const r = radius(kante.target) + 1.5;
              x2 = b.x - (dx / l) * r;
              y2 = b.y - (dy / l) * r;
            }
            return (
              <line
                key={i}
                x1={a.x}
                y1={a.y}
                x2={x2}
                y2={y2}
                className={"graph-kante" + (hell ? " betont" : zeiger ? " blass" : "")}
                strokeWidth={(hell ? 1.6 : 1) * ansicht.linienStaerke}
                markerEnd={ansicht.pfeile ? `url(#${pfeilId})` : undefined}
              />
            );
          })}
          {graph.nodes.map((node: GraphNode) => {
            const pp = p[node.id];
            if (!pp) return null;
            const r = radius(node.id);
            const fokus = imFokus(node.id);
            const selbst = zeiger === node.id;
            const text = zeiger ? (fokus ? 1 : 0) : node.id === mitte ? Math.max(textDeckkraft, 1) : textDeckkraft;
            return (
              <g
                key={node.id}
                data-node={node.id}
                transform={`translate(${pp.x}, ${pp.y})`}
                className={"graph-node" + (fokus ? "" : " blass")}
                onPointerEnter={() => setZeiger(node.id)}
                onPointerLeave={() => setZeiger(null)}
              >
                {/* A larger invisible circle makes small dots easy to grab. */}
                <circle r={Math.max(r + 4, 10)} className="graph-treffer" />
                <circle
                  r={selbst ? r * 1.25 : r}
                  fill={selbst ? undefined : ansicht.faerben ? farbe[ablageSchluessel(node)] : undefined}
                  className={"graph-knoten" + (selbst ? " betont" : "") + (node.id === mitte ? " mitte" : "")}
                />
                <text
                  y={r + LABEL_SCHRIFT + 2}
                  textAnchor="middle"
                  fontSize={LABEL_SCHRIFT}
                  className={"graph-name" + (selbst ? " betont" : "")}
                  opacity={text}
                  fontWeight={node.id === mitte ? 600 : undefined}
                >
                  {node.title || "Untitled"}
                </text>
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}
