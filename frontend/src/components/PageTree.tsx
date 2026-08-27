// The sidebar tree. It renders itself recursively, one level per call, from the
// flat page list the API returns: the backend never builds a nested structure,
// the shape comes purely from parentId.
import { PageMeta } from "../api/client";

// A gap between two rows: the place a page lands in when it is dropped there.
// vorId is the row below the gap, null for the gap at the end of a level.
// spaceId only matters at the top level of a section; below that the page takes
// the space of its parent.
export interface TreeGap {
  elternId: string | null;
  spaceId?: string | null;
  vorId: string | null;
}

export function gleicheLuecke(a: TreeGap | null, b: TreeGap | null): boolean {
  if (!a || !b) return false;
  return a.elternId === b.elternId && a.vorId === b.vorId && (a.spaceId ?? null) === (b.spaceId ?? null);
}

// Drag and drop state and callbacks, owned by the sidebar and threaded down.
// Keeping it in one object avoids passing eight separate props through every
// level of the recursion. Absent means the tree is not draggable, which is how
// the read-only lists reuse this component.
export interface TreeDnD {
  dragId: string | null;
  dropTarget: string | null;
  onDragStartPage: (id: string) => void;
  onDragEndPage: () => void;
  onDragOverPage: (id: string) => void;
  onDragLeavePage: (id: string) => void;
  onDropPage: (page: PageMeta) => void;
  canDropOnPage: (id: string) => boolean;
  // Dropping ONTO a row nests the page below it; dropping into a gap puts it
  // between two rows. Two gestures, one drag: without the gaps a page could
  // only ever be nested, never sorted.
  luecke: TreeGap | null;
  onDragOverLuecke: (l: TreeGap) => void;
  onDragLeaveLuecke: () => void;
  onDropLuecke: (l: TreeGap) => void;
}

interface Props {
  pages: PageMeta[];
  parentId: string | null;
  /** The space this level belongs to. Only read for the top level of a section:
      a page dropped into a gap there has to land in the right space. */
  spaceId?: string | null;
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
  spaceId = null,
  activeId,
  expanded,
  onToggle,
  onSelect,
  onCreateChild,
  onDelete,
  dnd,
  depth = 0,
}: Props) {
  // Filtering the whole list at every level is O(n) per node. That is fine at
  // the size a personal wiki reaches and keeps the component free of any
  // precomputed index that could fall out of sync with the list.
  const children = pages.filter((p) => (p.parentId ?? null) === parentId);
  if (children.length === 0) return null;

  // The strip between two rows. It is a few pixels high and only reacts while
  // something is being dragged; the rest of the time it is not in the way,
  // which is why it carries no padding of its own.
  const luecke = (vorId: string | null) => {
    if (!dnd) return null;
    const ziel: TreeGap = { elternId: parentId, spaceId, vorId };
    const aktiv = gleicheLuecke(dnd.luecke, ziel);
    return (
      <div
        className={"tree-luecke" + (aktiv ? " aktiv" : "") + (dnd.dragId ? " scharf" : "")}
        style={{ marginLeft: 6 + depth * 14 }}
        onDragOver={(e) => {
          if (!dnd.dragId) return;
          e.preventDefault();
          e.dataTransfer.dropEffect = "move";
          dnd.onDragOverLuecke(ziel);
        }}
        onDragLeave={() => dnd.onDragLeaveLuecke()}
        onDrop={(e) => {
          e.preventDefault();
          e.stopPropagation();
          dnd.onDropLuecke(ziel);
        }}
      />
    );
  };

  return (
    <>
      {children.map((p, i) => {
        const kids = pages.filter((c) => (c.parentId ?? null) === p.id);
        const isOpen = expanded.has(p.id);
        const isDropTarget = dnd?.dropTarget === p.id;
        const isDragging = dnd?.dragId === p.id;
        return (
          <div key={p.id}>
            {/* One gap above every row, and one more below the last: together
                they cover every place a page can go on this level. */}
            {luecke(p.id)}
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
                // canDropOnPage rejects a page's own subtree: dropping a parent
                // into its child would detach that branch from the tree.
                if (!dnd || !dnd.dragId || dnd.dragId === p.id) return;
                if (!dnd.canDropOnPage(p.id)) return;
                // Only preventDefault marks this element as a valid drop target;
                // without it the browser refuses the drop.
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
                dnd.onDragOverPage(p.id);
              }}
              onDragLeave={() => dnd?.onDragLeavePage(p.id)}
              onDrop={(e) => {
                if (!dnd) return;
                e.preventDefault();
                // Stop the event here, otherwise the enclosing rows would treat
                // the same drop as their own and move the page twice.
                e.stopPropagation();
                dnd.onDropPage(p);
              }}
            >
              {/* The caret only expands; stopping propagation keeps the click
                  from also selecting the page. */}
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
            {/* Recurse into the children, one level deeper. Collapsed branches
                are not rendered at all, so a large tree costs nothing while it
                is closed. */}
            {isOpen && (
              <PageTree
                pages={pages}
                parentId={p.id}
                spaceId={p.spaceId ?? null}
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
            {i === children.length - 1 && luecke(null)}
          </div>
        );
      })}
    </>
  );
}
