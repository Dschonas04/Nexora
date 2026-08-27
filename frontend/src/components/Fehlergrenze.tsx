// A boundary that catches render errors.
//
// React unmounts the whole tree when a render throws and nobody catches it. One
// unreadable block in a document would then not just fail to show: the entire
// window would go empty, down to the background colour, with no hint of what
// happened. That is what this prevents. Whatever throws is replaced locally by
// a message, everything around it keeps working.
import React from "react";

interface Props {
  children: React.ReactNode;
  /** What is said in place of the failed part. */
  text?: string;
}

interface Zustand {
  fehler: Error | null;
}

export default class Fehlergrenze extends React.Component<Props, Zustand> {
  state: Zustand = { fehler: null };

  static getDerivedStateFromError(fehler: Error): Zustand {
    return { fehler };
  }

  componentDidCatch(fehler: Error) {
    // Into the console, not silently swallowed: without the original message and
    // its stack a report of this is hard to chase.
    console.error("Fehlergrenze:", fehler);
  }

  render() {
    if (!this.state.fehler) return this.props.children;
    return (
      <div className="fehlergrenze">
        <div className="fehlergrenze-text">
          {this.props.text ?? "Dieser Teil konnte nicht angezeigt werden."}
        </div>
        <div className="fehlergrenze-grund muted small">{this.state.fehler.message}</div>
        <button className="btn" onClick={() => this.setState({ fehler: null })}>
          Noch einmal versuchen
        </button>
      </div>
    );
  }
}
