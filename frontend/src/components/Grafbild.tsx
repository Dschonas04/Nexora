// Das Grafbild: eine lebende Kräftesimulation, gezeichnet als SVG, zum
// Schieben, Zoomen und Ziehen.
//
// Es steht hier und nicht in der Graf-Ansicht, weil es zweimal gebraucht wird:
// einmal über dem ganzen Arbeitsbereich, einmal als kleiner Graf unter einer
// Seite. Der kleine war vorher ein starrer Stern aus fest gerechneten Winkeln
// -- dasselbe Bild, aber ohne alles, was den großen brauchbar macht.
//
// Die Simulation läuft vollständig außerhalb von React. Die Positionen liegen
// in Refs und werden in einer requestAnimationFrame-Schleife fortgeschrieben;
// React wird nur zum Neuzeichnen angestoßen, denn hunderte Koordinaten in den
// Zustand zu legen hieße, den ganzen Baum sechzigmal je Sekunde neu zu bauen.
import { useEffect, useMemo, useRef, useState } from "react";

import { Graph, GraphNode } from "../api/client";

// Farben je Ablage. Seiten ohne Ablage teilen sich die letzte, neutrale Farbe.
const ABLAGE_FARBEN = [
  "#2383e2", "#e2662c", "#159a6b", "#a84be0", "#d4356b",
  "#c99700", "#0f9bb0", "#7a52d6", "#5a8f3c", "#d05a2c",
];
const OHNE_ABLAGE_FARBE = "#9b9a97";

function ablageSchluessel(node: { spaceId: string | null }): string {
  return node.spaceId ?? "__none__";
}

// ablageFarben liefert die vorkommenden Ablagen in fester Reihenfolge und die
// Farbe dazu, damit ein Knoten die Farbe seiner Ablage tragen kann.
function ablageFarben(graph: Graph) {
  const schluessel: string[] = [];
  for (const n of graph.nodes) {
    const k = ablageSchluessel(n);
    if (!schluessel.includes(k)) schluessel.push(k);
  }
  const farbe: Record<string, string> = {};
  let i = 0;
  for (const k of schluessel) {
    farbe[k] = k === "__none__" ? OHNE_ABLAGE_FARBE : ABLAGE_FARBEN[i++ % ABLAGE_FARBEN.length];
  }
  return farbe;
}

// Ein simuliertes Teilchen je Seite.
interface Teilchen {
  x: number;
  y: number;
  vx: number;
  vy: number;
}

// Die Kräfte. Knoten stoßen sich ab, Kanten ziehen wie Federn -- die Hierarchie
// deutlich straffer als lose [[Verweise]] --, und eine milde Schwerkraft hält
// das Ganze in der Mitte.
export interface Physik {
  abstossung: number;
  federEltern: number;
  federVerweis: number;
  ruheEltern: number;
  ruheVerweis: number;
  schwerkraft: number;
  gleicheAblage: number;
}

// Für das große Bild über den ganzen Arbeitsbereich.
export const PHYSIK_GROSS: Physik = {
  abstossung: 13000,
  federEltern: 0.006,
  federVerweis: 0.0022,
  ruheEltern: 95,
  ruheVerweis: 210,
  schwerkraft: 0.014,
  gleicheAblage: 0.0016,
};

// Für den kleinen Graf unter einer Seite: weniger Knoten, weniger Fläche. Mit
// den Werten des großen Bildes flögen fünf Knoten in einem Kasten von 280
// Pixeln Höhe sofort an dessen Rand.
export const PHYSIK_KLEIN: Physik = {
  abstossung: 2600,
  federEltern: 0.010,
  federVerweis: 0.006,
  ruheEltern: 62,
  ruheVerweis: 105,
  schwerkraft: 0.035,
  gleicheAblage: 0,
};

