import { PageMeta } from "../api/client";

interface Props {
  pages: PageMeta[];
  parentId: string | null;
  activeId?: string;
  expanded: Set<string>;
  onToggle: (id: string) => void;
  onSelect: (id: string) => void;
  onCreateChild: (parentId: string) => void;
  onDelete: (id: string) => void;
  depth?: number;
}

export default function PageTree({
  pages,
  parentId,
  activeId,
  expanded,
  onToggle,
  onSelect,
  onCreateChild,
  onDelete,
  depth = 0,
}: Props) {
  const children = pages.filter((p) => (p.parentId ?? null) === parentId);
  if (children.length === 0) return null;

  return (
    <>
      {children.map((p) => {
        const kids = pages.filter((c) => (c.parentId ?? null) === p.id);
        const isOpen = expanded.has(p.id);
        return (
          <div key={p.id}>
            <div
              className={"tree-row" + (activeId === p.id ? " active" : "")}
              style={{ paddingLeft: 6 + depth * 14 }}
              onClick={() => onSelect(p.id)}
            >
              <span
                className="tree-caret"
                onClick={(e) => {
                  e.stopPropagation();
                  if (kids.length) onToggle(p.id);
                }}
              >
                {kids.length ? (isOpen ? "▾" : "▸") : ""}
              </span>
              <span className="tree-label">{p.title || "Untitled"}</span>
              <span className="tree-actions">
                <button
                  className="icon-btn"
                  title="Add subpage"
                  onClick={(e) => {
                    e.stopPropagation();
                    onCreateChild(p.id);
                  }}
                >
                  +
                </button>
                <button
                  className="icon-btn"
                  title="Delete"
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete(p.id);
                  }}
                >
                  ✕
                </button>
              </span>
            </div>
            {isOpen && (
              <PageTree
                pages={pages}
                parentId={p.id}
                activeId={activeId}
                expanded={expanded}
                onToggle={onToggle}
                onSelect={onSelect}
                onCreateChild={onCreateChild}
                onDelete={onDelete}
                depth={depth + 1}
              />
            )}
          </div>
        );
      })}
    </>
  );
}
