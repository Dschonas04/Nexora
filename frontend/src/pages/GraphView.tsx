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

export default function GraphView() {
  const nav = useNavigate();
  const [graph, setGraph] = useState<Graph>({ nodes: [], edges: [] });
  const wrapRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ w: 900, h: 600 });

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

  const pos = useMemo(() => layout(graph, size.w, size.h), [graph, size.w, size.h]);

  return (
    <div className="graph-wrap" ref={wrapRef}>
      {graph.nodes.length === 0 ? (
        <div className="empty-state">No pages to graph yet.</div>
      ) : (
        <svg width={size.w} height={size.h}>
          {graph.edges.map((e, i) =>
            pos[e.source] && pos[e.target] ? (
              <line
                key={i}
                x1={pos[e.source].x}
                y1={pos[e.source].y}
                x2={pos[e.target].x}
                y2={pos[e.target].y}
                stroke={e.kind === "parent" ? "#c7c7c4" : "#dcdcda"}
                strokeWidth={e.kind === "parent" ? 1.5 : 1}
                strokeDasharray={e.kind === "link" ? "4 3" : undefined}
              />
            ) : null
          )}
          {graph.nodes.map((node) => (
            <g
              key={node.id}
              transform={`translate(${pos[node.id]?.x ?? 0}, ${pos[node.id]?.y ?? 0})`}
              className="graph-node"
              onClick={() => nav(`/page/${node.id}`)}
            >
              <circle r={7} fill="#2383e2" />
              <text x={11} y={4} fontSize={12} fill="#37352f">
                {node.title || "Untitled"}
              </text>
            </g>
          ))}
        </svg>
      )}
    </div>
  );
}