const GESCHWINDIGKEIT_DAEMPFUNG = 0.82;
const ALPHA_ABFALL = 0.985;
const ALPHA_MIN = 0.02;
const ZUG_ALPHA = 0.35; // hält die Simulation warm, solange jemand zieht
const ZUG_SCHWELLE = 4; // Pixel, ab denen ein Druck als Ziehen gilt und nicht als Klick

// Beschriftung.
//
// Jeder Knoten trägt seinen Namen, also stehen die Namen einander im Weg. Nichts
// hiervon bewegt einen Knoten; nur die Beschriftung tritt zur Seite, und zwar
// nur auf einen von wenigen festen Plätzen um ihren eigenen Knoten herum. Eine
// Beschriftung, die frei wandert, ist schlimmer als eine, die überlappt: man
// sieht dann nicht mehr, welcher Name zu welchem Punkt gehört.
const LABEL_SCHRIFT = 12;
const LABEL_HOEHE = 14;
// Grobe Breite je Zeichen in dieser Größe. Richtig zu messen hieße, ein Canvas
// zu halten und je Name und Bild einmal zu messen; zum Ausweichen genügt eine
// Schätzung, die eher zu groß ausfällt -- zu breit heißt nur, dass eine
// Beschriftung etwas zu eifrig zur Seite tritt.
const LABEL_BREITE_JE_ZEICHEN = 6.4;

interface Kasten {
  x: number;
  y: number;
  w: number;
  h: number;
}

function ueberlappen(a: Kasten, b: Kasten): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
}

// Die Plätze, die eine Beschriftung einnehmen darf, nach Vorliebe geordnet:
// rechts vom Knoten zuerst, weil man sie dort sucht, dann links, dann darunter,
// dann darüber, und ganz zuletzt weiter darunter.
type Anker = "start" | "end" | "middle";
const LABEL_PLAETZE: { dx: (r: number) => number; dy: (r: number) => number; anker: Anker }[] = [
  { dx: (r) => r + 5, dy: () => 4, anker: "start" },
  { dx: (r) => -(r + 5), dy: () => 4, anker: "end" },
  { dx: () => 0, dy: (r) => r + 14, anker: "middle" },
  { dx: () => 0, dy: (r) => -(r + 8), anker: "middle" },
  { dx: () => 0, dy: (r) => r + 28, anker: "middle" },
];

// kastenFuer sagt, wo eine Beschriftung liegt, wenn sie einen dieser Plätze
// eingenommen hat.
function kastenFuer(x: number, y: number, breite: number, dx: number, dy: number, anker: Anker): Kasten {
  const links = anker === "start" ? x + dx : anker === "end" ? x + dx - breite : x + dx - breite / 2;
  return { x: links, y: y + dy - LABEL_HOEHE + 3, w: breite, h: LABEL_HOEHE };
}

interface Props {
  graph: Graph;
  /** Ein Klick auf einen Knoten, der kein Ziehen war. */
  onOeffnen: (id: string) => void;
  /** Diese Seite wird betont und in der Mitte gehalten -- der kleine Graf. */
  mitte?: string;
  physik?: Physik;
  /** Feste Höhe in Pixeln; ohne Angabe füllt das Bild seinen Kasten. */
  hoehe?: number;
  hinweis?: string;
  legende?: boolean;
  zentrieren?: boolean;
  /** Ob das Mausrad zoomt. In einer Seite nur mit Strg, sonst bliebe beim
      Scrollen der Text stehen und der Graf zöge sich zusammen. */
  radZoom?: "immer" | "mit-strg";
}

