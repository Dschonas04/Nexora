// Which color a collection ("Ablage") uses.
//
// Kept in a separate file because the same question is asked in two places:
// the sidebar shows a dot before each name and the graph colors each node by
// its collection. Having two lists would produce two answers and the same
// collection could appear blue in one place and orange in another.
//
// The sequence provides default colors only; when a user chooses a color it
// is applied everywhere.

/** The sequence used to pick a collection's default color when none is chosen. */
export const ABLAGE_FARBEN = [
  "#2383e2",
  "#e2662c",
  "#159a6b",
  "#a84be0",
  "#d4356b",
  "#c99700",
  "#0f9bb0",
  "#7a52d6",
  "#5a8f3c",
  "#d05a2c",
];

/** Color for pages without a collection. Gray represents the absence of a collection. */
export const OHNE_ABLAGE_FARBE = "#9b9a97";

/**
 * ausReihe selects a color from the sequence based on the id.
 *
 * The color is computed from the id rather than assigned by counting. A
 * counted assignment would shift colors when collections are added or removed,
 * making the same view look different after a filter or when a new empty
 * collection appears. Computing keeps a collection's color stable while it
 * exists.
 */
function ausReihe(id: string): string {
  let summe = 0;
  for (let i = 0; i < id.length; i++) summe = (summe * 31 + id.charCodeAt(i)) % 1000003;
  return ABLAGE_FARBEN[summe % ABLAGE_FARBEN.length];
}

/**
 * farbeFuerAblage returns a collection's color: the chosen color if present,
 * otherwise the color computed from the sequence. An empty id represents
 * "no collection".
 */
export function farbeFuerAblage(id: string | null | undefined, eigene?: string | null): string {
  if (eigene) return eigene;
  if (!id || id === "__none__") return OHNE_ABLAGE_FARBE;
  return ausReihe(id);
}
