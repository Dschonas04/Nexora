// The avatar for an account, or the initials when no image is present.
import { useState } from "react";

/** Where the background tone for an initials badge comes from when no image exists. */
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

// Always the same tone for the same account. Deterministic from the id so a
// user is recognized by color across reloads.
function tonFuer(id: string): string {
  let summe = 0;
  for (let i = 0; i < id.length; i++) summe = (summe * 31 + id.charCodeAt(i)) % 100000;
  return TOENE[summe % TOENE.length];
}

/** Initials: from first and last name if available, otherwise the first letter. */
export function kuerzel(name: string, email = ""): string {
  const teile = name.trim().split(/\s+/).filter(Boolean);
  if (teile.length >= 2) return (teile[0][0] + teile[teile.length - 1][0]).toUpperCase();
  if (teile.length === 1) return teile[0].slice(0, 2).toUpperCase();
  return (email.trim()[0] ?? "?").toUpperCase();
}

/**
 * URL for the profile image. The `stand` parameter is used as a cache-buster
 * so a newly uploaded image appears immediately instead of the browser
 * showing a cached one.
 */
export function bildUrl(id: string, stand?: string | null): string {
  return `/api/users/${id}/bild?v=${encodeURIComponent(stand ?? "")}`;
}

interface Props {
  id: string;
  name: string;
  email?: string;
  /** If absent, there is no image and the initials are shown. */
  stand?: string | null;
  /** Edge length in pixels. */
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
  // If the image fails to load (for example it was deleted while the page
  // was open), fall back to the initials badge rather than showing a broken
  // image icon.
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
