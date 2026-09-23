// The head above a list in the administration.
//
// Every list there answers the same three questions, and they used to stand
// differently in every place: how much is it (the number belongs on the
// heading, because an administrator counts it anyway as soon as they see the
// table), how do I find a row (a field beats paging from about twenty rows on),
// and what can I create here (a button, not a form above the list).
//
// As a component of its own and not as markup copied out six times: otherwise
// it is not a pattern but six places that drift apart.
import { ReactNode } from "react";

export default function Listenkopf({
  titel,
  zahl,
  filter,
  setFilter,
  platzhalter,
  children,
}: {
  titel: string;
  /** The number beside the heading. Left out where there is nothing to count. */
  zahl?: ReactNode;
  /** Together with setFilter: the filter field on the right. Both or neither. */
  filter?: string;
  setFilter?: (v: string) => void;
  platzhalter?: string;
  /** What stands to the right of the filter -- as a rule a button. */
  children?: ReactNode;
}) {
  return (
    <div className="listenkopf">
      <h3>
        {titel}
        {zahl !== undefined && <span className="muted small"> {zahl}</span>}
      </h3>
      <div className="listenkopf-werkzeug">
        {setFilter && (
          <input
            className="listenfilter"
            placeholder={platzhalter ?? "Filtern"}
            value={filter ?? ""}
            onChange={(e) => setFilter(e.target.value)}
          />
        )}
        {children}
      </div>
    </div>
  );
}
