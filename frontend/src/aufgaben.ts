// The marks that turn a checklist item into a task, shared by the editor (which
// highlights them) and the task list (which reads them). The service reads the
// same marks in backend/internal/handlers/aufgaben.go; the patterns have to
// agree with the ones there.

export const DATUM_RE = /📅\s*(\d{4}-\d{2}-\d{2}|\d{1,2}\.\d{1,2}\.\d{4})/g;
export const PERSON_RE = /(^|[\s(\[,;])@([A-Za-z0-9._-]{3,32})/g;

/** The date as YYYY-MM-DD, from either way of writing it, or null. */
export function isoDatum(roh: string): string | null {
  const iso = /^(\d{4})-(\d{2})-(\d{2})$/.exec(roh);
  if (iso) return roh;
  const de = /^(\d{1,2})\.(\d{1,2})\.(\d{4})$/.exec(roh);
  if (de) return `${de[3]}-${de[2].padStart(2, "0")}-${de[1].padStart(2, "0")}`;
  return null;
}

function heute(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** A date some days from today, as YYYY-MM-DD. */
export function inTagen(tage: number): string {
  const d = new Date();
  d.setDate(d.getDate() + tage);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** How a due date stands against today: the word to show and the class for its colour. */
export function faelligkeit(datum?: string | null): { text: string; art: string } {
  if (!datum) return { text: "", art: "" };
  const h = heute();
  if (datum < h) return { text: `overdue since ${anzeige(datum)}`, art: "ueberfaellig" };
  if (datum === h) return { text: "due today", art: "heute" };
  if (datum <= inTagen(7)) return { text: `due ${anzeige(datum)}`, art: "bald" };
  return { text: `due ${anzeige(datum)}`, art: "spaeter" };
}

function anzeige(iso: string): string {
  const [j, m, t] = iso.split("-").map(Number);
  return new Date(j, m - 1, t).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

/** The text without the date mark, for the list where the date has its own column. */
export function ohneMarken(text: string): string {
  return text.replace(DATUM_RE, "").replace(/\s{2,}/g, " ").trim();
}
