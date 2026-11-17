import type { ReactElement } from "react";

// Small line icons for the page header. Drawn in the text colour, so they turn
// over with the theme by themselves, in the same weight as the rest of the UI.
type Name =
  | "unterseite"
  | "stern"
  | "auge"
  | "verlauf"
  | "teilen"
  | "loeschen"
  | "menue";

const pfade: Record<Name, ReactElement> = {
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
};

export default function Symbol({ name, gefuellt = false }: { name: Name; gefuellt?: boolean }) {
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
