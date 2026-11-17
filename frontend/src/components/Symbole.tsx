import type { ReactElement } from "react";

// Small line icons for the page header. Drawn in the text colour, so they turn
// over with the theme by themselves, in the same weight as the rest of the UI.
export type SymbolName =
  | "unterseite"
  | "stern"
  | "auge"
  | "verlauf"
  | "teilen"
  | "loeschen"
  | "menue"
  | "plus"
  | "leiste"
  | "person"
  | "palette"
  | "sprache"
  | "glocke"
  | "schloss"
  | "schild"
  | "geraet"
  | "schluessel";

const pfade: Record<SymbolName, ReactElement> = {
  unterseite: (
    <>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
      <path d="M14 3v5h5M12 11v6M9 14h6" />
    </>
  ),
  stern: (
    <path d="M12 3.5l2.6 5.3 5.9.9-4.25 4.1 1 5.8L12 16.9l-5.25 2.7 1-5.8L3.5 9.7l5.9-.9z" />
  ),
  auge: (
    <>
      <path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  verlauf: (
    <>
      <path d="M3.5 12a8.5 8.5 0 1 0 2.5-6" />
      <path d="M3.5 4v4.5H8M12 7.5V12l3 2" />
    </>
  ),
  teilen: (
    <>
      <circle cx="18" cy="5.5" r="2.5" />
      <circle cx="6" cy="12" r="2.5" />
      <circle cx="18" cy="18.5" r="2.5" />
      <path d="M8.2 10.9l7.6-4.3M8.2 13.1l7.6 4.3" />
    </>
  ),
  loeschen: (
    <>
      <path d="M4 6.5h16M9.5 6.5V4h5v2.5M6 6.5l1 13a1.5 1.5 0 0 0 1.5 1.5h7a1.5 1.5 0 0 0 1.5-1.5l1-13" />
      <path d="M10 10.5v6M14 10.5v6" />
    </>
  ),
  menue: <path d="M4 7h16M4 12h16M4 17h16" />,
  plus: <path d="M12 5v14M5 12h14" />,
  person: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4.5 20.5c1-4 4-6 7.5-6s6.5 2 7.5 6" />
    </>
  ),
  palette: (
    <>
      <path d="M12 3.5a8.5 8.5 0 1 0 0 17c1.4 0 2-1 1.4-2.2-.7-1.3.1-2.8 1.6-2.8h2c2 0 3.5-1.6 3.5-3.6C20.5 7.3 16.7 3.5 12 3.5z" />
      <circle cx="7.5" cy="11" r="1.2" />
      <circle cx="10.5" cy="7.5" r="1.2" />
      <circle cx="15" cy="7.8" r="1.2" />
    </>
  ),
  sprache: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M3.5 12h17M12 3.5c2.5 2.6 3.5 5.4 3.5 8.5s-1 5.9-3.5 8.5c-2.5-2.6-3.5-5.4-3.5-8.5s1-5.9 3.5-8.5z" />
    </>
  ),
  glocke: (
    <>
      <path d="M6 16.5V11a6 6 0 0 1 12 0v5.5l1.5 2h-15z" />
      <path d="M10 20.5a2 2 0 0 0 4 0" />
    </>
  ),
  schloss: (
    <>
      <rect x="5" y="10.5" width="14" height="10" rx="2" />
      <path d="M8 10.5V7.5a4 4 0 0 1 8 0v3" />
    </>
  ),
  schild: (
    <>
      <path d="M12 3l7.5 3v5.5c0 4.5-3.2 8-7.5 9.5-4.3-1.5-7.5-5-7.5-9.5V6z" />
      <path d="M9 12l2 2 4-4" />
    </>
  ),
  geraet: (
    <>
      <rect x="3" y="4.5" width="18" height="12" rx="1.5" />
      <path d="M9 20h6M12 16.5V20" />
    </>
  ),
  schluessel: (
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="M11 12l8.5-8.5M16 7l2.5 2.5M14 9l2 2" />
    </>
  ),
  leiste: (
    <>
      <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
      <path d="M9.5 4.5v15M15.5 10l-2 2 2 2" />
    </>
  ),
};

export default function Symbol({ name, gefuellt = false }: { name: SymbolName; gefuellt?: boolean }) {
  return (
    <svg
      className="symbol"
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill={gefuellt ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {pfade[name]}
    </svg>
  );
}
