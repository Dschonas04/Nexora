// The switch between German and English. translate="no", because the two names
// are always written in their own language.
import { setzeSprache, useSprache, type Sprache } from "./index";

const WAHL: { wert: Sprache; name: string }[] = [
  { wert: "de", name: "Deutsch" },
  { wert: "en", name: "English" },
];

export default function Sprachwahl() {
  const aktiv = useSprache();
  return (
    <div className="sprachwahl" role="radiogroup" aria-label="Language" translate="no">
      {WAHL.map((w) => (
        <button
          key={w.wert}
          type="button"
          role="radio"
          aria-checked={aktiv === w.wert}
          className={aktiv === w.wert ? "gewaehlt" : ""}
          onClick={() => setzeSprache(w.wert)}
        >
          {w.name}
        </button>
      ))}
    </div>
  );
}
