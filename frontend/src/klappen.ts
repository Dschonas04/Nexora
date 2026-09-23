// Menus that are meant to open and close again.
//
// The dropdowns used to hang on onMouseLeave alone: they only closed once the
// pointer had been inside them and out again. Whoever opened the menu and then
// clicked somewhere else without touching it still had it open -- above the
// page tree, where it caught the clicks underneath.
//
// A click beside it and the Escape key are the two ways everybody expects, and
// together they cost a dozen lines.
import { MutableRefObject, useEffect, useRef } from "react";

export function useAussenklick<T extends HTMLElement>(
  offen: boolean,
  schliessen: () => void,
): MutableRefObject<T | null> {
  const bereich = useRef<T | null>(null);

  useEffect(() => {
    if (!offen) return;

    // pointerdown and not click: the click would only come on release, and by
    // then whatever one aimed at has already got the pointer.
    const daneben = (e: PointerEvent) => {
      const ziel = e.target as Node | null;
      if (ziel && bereich.current?.contains(ziel)) return;
      schliessen();
    };
    const taste = (e: KeyboardEvent) => {
      if (e.key === "Escape") schliessen();
    };

    document.addEventListener("pointerdown", daneben);
    document.addEventListener("keydown", taste);
    return () => {
      document.removeEventListener("pointerdown", daneben);
      document.removeEventListener("keydown", taste);
    };
  }, [offen, schliessen]);

  return bereich;
}
