// Menüs, die aufgehen und wieder zugehen sollen.
//
// Die Klapplisten hingen bisher allein an onMouseLeave: sie schlossen erst,
// wenn der Zeiger einmal in ihnen war und wieder heraus. Wer das Menü öffnete
// und dann woanders hinklickte, ohne es zu berühren, hatte es weiter offen --
// über dem Seitenbaum, wo es die Klicks darunter abfing.
//
// Ein Klick daneben und die Escape-Taste sind die beiden Wege, die jeder
// erwartet, und sie kosten zusammen ein Dutzend Zeilen.
import { MutableRefObject, useEffect, useRef } from "react";

export function useAussenklick<T extends HTMLElement>(
  offen: boolean,
  schliessen: () => void,
): MutableRefObject<T | null> {
  const bereich = useRef<T | null>(null);

  useEffect(() => {
    if (!offen) return;

    // pointerdown und nicht click: der Klick käme erst beim Loslassen, und bis
    // dahin hat das, worauf man gezielt hat, den Zeiger schon bekommen.
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
