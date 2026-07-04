import { Graph } from "../api/client";

interface Props {
  graph: Graph;
  pageId: string;
  onOpen: (id: string) => void;
}

// LocalGraph is the small "linked references" view shown on a page: the current
// page in the middle with its direct neighbours (parent/child + [[wiki-links]],
// both directions) arranged around it. Purely radial, no simulation needed.
export default function LocalGraph({ graph, pageId, onOpen }: Props) {
  const title: Record<string, string> = {};
  for (const n of graph.nodes) title[n.id] = n.title || "Ohne Titel";

  // Collect neighbours and remember the edge kind for styling.
  const neighbours: { id: string; kind: string }[] = [];
  const seen = new Set<string>();
  for (const e of graph.edges) {
    let other: string | null = null;
    if (e.source === pageId) other = e.target;
    else if (e.target === pageId) other = e.source;
    if (other && !seen.has(other) && title[other] !== undefined) {
      seen.add(other);
      neighbours.push({ id: other, kind: e.kind });
    }
  }

  if (neighbours.length === 0) return null;

  const W = 520;
  const cx = W / 2;
  const cy = 150;
  const R = Math.min(120, 40 + neighbours.length * 12);
  const H = cy + R + 40;

  return (
    <div className="local-graph">
      <div className="local-graph-title">Lokaler Graph</div>
      <svg width="100%" viewBox={`0 0 ${W} ${H}`} className="local-graph-svg">
        {neighbours.map((nb, i) => {
          const ang = (i / neighbours.length) * Math.PI * 2 - Math.PI / 2;
          const x = cx + Math.cos(ang) * R;
          const y = cy + Math.sin(ang) * R;
          const label = title[nb.id];
          // Keep labels inside the box: flip anchor on the left half.
          const anchor = x < cx - 4 ? "end" : x > cx + 4 ? "start" : "middle";
          const dx = anchor === "end" ? -10 : anchor === "start" ? 10 : 0;
          return (
            <g key={nb.id}>
              <line
                x1={cx}
                y1={cy}
                x2={x}
                y2={y}
                stroke="#dcdcda"
                strokeWidth={nb.kind === "parent" ? 1.6 : 1}
                strokeDasharray={nb.kind === "link" ? "4 3" : undefined}
              />
              <g className="local-node" onClick={() => onOpen(nb.id)}>
                <circle cx={x} cy={y} r={6} fill="#2383e2" />
                <text
                  x={x + dx}
                  y={y + 4}
                  fontSize={12}
                  textAnchor={anchor}
                  fill="#37352f"
                  stroke="#ffffff"
                  strokeWidth={3.5}
                  paintOrder="stroke"
                  strokeLinejoin="round"
                >
                  {label}
                </text>
              </g>
            </g>
          );
        })}
        <g>
          <circle cx={cx} cy={cy} r={9} fill="#1a73d0" />
          <text
            x={cx}
            y={cy - 16}
            fontSize={12}
            textAnchor="middle"
            fill="#37352f"
            fontWeight={600}
            stroke="#ffffff"
            strokeWidth={3.5}
            paintOrder="stroke"
            strokeLinejoin="round"
          >
            {title[pageId] || "Diese Seite"}
          </text>
        </g>
      </svg>
    </div>
  );
}
