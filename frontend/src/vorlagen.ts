// The templates every instance has, in the reader's language. Saved templates
// come from the service; these two are here so an empty page has something
// to offer from the first day on.
//
// Written as plain partial blocks: the editor fills in ids and defaults when
// it takes them over, exactly as it does for pasted content.
import type { Sprache } from "./sprache";

export type EingebauteVorlage = {
  key: string;
  title: string;
  icon: string;
  content: unknown[];
};

const h = (level: number, text: string) => ({ type: "heading", props: { level }, content: text });
const p = (text = "") => ({ type: "paragraph", content: text });
const li = (text: string) => ({ type: "bulletListItem", content: text });
const nr = (text: string) => ({ type: "numberedListItem", content: text });

function heute(s: Sprache): string {
  return new Date().toLocaleDateString(s === "de" ? "de-DE" : s === "fr" ? "fr-FR" : "en-GB");
}

export function eingebauteVorlagen(s: Sprache): EingebauteVorlage[] {
  if (s === "de") {
    return [
      {
        key: "besprechung",
        title: `Besprechung ${heute(s)}`,
        icon: "",
        content: [
          p("Datum: " + heute(s) + " · Ort: "),
          h(2, "Teilnehmende"),
          li("@"),
          h(2, "Tagesordnung"),
          nr("Stand der Dinge"),
          nr("Offene Fragen"),
          h(2, "Notizen"),
          p(),
          h(2, "Beschlüsse"),
          li(""),
        ],
      },
      {
        key: "anleitung",
        title: "Anleitung: ",
        icon: "",
        content: [
          p("Wofür ist diese Anleitung, und für wen?"),
          h(2, "Voraussetzungen"),
          li("Zugang zu …"),
          h(2, "Schritte"),
          nr("Erster Schritt"),
          nr("Zweiter Schritt"),
          nr("Prüfen, ob es geklappt hat"),
          h(2, "Wenn es nicht klappt"),
          p("Typische Fehler und was hilft."),
          h(2, "Siehe auch"),
          li("[[Verwandte Seite]]"),
        ],
      },
    ];
  }
  const fr = s === "fr";
  return [
    {
      key: "besprechung",
      title: (fr ? "Réunion " : "Meeting ") + heute(s),
      icon: "",
      content: [
        p((fr ? "Date : " : "Date: ") + heute(s) + (fr ? " · Lieu : " : " · Place: ")),
        h(2, fr ? "Participants" : "Attendees"),
        li("@"),
        h(2, fr ? "Ordre du jour" : "Agenda"),
        nr(fr ? "Où en sommes-nous" : "Where we stand"),
        nr(fr ? "Questions ouvertes" : "Open questions"),
        h(2, "Notes"),
        p(),
        h(2, fr ? "Décisions" : "Decisions"),
        li(""),
      ],
    },
    {
      key: "anleitung",
      title: fr ? "Guide : " : "How-to: ",
      icon: "",
      content: [
        p(fr ? "À quoi sert ce guide, et pour qui ?" : "What is this guide for, and who is it for?"),
        h(2, fr ? "Prérequis" : "Before you start"),
        li(fr ? "Accès à …" : "Access to …"),
        h(2, fr ? "Étapes" : "Steps"),
        nr(fr ? "Première étape" : "First step"),
        nr(fr ? "Deuxième étape" : "Second step"),
        nr(fr ? "Vérifier que ça a marché" : "Check that it worked"),
        h(2, fr ? "Si ça ne marche pas" : "If it does not work"),
        p(fr ? "Erreurs typiques et ce qui aide." : "Typical mistakes and what helps."),
        h(2, fr ? "Voir aussi" : "See also"),
        li(fr ? "[[Page liée]]" : "[[Related page]]"),
      ],
    },
  ];
}
