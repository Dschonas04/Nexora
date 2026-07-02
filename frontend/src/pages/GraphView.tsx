import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Graph, api } from "../api/client";

interface Pos {
  x: number;
  y: number;
}

// A tiny force-directed layout: a handful of iterations of repulsion between
// all nodes plus spring attraction along edges. No external dependency.
function layout(graph: Graph, width: number, height: number): Record<string, Pos> {
  const pos: Record<string, Pos> = {};
  const n = graph.nodes.length || 1;
  graph.nodes.forEach((node, i) => {
    const angle = (i / n) * Math.PI * 2;
    pos[node.id] = {
      x: width / 2 + Math.cos(angle) * (Math.min(width, height) / 3),
      y: height / 2 + Math.sin(angle) * (Math.min(width, height) / 3),
    };
  });

  for (let iter = 0; iter < 220; iter++) {
    const disp: Record<string, Pos> = {};
    for (const node of graph.nodes) disp[node.id] = { x: 0, y: 0 };

    // Repulsion.
    for (let i = 0; i < graph.nodes.length; i++) {
      for (let j = i + 1; j < graph.nodes.length; j++) {
        const a = graph.nodes[i].id;
        const b = graph.nodes[j].id;
        let dx = pos[a].x - pos[b].x;
        let dy = pos[a].y - pos[b].y;
        let dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
        const force = 4200 / (dist * dist);
        dx /= dist;
        dy /= dist;
        disp[a].x += dx * force;
        disp[a].y += dy * force;
        disp[b].x -= dx * force;
        disp[b].y -= dy * force;
      }
    }
    // Attraction along edges.
    for (const e of graph.edges) {
      if (!pos[e.source] || !pos[e.target]) continue;
      let dx = pos[e.source].x - pos[e.target].x;
      let dy = pos[e.source].y - pos[e.target].y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const force = (dist - 90) * 0.04;
      dx /= dist;
      dy /= dist;
      disp[e.source].x -= dx * force;
      disp[e.source].y -= dy * force;
      disp[e.target].x += dx * force;
      disp[e.target].y += dy * force;
    }
    for (const node of graph.nodes) {
      pos[node.id].x = Math.max(30, Math.min(width - 30, pos[node.id].x + disp[node.id].x * 0.05));
      pos[node.id].y = Math.max(30, Math.min(height - 30, pos[node.id].y + disp[node.id].y * 0.05));
    }
  }
  return pos;
}

