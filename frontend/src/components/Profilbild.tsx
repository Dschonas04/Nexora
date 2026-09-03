// Das Gesicht an einem Konto -- oder, solange keines da ist, die
// Anfangsbuchstaben.
//
// Eine eigene Datei, weil das an mehreren Stellen erscheint: unten in der
// Leiste, im Profilfenster, und überall dort, wo künftig ein Name mit einem
// Gesicht daneben stehen soll. Ein Bild, das an drei Stellen anders aussieht,
// sieht nach drei verschiedenen Programmen aus.
import { useState } from "react";

/** Woher die Farbe eines Kürzels kommt, wenn kein Bild da ist. */
const TOENE = [
  "#c2410c",
  "#b45309",
  "#15803d",
  "#0f766e",
  "#1d4ed8",
  "#6d28d9",
  "#a21caf",
  "#be123c",
];

// Immer dieselbe Farbe für dasselbe Konto. Nicht zufällig, sondern aus der
// Kennung gerechnet: sonst wechselte ein Gesicht bei jedem Laden die Farbe, und
// gerade daran erkennt man jemanden in einer Liste wieder.
function tonFuer(id: string): string {
  let summe = 0;
  for (let i = 0; i < id.length; i++) summe = (summe * 31 + id.charCodeAt(i)) % 100000;
  return TOENE[summe % TOENE.length];
}

/** Die Anfangsbuchstaben: aus Vor- und Nachnamen, sonst der erste Buchstabe. */
export function kuerzel(name: string, email = ""): string {
  const teile = name.trim().split(/\s+/).filter(Boolean);
  if (teile.length >= 2) return (teile[0][0] + teile[teile.length - 1][0]).toUpperCase();
  if (teile.length === 1) return teile[0].slice(0, 2).toUpperCase();
  return (email.trim()[0] ?? "?").toUpperCase();
}

/**
 * Die Adresse des Bildes. Der Stand hängt daran, damit ein neues Bild sofort
 * erscheint: der Browser hebt das alte einen Tag lang auf, und ohne eine Zahl,
 * die sich mitbewegt, sähe man sein eigenes neues Gesicht erst morgen.
 */
export function bildUrl(id: string, stand?: string | null): string {
  return `/api/users/${id}/bild?v=${encodeURIComponent(stand ?? "")}`;
}

interface Props {
  id: string;
  name: string;
  email?: string;
  /** Fehlt er, gibt es kein Bild und es bleibt beim Kürzel. */
  stand?: string | null;
  /** Kantenlänge in Pixeln. */
  groesse?: number;
  className?: string;
}

export default function Profilbild({
  id,
  name,
  email = "",
  stand,
  groesse = 28,
  className = "",
}: Props) {
  // Ein Bild, das nicht lädt -- gelöscht, während die Seite offen stand --,
  // fällt auf das Kürzel zurück statt ein zerbrochenes Sinnbild zu zeigen.
  const [gescheitert, setGescheitert] = useState(false);

  const stil = {
    width: groesse,
    height: groesse,
    fontSize: Math.max(10, Math.round(groesse * 0.4)),
  };

  if (stand && !gescheitert) {
    return (
      <img
        className={"profilbild " + className}
        style={stil}
        src={bildUrl(id, stand)}
        alt={name}
        title={name}
        onError={() => setGescheitert(true)}
      />
    );
  }
  return (
    <span
      className={"profilbild profilbild-kuerzel " + className}
      style={{ ...stil, background: tonFuer(id) }}
      title={name}
      aria-hidden="true"
    >
      {kuerzel(name, email)}
    </span>
  );
}
