import { PageMeta } from "../api/client";

export interface TreeDnD {
  dragId: string | null;
  dropTarget: string | null;
  onDragStartPage: (id: string) => void;
  onDragEndPage: () => void;
  onDragOverPage: (id: string) => void;
  onDragLeavePage: (id: string) => void;
  onDropPage: (page: PageMeta) => void;
  canDropOnPage: (id: string) => boolean;
}

interface Props {
  pages: PageMeta[];
  parentId: string | null;
  activeId?: string;
  expanded: Set<string>;
  onToggle: (id: string) => void;
  onSelect: (id: string) => void;
  onCreateChild: (parentId: string) => void;
  onDelete: (id: string) => void;
  dnd?: TreeDnD;
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
  dnd,
  depth = 0,
}: Props) {
  const children = pages.filter((p) => (p.parentId ?? null) === parentId);
  if (children.length === 0) return null;

  return (
    <>
      {children.map((p) => {
        const kids = pages.filter((c) => (c.parentId ?? null) === p.id);
        const isOpen = expanded.has(p.id);
        const isDropTarget = dnd?.dropTarget === p.id;
        const isDragging = dnd?.dragId === p.id;
        return (
          <div key={p.id}>
            <div
              className={
                "tree-row" +
                (activeId === p.id ? " active" : "") +
                (isDropTarget ? " drop-target" : "") +
                (isDragging ? " dragging" : "")
              }
              style={{ paddingLeft: 6 + depth * 14 }}
              onClick={() => onSelect(p.id)}
              draggable={!!dnd}
              onDragStart={(e) => {
                if (!dnd) return;
                e.dataTransfer.setData("text/plain", p.id);
                e.dataTransfer.effectAllowed = "move";
                dnd.onDragStartPage(p.id);
              }}
              onDragEnd={() => dnd?.onDragEndPage()}
              onDragOver={(e) => {
                if (!dnd || !dnd.dragId || dnd.dragId === p.id) return;
                if (!dnd.canDropOnPage(p.id)) return;
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
                dnd.onDragOverPage(p.id);
              }}
              onDragLeave={() => dnd?.onDragLeavePage(p.id)}
              onDrop={(e) => {
                if (!dnd) return;
                e.preventDefault();
                e.stopPropagation();
                dnd.onDropPage(p);
              }}
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
              <span className="tree-label">{p.title || "Ohne Titel"}</span>
              <span className="tree-actions">
                <button
                  className="icon-btn"
                  title="Unterseite hinzufügen"
                  onClick={(e) => {
                    e.stopPropagation();
                    onCreateChild(p.id);
                  }}
                >
                  +
                </button>
                <button
                  className="icon-btn"
                  title="Löschen"
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
                dnd={dnd}
                depth={depth + 1}
              />
            )}
          </div>
        );
      })}
    </>
  );
}
