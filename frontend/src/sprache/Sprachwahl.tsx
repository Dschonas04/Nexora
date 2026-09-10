// The switch between German and English. translate="no", because the two names
// are always written in their own language.
import { api } from "../api/client";
import { useAuth } from "../auth";
import { setzeSprache, useSprache, type Sprache } from "./index";

const WAHL: { wert: Sprache; name: string }[] = [
  { wert: "de", name: "Deutsch" },
  { wert: "en", name: "English" },
];

export default function Sprachwahl() {
  const aktiv = useSprache();
  const { user } = useAuth();
  // Signed in, the choice goes to the account; before signing in it only holds
  // for this browser, until the account's own choice arrives with /design.
  const waehlen = (neu: Sprache) => {
    setzeSprache(neu);
    if (user) api.spracheSpeichern(neu).catch(() => {});
  };
  return (
    <div className="sprachwahl" role="radiogroup" aria-label="Language" translate="no">
      {WAHL.map((w) => (
        <button
          key={w.wert}
          type="button"
          role="radio"
          aria-checked={aktiv === w.wert}
          className={aktiv === w.wert ? "gewaehlt" : ""}
          onClick={() => waehlen(w.wert)}
        >
          {w.name}
        </button>
      ))}
    </div>
  );
}
