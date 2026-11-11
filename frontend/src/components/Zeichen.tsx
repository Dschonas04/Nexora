// Nexora's mark: the N as a piece of the knowledge graph -- four pages, three
// links, each page in its own colour. Without a ground, so it reads on light
// and dark themes alike. Same drawing as public/favicon.svg.
export default function Zeichen({ groesse = 20 }: { groesse?: number }) {
  return (
    <svg className="zeichen" width={groesse} height={groesse} viewBox="128 128 768 768" aria-hidden="true" focusable="false">
      <defs>
        <linearGradient id="nx-kante" x1="0" y1="1" x2="1" y2="0">
          <stop offset="0" stopColor="#6366f1" />
          <stop offset="0.5" stopColor="#8b5cf6" />
          <stop offset="1" stopColor="#14b8a6" />
        </linearGradient>
        <radialGradient id="nx-k1" cx="0.35" cy="0.3" r="0.8"><stop offset="0" stopColor="#a5b4fc" /><stop offset="1" stopColor="#4f46e5" /></radialGradient>
        <radialGradient id="nx-k2" cx="0.35" cy="0.3" r="0.8"><stop offset="0" stopColor="#d8b4fe" /><stop offset="1" stopColor="#7c3aed" /></radialGradient>
        <radialGradient id="nx-k3" cx="0.35" cy="0.3" r="0.8"><stop offset="0" stopColor="#99f6e4" /><stop offset="1" stopColor="#0d9488" /></radialGradient>
        <radialGradient id="nx-k4" cx="0.35" cy="0.3" r="0.8"><stop offset="0" stopColor="#fde68a" /><stop offset="1" stopColor="#f59e0b" /></radialGradient>
      </defs>
      <path d="M300 724 V300 L724 724 V300" fill="none" stroke="url(#nx-kante)" strokeWidth={72} strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="300" cy="724" r="84" fill="url(#nx-k1)" />
      <circle cx="300" cy="300" r="84" fill="url(#nx-k2)" />
      <circle cx="724" cy="724" r="84" fill="url(#nx-k3)" />
      <circle cx="724" cy="300" r="84" fill="url(#nx-k4)" />
    </svg>
  );
}