const DRAG_THRESHOLD = 4; // px moved before a press counts as a drag, not a click

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const wrapRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [size, setSize] = useState({ w: 900, h: 600 });
  const [positions, setPositions] = useState<Record<string, Pos>>({});
  const [view, setView] = useState({ x: 0, y: 0, scale: 1 });
  const [hover, setHover] = useState<string | null>(null);

  // Interaction state kept in a ref so handlers don't need to re-bind.
  const drag = useRef<{
    kind: "node" | "pan" | null;
    id?: string;
    startX: number;
    startY: number;
    origin: Pos;
    moved: boolean;
  }>({ kind: null, startX: 0, startY: 0, origin: { x: 0, y: 0 }, moved: false });

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

  const initial = useMemo(() => layout(graph, size.w, size.h), [graph, size.w, size.h]);
  useEffect(() => setPositions(initial), [initial]);

  // Convert a client point to graph coordinates (undo pan + zoom).
  const toGraph = (clientX: number, clientY: number): Pos => {
    const rect = svgRef.current!.getBoundingClientRect();
    return {
      x: (clientX - rect.left - view.x) / view.scale,
      y: (clientY - rect.top - view.y) / view.scale,
    };
  };

  const onNodeDown = (e: React.PointerEvent, id: string) => {
    e.stopPropagation();
    (e.target as Element).setPointerCapture?.(e.pointerId);
    drag.current = {
      kind: "node",
      id,
      startX: e.clientX,
      startY: e.clientY,
      origin: positions[id] ?? { x: 0, y: 0 },
      moved: false,
    };
  };

  const onBackgroundDown = (e: React.PointerEvent) => {
    svgRef.current?.setPointerCapture?.(e.pointerId);
    drag.current = {
      kind: "pan",
      startX: e.clientX,
      startY: e.clientY,
      origin: { x: view.x, y: view.y },
      moved: false,
    };
  };

  const onMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d.kind) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    if (!d.moved && Math.hypot(dx, dy) > DRAG_THRESHOLD) d.moved = true;
    if (d.kind === "node" && d.id) {
      const p = toGraph(e.clientX, e.clientY);
      setPositions((prev) => ({ ...prev, [d.id!]: p }));
    } else if (d.kind === "pan") {
      setView((v) => ({ ...v, x: d.origin.x + dx, y: d.origin.y + dy }));
    }
  };

  const onUp = (e: React.PointerEvent) => {
    const d = drag.current;
    if (d.kind === "node" && d.id && !d.moved) nav(`/page/${d.id}`);
    drag.current = { kind: null, startX: 0, startY: 0, origin: { x: 0, y: 0 }, moved: false };
    (e.target as Element).releasePointerCapture?.(e.pointerId);
  };

  const onWheel = (e: React.WheelEvent) => {
    const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    const rect = svgRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    setView((v) => {
      const scale = Math.max(0.3, Math.min(3, v.scale * factor));
      // Zoom towards the cursor.
      return {
        scale,
        x: mx - ((mx - v.x) / v.scale) * scale,
        y: my - ((my - v.y) / v.scale) * scale,
      };
    });
  };

  const reset = () => setView({ x: 0, y: 0, scale: 1 });

  return (
    <div className="graph-wrap" ref={wrapRef}>
      {graph.nodes.length === 0 ? (
        <div className="empty-state">No pages to graph yet.</div>
      ) : (
        <>
          <div className="graph-hint">Drag nodes · drag background to pan · scroll to zoom</div>
          <button className="btn graph-reset" onClick={reset}>
            Reset view
          </button>
          <svg
            ref={svgRef}
            width={size.w}
            height={size.h}
            className="graph-svg"
            onPointerDown={onBackgroundDown}
            onPointerMove={onMove}
            onPointerUp={onUp}
            onWheel={onWheel}
          >
            <g transform={`translate(${view.x}, ${view.y}) scale(${view.scale})`}>
              {graph.edges.map((edge, i) =>
                positions[edge.source] && positions[edge.target] ? (
                  <line
                    key={i}
                    x1={positions[edge.source].x}
                    y1={positions[edge.source].y}
                    x2={positions[edge.target].x}
                    y2={positions[edge.target].y}
                    stroke={
                      hover && (edge.source === hover || edge.target === hover)
                        ? "#2383e2"
                        : edge.kind === "parent"
                        ? "#c7c7c4"
                        : "#dcdcda"
                    }
                    strokeWidth={edge.kind === "parent" ? 1.5 : 1}
                    strokeDasharray={edge.kind === "link" ? "4 3" : undefined}
                  />
                ) : null
              )}
              {graph.nodes.map((node) => (
                <g
                  key={node.id}
                  transform={`translate(${positions[node.id]?.x ?? 0}, ${positions[node.id]?.y ?? 0})`}
                  className="graph-node"
                  onPointerDown={(e) => onNodeDown(e, node.id)}
                  onPointerEnter={() => setHover(node.id)}
                  onPointerLeave={() => setHover(null)}
                >
                  <circle r={hover === node.id ? 9 : 7} fill={hover === node.id ? "#1a73d0" : "#2383e2"} />
                  <text x={12} y={4} fontSize={12} fill="#37352f">
                    {node.title || "Untitled"}
                  </text>
                </g>
              ))}
            </g>
          </svg>
        </>
      )}
    </div>
  );
}
