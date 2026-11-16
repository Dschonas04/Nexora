// Starting points for the "/" menu.
//
// One entry used to insert a flowchart and nothing else, so whoever did not
// already know Mermaid never found out that it also draws pies, bars, mind
// maps or sequences. Each template is short and shows the one or two
// constructs that kind of diagram is made of; the dialog takes it from there.
//
// The texts are in the menu language (de, en, fr) because they end up as page
// content, where the interface translation does not reach.

export interface Diagrammvorlage {
  titel: [string, string, string];
  unter: [string, string, string];
  aliases: string[];
  code: [string, string, string];
}

export const DIAGRAMMVORLAGEN: Diagrammvorlage[] = [
  {
    titel: ["Ablaufdiagramm", "Flowchart", "Organigramme"],
    unter: ["Schritte und Entscheidungen", "Steps and decisions", "Étapes et décisions"],
    aliases: ["diagramm", "diagram", "mermaid", "flowchart", "ablauf", "flussdiagramm"],
    code: [
      "flowchart LR\n  A[Idee] --> B{Passt?}\n  B -- ja --> C[Umsetzen]\n  B -- nein --> A",
      "flowchart LR\n  A[Idea] --> B{Fits?}\n  B -- yes --> C[Build it]\n  B -- no --> A",
      "flowchart LR\n  A[Idée] --> B{Ça va ?}\n  B -- oui --> C[Réaliser]\n  B -- non --> A",
    ],
  },
  {
    titel: ["Sequenzdiagramm", "Sequence diagram", "Diagramme de séquence"],
    unter: ["Wer schickt wem was", "Who sends what to whom", "Qui envoie quoi à qui"],
    aliases: ["sequenz", "sequence", "ablauf", "mermaid"],
    code: [
      "sequenceDiagram\n  participant B as Browser\n  participant S as Server\n  B->>S: Anmelden\n  S-->>B: Sitzung\n  B->>S: Seite laden\n  S-->>B: Inhalt",
      "sequenceDiagram\n  participant B as Browser\n  participant S as Server\n  B->>S: Sign in\n  S-->>B: Session\n  B->>S: Load page\n  S-->>B: Content",
      "sequenceDiagram\n  participant B as Navigateur\n  participant S as Serveur\n  B->>S: Connexion\n  S-->>B: Session\n  B->>S: Charger la page\n  S-->>B: Contenu",
    ],
  },
  {
    titel: ["Kreisdiagramm", "Pie chart", "Camembert"],
    unter: ["Anteile, als Tabelle bearbeitbar", "Shares, editable as a table", "Parts, modifiable en tableau"],
    aliases: ["kreis", "torte", "pie", "anteile", "chart", "mermaid"],
    code: [
      'pie title Speicher\n  "Fotos" : 42\n  "Dokumente" : 25\n  "Sicherungen" : 18\n  "Frei" : 15',
      'pie title Storage\n  "Photos" : 42\n  "Documents" : 25\n  "Backups" : 18\n  "Free" : 15',
      'pie title Stockage\n  "Photos" : 42\n  "Documents" : 25\n  "Sauvegardes" : 18\n  "Libre" : 15',
    ],
  },
  {
    titel: ["Balkendiagramm", "Bar chart", "Diagramme en barres"],
    unter: ["Werte vergleichen, als Tabelle bearbeitbar", "Compare values, editable as a table", "Comparer des valeurs, modifiable en tableau"],
    aliases: ["balken", "säulen", "bar", "chart", "xychart", "mermaid"],
    code: [
      'xychart\n  title "Besuche je Monat"\n  x-axis ["Jan", "Feb", "Mär", "Apr"]\n  y-axis "Besuche" 0 --> 250\n  bar [120, 180, 150, 210]',
      'xychart\n  title "Visits per month"\n  x-axis ["Jan", "Feb", "Mar", "Apr"]\n  y-axis "Visits" 0 --> 250\n  bar [120, 180, 150, 210]',
      'xychart\n  title "Visites par mois"\n  x-axis ["janv.", "févr.", "mars", "avr."]\n  y-axis "Visites" 0 --> 250\n  bar [120, 180, 150, 210]',
    ],
  },
  {
    titel: ["Liniendiagramm", "Line chart", "Graphique linéaire"],
    unter: ["Verlauf über die Zeit, als Tabelle bearbeitbar", "Trend over time, editable as a table", "Évolution dans le temps, modifiable en tableau"],
    aliases: ["linie", "verlauf", "kurve", "line", "chart", "xychart", "mermaid"],
    code: [
      'xychart\n  title "Temperatur"\n  x-axis ["Mo", "Di", "Mi", "Do", "Fr"]\n  y-axis "°C"\n  line [14, 16, 15, 19, 21]',
      'xychart\n  title "Temperature"\n  x-axis ["Mon", "Tue", "Wed", "Thu", "Fri"]\n  y-axis "°C"\n  line [14, 16, 15, 19, 21]',
      'xychart\n  title "Température"\n  x-axis ["lun.", "mar.", "mer.", "jeu.", "ven."]\n  y-axis "°C"\n  line [14, 16, 15, 19, 21]',
    ],
  },
  {
    titel: ["Mindmap", "Mind map", "Carte mentale"],
    unter: ["Ein Thema und seine Äste", "A topic and its branches", "Un sujet et ses branches"],
    aliases: ["mindmap", "gedanken", "ideen", "mermaid"],
    code: [
      "mindmap\n  root((Homelab))\n    Dienste\n      Wiki\n      Fotos\n    Netz\n      DNS\n      Proxy\n    Sicherung",
      "mindmap\n  root((Homelab))\n    Services\n      Wiki\n      Photos\n    Network\n      DNS\n      Proxy\n    Backup",
      "mindmap\n  root((Homelab))\n    Services\n      Wiki\n      Photos\n    Réseau\n      DNS\n      Proxy\n    Sauvegarde",
    ],
  },
  {
    titel: ["Organigramm", "Org chart", "Organigramme hiérarchique"],
    unter: ["Aufbau von oben nach unten", "Structure from top to bottom", "Structure de haut en bas"],
    aliases: ["organigramm", "hierarchie", "baum", "org", "mermaid"],
    code: [
      "flowchart TD\n  L[Leitung] --> E[Entwicklung]\n  L --> B[Betrieb]\n  L --> V[Vertrieb]\n  E --> E1[Frontend]\n  E --> E2[Backend]",
      "flowchart TD\n  L[Management] --> E[Engineering]\n  L --> B[Operations]\n  L --> V[Sales]\n  E --> E1[Frontend]\n  E --> E2[Backend]",
      "flowchart TD\n  L[Direction] --> E[Développement]\n  L --> B[Exploitation]\n  L --> V[Ventes]\n  E --> E1[Frontend]\n  E --> E2[Backend]",
    ],
  },
  {
    titel: ["Zustandsdiagramm", "State diagram", "Diagramme d'états"],
    unter: ["Zustände und Übergänge", "States and transitions", "États et transitions"],
    aliases: ["zustand", "status", "state", "mermaid"],
    code: [
      "stateDiagram-v2\n  [*] --> Entwurf\n  Entwurf --> Prüfung: einreichen\n  Prüfung --> Entwurf: zurückgeben\n  Prüfung --> Veröffentlicht: freigeben\n  Veröffentlicht --> [*]",
      "stateDiagram-v2\n  [*] --> Draft\n  Draft --> Review: submit\n  Review --> Draft: send back\n  Review --> Published: approve\n  Published --> [*]",
      "stateDiagram-v2\n  [*] --> Brouillon\n  Brouillon --> Relecture: soumettre\n  Relecture --> Brouillon: renvoyer\n  Relecture --> Publié: valider\n  Publié --> [*]",
    ],
  },
];
