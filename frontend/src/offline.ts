// The browser side of working offline; the work itself happens in the service
// worker (public/sw.js). This module asks it to check for the server every
// half minute, to keep the mirror fresh in the app, and passes on what it
// reports: whether the server is away, how many changes wait, and when a sync
// went through.

export interface OfflineZustand {
  offline: boolean;
  wartend: number;
  /** Changes of another account wait on this device. */
  fremd: boolean;
}

let zustand: OfflineZustand = { offline: false, wartend: 0, fremd: false };
const hoerer = new Set<(z: OfflineZustand) => void>();

export const offlineZustand = () => zustand;

export function aufOffline(f: (z: OfflineZustand) => void): () => void {
  hoerer.add(f);
  return () => {
    hoerer.delete(f);
  };
}

function setzen(neu: Partial<OfflineZustand>) {
  zustand = { ...zustand, ...neu };
  hoerer.forEach((f) => f(zustand));
}

export const ABGLEICH_SCHLUESSEL = "nexora-abgeglichen";

/**
 * Whether this window keeps a mirror of all pages. Only in the app (Android,
 * or installed from the browser); a plain browser tab keeps what was opened.
 * The desktop app keeps its own mirror outside the browser.
 */
function spiegelt(): boolean {
  if (/Electron/.test(navigator.userAgent)) return false;
  return /; wv\)/.test(navigator.userAgent) || window.matchMedia?.("(display-mode: standalone)").matches;
}

export function offlineStarten() {
  const sw = navigator.serviceWorker;
  if (!sw) return;
  sw.addEventListener("message", (e: MessageEvent) => {
    const d = e.data;
    if (!d || typeof d !== "object") return;
    if (d.art === "nexora-offline") setzen({ offline: !!d.offline, wartend: Number(d.wartend) || 0 });
    else if (d.art === "nexora-fremd") setzen({ fremd: true });
    else if (d.art === "nexora-abgeglichen") {
      try {
        sessionStorage.setItem(ABGLEICH_SCHLUESSEL, JSON.stringify(d));
      } catch {
        /* then without the note */
      }
      // The editor still holds the state from before the sync; reload, and
      // follow a page that got its real id from the server.
      let pfad = location.pathname;
      for (const [alt, neu] of Object.entries((d.neueKennungen as Record<string, string>) || {})) {
        pfad = pfad.replace(alt, neu);
      }
      if (pfad !== location.pathname) location.replace(pfad + location.search);
      else location.reload();
    }
  });
  const senden = (m: string) => sw.controller?.postMessage(m);
  void sw.ready.then(() => {
    senden("zustand");
    window.setTimeout(() => {
      senden("pruefen");
      if (spiegelt()) senden("spiegeln");
    }, 3000);
  });
  window.setInterval(() => senden("pruefen"), 30_000);
  if (spiegelt()) {
    window.setInterval(() => {
      if (!zustand.offline && document.visibilityState === "visible") senden("spiegeln");
    }, 120_000);
  }
  window.addEventListener("online", () => senden("pruefen"));
}
