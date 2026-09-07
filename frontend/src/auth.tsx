// Session state for the whole app. The token itself is an httpOnly cookie and
// therefore invisible here; "signed in" simply means /auth/me answered.
import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { Anmeldung, api, brauchtZweitenSchritt, User } from "./api/client";

interface AuthCtx {
  user: User | null;
  loading: boolean;
  /**
   * Anmelden. Zurück kommt entweder das Konto -- dann steht die Sitzung und der
   * Zustand hier ist gesetzt -- oder die Aufforderung zum zweiten Schritt. Die
   * Entscheidung liegt beim Server; die Anmeldeseite liest sie hier ab, statt
   * sie selbst zu treffen.
   */
  login: (kennung: string, password: string) => Promise<Anmeldung>;
  /** Der zweite Schritt: Code aus der App oder ein Ersatzcode. */
  zweiterSchritt: (ticket: string, code: string) => Promise<void>;
  register: (email: string, name: string, password: string, benutzername?: string) => Promise<void>;
  logout: () => Promise<void>;
  /**
   * Das Konto noch einmal lesen. Gebraucht, nachdem jemand am eigenen Profil
   * etwas geändert hat: Name und Bild stehen an mehreren Stellen der
   * Oberfläche, und die sollen es sofort zeigen und nicht erst nach einem
   * Neuladen.
   */
  neuLaden: () => Promise<void>;
}

// No default value: every consumer sits under the provider, and the cast keeps
// the context type honest instead of making every field optional.
const Ctx = createContext<AuthCtx>(null as unknown as AuthCtx);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  // Ask the backend once on mount who we are. A failure is the normal case for
  // a visitor without a session, hence the silent catch.
  useEffect(() => {
    api
      .me()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setLoading(false));
  }, []);

  const login = async (kennung: string, password: string) => {
    const antwort = await api.login(kennung, password);
    // Beim zweiten Schritt bleibt der Zustand leer: angemeldet ist noch
    // niemand, und ein hier gesetztes Konto liesse die Oberfläche den
    // Arbeitsbereich zeigen, den der Server gar nicht herausgibt.
    if (!brauchtZweitenSchritt(antwort)) setUser(antwort);
    return antwort;
  };
  const zweiterSchritt = async (ticket: string, code: string) => {
    setUser(await api.zweitfaktorPruefen(ticket, code));
  };
  const register = async (email: string, name: string, password: string, benutzername = "") => {
    setUser(await api.register(email, name, password, benutzername));
  };
  // Clear the cookie first, then the local state, so a failing request leaves
  // the user signed in rather than showing a logged-out UI with a live session.
  const logout = async () => {
    await api.logout();
    setUser(null);
  };
  // Ein Fehlschlag lässt den bisherigen Stand stehen: er ist veraltet, aber
  // brauchbar. Abzumelden, weil eine Nachfrage scheiterte, wäre schlimmer.
  const neuLaden = async () => {
    try {
      setUser(await api.me());
    } catch {
      /* der bisherige Stand bleibt */
    }
  };

  return <Ctx.Provider value={{ user, loading, login, zweiterSchritt, register, logout, neuLaden }}>{children}</Ctx.Provider>;
}

export const useAuth = () => useContext(Ctx);
