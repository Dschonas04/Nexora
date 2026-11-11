// Nexora's mark: the N as a piece of the knowledge graph -- four pages, three
// links. Drawn in the text colour with the ground cut out, so it turns over with
// the theme by itself: black on light, white on dark.
export default function Zeichen({ groesse = 20 }: { groesse?: number }) {
  const grund = { fill: "var(--bg)" };
  return (
    <svg className="zeichen" width={groesse} height={groesse} viewBox="0 0 64 64" aria-hidden="true" focusable="false">
      <rect width="64" height="64" rx="15" fill="currentColor" />
      <path
        d="M20 44V20L44 44V20"
        style={{ fill: "none", stroke: "var(--bg)" }}
        strokeWidth={5}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="20" cy="44" r="5.5" style={grund} />
      <circle cx="20" cy="20" r="5.5" style={grund} />
      <circle cx="44" cy="44" r="5.5" style={grund} />
      <circle cx="44" cy="20" r="5.5" style={grund} />
    </svg>
  );
}