export default function Grafbild({
  graph,
  onOeffnen,
  mitte,
  physik = PHYSIK_GROSS,
  hoehe,
  hinweis,
  legende = false,
  zentrieren = false,
  radZoom = "immer",
}: Props) {
  const rahmenRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const [groesse, setGroesse] = useState({ w: 900, h: hoehe ?? 600 });
  const [blick, setBlick] = useState({ x: 0, y: 0, scale: 1 });
  const [zeiger, setZeiger] = useState<string | null>(null);
  const [, neuZeichnen] = useState(0);

  const groesseRef = useRef(groesse);
  groesseRef.current = groesse;
  const blickRef = useRef(blick);
  blickRef.current = blick;
  const physikRef = useRef(physik);
  physikRef.current = physik;
  const mitteRef = useRef(mitte);
  mitteRef.current = mitte;

  const farbe = ablageFarben(graph);

  // Abgeleitetes, das nur von der Knotenmenge abhängt: Grad (für die Größe),
  // Nachbarschaften (für das Hervorheben) und die Legende.
  const idsKey = graph.nodes.map((n) => n.id).sort().join(",");
  const abgeleitet = useMemo(() => {
    const grad: Record<string, number> = {};
    const nachbarn: Record<string, Set<string>> = {};
    for (const n of graph.nodes) {
      grad[n.id] = 0;
      nachbarn[n.id] = new Set();
    }
    for (const e of graph.edges) {
      if (grad[e.source] === undefined || grad[e.target] === undefined) continue;
      grad[e.source]++;
      grad[e.target]++;
      nachbarn[e.source].add(e.target);
      nachbarn[e.target].add(e.source);
    }
    const eintraege: { key: string; label: string; color: string }[] = [];
    const gesehen = new Set<string>();
    for (const n of graph.nodes) {
      const k = ablageSchluessel(n);
      if (gesehen.has(k)) continue;
      gesehen.add(k);
      eintraege.push({ key: k, label: k === "__none__" ? "Keine Ablage" : n.space || "Ablage", color: farbe[k] });
    }
    return { grad, nachbarn, legende: eintraege };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  // Der Knoten in der Mitte ist etwas größer: er ist der Grund, aus dem das
  // Bild überhaupt dasteht.
  const radius = (id: string) =>
    (id === mitte ? 3 : 0) + 6 + Math.min(7, Math.sqrt(abgeleitet.grad[id] || 0) * 2.2);

  const teilchen = useRef<Record<string, Teilchen>>({});
  const beschriftung = useRef<Record<string, { dx: number; dy: number; anker: Anker }>>({});
  const alpha = useRef(0);
  const raf = useRef<number | null>(null);
  const gehalten = useRef<{ id: string; x: number; y: number } | null>(null);
  const ablageVon = useRef<Record<string, string>>({});

  const zug = useRef<{
    art: "knoten" | "flaeche" | null;
    id?: string;
    offX: number;
    offY: number;
    startX: number;
    startY: number;
    panX: number;
    panY: number;
    bewegt: boolean;
  }>({ art: null, offX: 0, offY: 0, startX: 0, startY: 0, panX: 0, panY: 0, bewegt: false });

  useEffect(() => {
    const el = rahmenRef.current;
    if (!el) return;
    const messen = () => setGroesse({ w: el.clientWidth, h: hoehe ?? el.clientHeight });
    messen();
    const ro = new ResizeObserver(messen);
    ro.observe(el);
    return () => ro.disconnect();
  }, [hoehe]);

  // Ein Schritt der Simulation über die aktuelle Teilchenmenge.
  const schritt = () => {
    const nodes = graph.nodes;
    const p = teilchen.current;
    const a = alpha.current;
    const { w, h } = groesseRef.current;
    const ph = physikRef.current;

    // Abstoßung zwischen jedem Paar.
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const pa = p[nodes[i].id];
        const pb = p[nodes[j].id];
        if (!pa || !pb) continue;
        let dx = pa.x - pb.x;
        let dy = pa.y - pb.y;
        let l = dx * dx + dy * dy;
        if (l < 0.01) {
          dx = (Math.random() - 0.5) * 0.5;
          dy = (Math.random() - 0.5) * 0.5;
          l = dx * dx + dy * dy + 0.01;
        }
        const dist = Math.sqrt(l);
        const rep = (ph.abstossung * a) / l;
        const ux = dx / dist;
        const uy = dy / dist;
        pa.vx += ux * rep;
        pa.vy += uy * rep;
        pb.vx -= ux * rep;
        pb.vy -= uy * rep;
        // Lose Anziehung innerhalb einer Ablage: Ordner finden zusammen, ohne
        // festgenagelt zu sein.
        if (ph.gleicheAblage && ablageVon.current[nodes[i].id] === ablageVon.current[nodes[j].id]) {
          pa.vx -= ux * ph.gleicheAblage * dist * a;
          pa.vy -= uy * ph.gleicheAblage * dist * a;
          pb.vx += ux * ph.gleicheAblage * dist * a;
          pb.vy += uy * ph.gleicheAblage * dist * a;
        }
      }
    }

    // Federn an den Kanten -- Hierarchie kurz und straff, Verweise lang und weich.
    for (const e of graph.edges) {
      const ps = p[e.source];
      const pt = p[e.target];
      if (!ps || !pt) continue;
      const dx = pt.x - ps.x;
      const dy = pt.y - ps.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const eltern = e.kind === "parent";
      const ruhe = eltern ? ph.ruheEltern : ph.ruheVerweis;
      const k = eltern ? ph.federEltern : ph.federVerweis;
      const spr = k * (dist - ruhe) * a;
      const ux = dx / dist;
      const uy = dy / dist;
      ps.vx += ux * spr;
      ps.vy += uy * spr;
      pt.vx -= ux * spr;
      pt.vy -= uy * spr;
    }

    // Schwerkraft zur Mitte der Fläche. Der betonte Knoten wird stärker
    // gehalten: im kleinen Graf soll die Seite, um die es geht, auch in der
    // Mitte stehen -- ziehen lässt sie sich trotzdem.
    for (const n of nodes) {
      const pp = p[n.id];
      if (!pp) continue;
      const stark = n.id === mitteRef.current ? 6 : 1;
      pp.vx += (w / 2 - pp.x) * ph.schwerkraft * stark * a;
      pp.vy += (h / 2 - pp.y) * ph.schwerkraft * stark * a;
    }

    // Fortschreiben.
    for (const n of nodes) {
      const pp = p[n.id];
      if (!pp) continue;
      const f = gehalten.current;
      if (f && f.id === n.id) {
        pp.x = f.x;
        pp.y = f.y;
        pp.vx = 0;
        pp.vy = 0;
        continue;
      }
      pp.vx *= GESCHWINDIGKEIT_DAEMPFUNG;
      pp.vy *= GESCHWINDIGKEIT_DAEMPFUNG;
      pp.x += pp.vx;
      pp.y += pp.vy;
    }

    // Das Ganze mittig halten: alle Knoten so verschieben, dass ihr Schwerpunkt
    // in der Mitte liegt. Beim Ziehen ausgesetzt, damit der gegriffene Knoten
    // unter dem Zeiger bleibt.
    if (!gehalten.current) {
      let cx = 0;
      let cy = 0;
      for (const n of nodes) {
        const pp = p[n.id];
        if (pp) {
          cx += pp.x;
          cy += pp.y;
        }
      }
      const anzahl = nodes.length || 1;
      const schiebeX = w / 2 - cx / anzahl;
      const schiebeY = h / 2 - cy / anzahl;
      for (const n of nodes) {
        const pp = p[n.id];
        if (pp) {
          pp.x += schiebeX;
          pp.y += schiebeY;
        }
      }
    }

    beschriftungenVerteilen();
  };

  // Jeder Beschriftung einen Platz geben, an dem sie so wenig wie möglich
  // verdeckt.
  //
  // Gierig und in einem Durchgang: die Knoten mit den meisten Kanten kommen
  // zuerst und behalten den guten Platz rechts, alles andere weicht um sie
  // herum aus. Ein richtiges Optimum wäre weder den Code noch die Millisekunden
  // wert -- es wird ohnehin in jedem Bild neu gerechnet, und ein Bild später
  // steht alles woanders.
  const beschriftungenVerteilen = () => {
    const p = teilchen.current;
    const reihe = [...graph.nodes].sort(
      (a, b) => (abgeleitet.grad[b.id] || 0) - (abgeleitet.grad[a.id] || 0),
    );
    const belegt: Kasten[] = [];
    for (const n of reihe) {
      const pp = p[n.id];
      if (!pp) continue;
      const r = radius(n.id);
      belegt.push({ x: pp.x - r, y: pp.y - r, w: r * 2, h: r * 2 });
    }
    const gewaehlt: Record<string, { dx: number; dy: number; anker: Anker }> = {};
    for (const n of reihe) {
      const pp = p[n.id];
      if (!pp) continue;
      const r = radius(n.id);
      const breite = (n.title || "Ohne Titel").length * LABEL_BREITE_JE_ZEICHEN;
      let platz = LABEL_PLAETZE[0];
      let kasten = kastenFuer(pp.x, pp.y, breite, platz.dx(r), platz.dy(r), platz.anker);
      for (const k of LABEL_PLAETZE) {
        const versuch = kastenFuer(pp.x, pp.y, breite, k.dx(r), k.dy(r), k.anker);
        if (!belegt.some((b) => ueberlappen(b, versuch))) {
          platz = k;
          kasten = versuch;
          break;
        }
      }
      // So oder so belegt: auch der letzte Ausweg sperrt den Platz für den
      // Nächsten, sonst lägen zwei Beschriftungen, die beide aufgeben mussten,
      // übereinander.
      belegt.push(kasten);
      gewaehlt[n.id] = { dx: platz.dx(r), dy: platz.dy(r), anker: platz.anker };
    }
    beschriftung.current = gewaehlt;
  };

  const schleife = () => {
    schritt();
    alpha.current *= ALPHA_ABFALL;
    neuZeichnen((v) => v + 1);
    if (alpha.current > ALPHA_MIN || gehalten.current) {
      raf.current = requestAnimationFrame(schleife);
    } else {
      raf.current = null;
    }
  };

  const anheizen = (ziel = 1) => {
    alpha.current = Math.max(alpha.current, ziel);
    if (raf.current == null) raf.current = requestAnimationFrame(schleife);
  };

  // Teilchen neu setzen, sobald sich die Menge der Seiten ändert, dann laufen
  // lassen. Bekannte Knoten behalten ihre Stelle über einen Datenwechsel hinweg.
  useEffect(() => {
    const { w, h } = groesseRef.current;
    const naechste: Record<string, Teilchen> = {};
    const ablagen: Record<string, string> = {};
    const n = graph.nodes.length || 1;
    graph.nodes.forEach((node, i) => {
      ablagen[node.id] = ablageSchluessel(node);
      const vorher = teilchen.current[node.id];
      if (vorher) {
        naechste[node.id] = vorher;
      } else if (node.id === mitte) {
        naechste[node.id] = { x: w / 2, y: h / 2, vx: 0, vy: 0 };
      } else {
        const ang = (i / n) * Math.PI * 2;
        naechste[node.id] = {
          x: w / 2 + Math.cos(ang) * (Math.min(w, h) / 3.5),
          y: h / 2 + Math.sin(ang) * (Math.min(w, h) / 3.5),
          vx: 0,
          vy: 0,
        };
      }
    });
    teilchen.current = naechste;
    ablageVon.current = ablagen;
    if (graph.nodes.length) anheizen(1);
    return () => {
      if (raf.current != null) cancelAnimationFrame(raf.current);
      raf.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  // Bildschirm- in Simulationskoordinaten, Verschiebung und Zoom
  // herausgerechnet. Jeder Zeigerhandler braucht das, denn die Physik weiß
  // nichts vom Ausschnitt.
  const zuGraf = (clientX: number, clientY: number) => {
    const rect = svgRef.current!.getBoundingClientRect();
    const v = blickRef.current;
    return { x: (clientX - rect.left - v.x) / v.scale, y: (clientY - rect.top - v.y) / v.scale };
  };

  // Ein Druck greift entweder einen Knoten oder schiebt die Fläche -- je
  // nachdem, worauf er landet. Der Zeiger wird eingefangen, damit ein schneller
  // Zug, der die Fläche verlässt, weiter verfolgt wird.
  const zeigerAb = (e: React.PointerEvent) => {
    const treffer = (e.target as Element).closest?.("[data-node]") as Element | null;
    svgRef.current?.setPointerCapture(e.pointerId);
    if (treffer) {
      const id = treffer.getAttribute("data-node")!;
      const g = zuGraf(e.clientX, e.clientY);
      const p = teilchen.current[id] ?? { x: g.x, y: g.y };
      zug.current = {
        art: "knoten",
        id,
        offX: g.x - p.x,
        offY: g.y - p.y,
        startX: e.clientX,
        startY: e.clientY,
        panX: 0,
        panY: 0,
        bewegt: false,
      };
      gehalten.current = { id, x: p.x, y: p.y };
      anheizen(ZUG_ALPHA);
    } else {
      zug.current = {
        art: "flaeche",
        offX: 0,
        offY: 0,
        startX: e.clientX,
        startY: e.clientY,
        panX: blick.x,
        panY: blick.y,
        bewegt: false,
      };
    }
  };

  // Ein gezogener Knoten hängt am Zeiger, die Simulation bleibt warm, damit
  // seine Nachbarn mitkommen statt zu erstarren.
  const zeigerBewegt = (e: React.PointerEvent) => {
    const d = zug.current;
    if (!d.art) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    if (!d.bewegt && Math.hypot(dx, dy) > ZUG_SCHWELLE) d.bewegt = true;
    if (d.art === "knoten" && d.id) {
      const g = zuGraf(e.clientX, e.clientY);
      gehalten.current = { id: d.id, x: g.x - d.offX, y: g.y - d.offY };
      anheizen(ZUG_ALPHA);
    } else if (d.art === "flaeche") {
      setBlick((v) => ({ ...v, x: d.panX + dx, y: d.panY + dy }));
    }
  };

  // Loslassen. Ein Druck, der die Schwelle nie überschritten hat, gilt als
  // Klick und öffnet die Seite -- ein gezogener Knoten führt also nicht
  // versehentlich woanders hin.
  const zeigerAuf = (e: React.PointerEvent) => {
    const d = zug.current;
    svgRef.current?.releasePointerCapture(e.pointerId);
    if (d.art === "knoten" && d.id && !d.bewegt) onOeffnen(d.id);
    gehalten.current = null;
    anheizen(0.15);
    zug.current.art = null;
  };

  // Zum Zeiger hin zoomen und nicht zur Mitte: der Punkt unter dem Zeiger muss
  // stehen bleiben, dafür die Umrechnung. Der Maßstab ist begrenzt, damit der
  // Graf weder verschwindet noch als einzelner Knoten das Fenster füllt.
  const amRad = (e: React.WheelEvent) => {
    if (radZoom === "mit-strg" && !e.ctrlKey && !e.metaKey) return;
    e.preventDefault();
    const faktor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    const rect = svgRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    setBlick((v) => {
      const scale = Math.max(0.3, Math.min(3, v.scale * faktor));
      return { scale, x: mx - ((mx - v.x) / v.scale) * scale, y: my - ((my - v.y) / v.scale) * scale };
    });
  };

  // Zurücksetzen: Ausschnitt zurück auf Anfang und der Simulation einen frischen
  // Stoß, damit sie sich neu in der Mitte setzt.
  const zurueck = () => {
    setBlick({ x: 0, y: 0, scale: 1 });
    if (graph.nodes.length) anheizen(1);
  };

  const p = teilchen.current;
  const deckkraft = (id: string) => {
    if (!zeiger) return 1;
    if (id === zeiger || abgeleitet.nachbarn[zeiger]?.has(id)) return 1;
    return 0.15;
  };

  // Jeder Knoten trägt seinen Namen, in jeder Vergrößerung. Sie erst beim
  // Hineinzoomen zu zeigen hielt das Bild aufgeräumt und machte es nutzlos: ein
  // Graf aus namenlosen Punkten sagt nichts darüber, was womit zusammenhängt.
  return (
    <div className="graph-wrap" ref={rahmenRef} style={hoehe ? { height: hoehe } : undefined}>
      {hinweis && <div className="graph-hint">{hinweis}</div>}
      {legende && abgeleitet.legende.length > 1 && (
        <div className="graph-legend">
          {abgeleitet.legende.map((l) => (
            <span key={l.key} className="graph-legend-item">
              <span className="graph-legend-dot" style={{ background: l.color }} />
              {l.label}
            </span>
          ))}
        </div>
      )}
      {zentrieren && (
        <button className="btn graph-reset" onClick={zurueck}>
          Zentrieren
        </button>
      )}
      <svg
        ref={svgRef}
        width={groesse.w}
        height={groesse.h}
        className="graph-svg"
        onPointerDown={zeigerAb}
        onPointerMove={zeigerBewegt}
        onPointerUp={zeigerAuf}
        onWheel={amRad}
      >
        <g transform={`translate(${blick.x}, ${blick.y}) scale(${blick.scale})`}>
          {graph.edges.map((kante, i) => {
            const a = p[kante.source];
            const b = p[kante.target];
            if (!a || !b) return null;
            const hell = zeiger && (kante.source === zeiger || kante.target === zeiger);
            const blass = zeiger && !hell;
            return (
              <line
                key={i}
                x1={a.x}
                y1={a.y}
                x2={b.x}
                y2={b.y}
                // Die Farben stehen im Stilblatt, nicht hier: als Attribut wären
                // sie an einen Grundton gebunden, und im dunklen stünden helle
                // Linien auf hellem Text.
                className={
                  "graph-kante" +
                  (kante.kind === "parent" ? " eltern" : "") +
                  (hell ? " betont" : "")
                }
                strokeWidth={kante.kind === "parent" ? 1.6 : 1}
                strokeDasharray={kante.kind === "link" ? "4 3" : undefined}
                opacity={blass ? 0.12 : 1}
              />
            );
          })}
          {graph.nodes.map((node: GraphNode) => {
            const pp = p[node.id];
            if (!pp) return null;
            const r = radius(node.id);
            const platz = beschriftung.current[node.id] ?? { dx: r + 5, dy: 4, anker: "start" as Anker };
            const betont = zeiger === node.id || node.id === mitte;
            return (
              <g
                key={node.id}
                data-node={node.id}
                transform={`translate(${pp.x}, ${pp.y})`}
                className="graph-node"
                opacity={deckkraft(node.id)}
                onPointerEnter={() => setZeiger(node.id)}
                onPointerLeave={() => setZeiger(null)}
              >
                <circle
                  r={zeiger === node.id ? r + 2 : r}
                  fill={farbe[ablageSchluessel(node)] ?? "#2383e2"}
                  // Der Rand ist der Grund der Seite, damit ein Knoten sich von
                  // den Linien dahinter abhebt -- hell im Hellen, dunkel im
                  // Dunklen.
                  className={"graph-knoten" + (betont ? " betont" : "")}
                  strokeWidth={betont ? 2 : 1}
                />
                {/* Der Saum um die Buchstaben hält einen Namen dort lesbar, wo
                    er eine Linie kreuzt -- das Ausweichen regelt die
                    Beschriftungen untereinander, nicht die Linien. */}
                <text
                  x={platz.dx}
                  y={platz.dy}
                  textAnchor={platz.anker}
                  fontSize={LABEL_SCHRIFT}
                  className="graph-name"
                  fontWeight={node.id === mitte ? 600 : undefined}
                  strokeWidth={3.5}
                  paintOrder="stroke"
                  strokeLinejoin="round"
                >
                  {node.title || "Ohne Titel"}
                </text>
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}
