// Session state for the whole app. The token itself is an httpOnly cookie and
// therefore invisible here; "signed in" simply means /auth/me answered.
import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { Anmeldung, api, brauchtZweitenSchritt, User } from "./api/client";

interface AuthCtx {
  user: User | null;
  loading: boolean;
  /**
   * Signing in. Back comes either the account -- then the session is up and the
   * state here is set -- or the prompt for the second step. The decision lies
   * with the server; the sign-in page reads it off here instead of making it
   * itself.
   */
  login: (kennung: string, password: string) => Promise<Anmeldung>;
  /** The second step: code from the app or a recovery code. */
  zweiterSchritt: (ticket: string, code: string) => Promise<void>;
  register: (email: string, name: string, password: string, benutzername?: string) => Promise<void>;
  logout: () => Promise<void>;
  /**
   * Read the account once more. Needed after somebody has changed something on
   * their own profile: name and picture stand in several places of the
   * interface, and those should show it at once and not only after a reload.
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
    // With the second step the state stays empty: nobody is signed in yet, and
    // an account set here would let the interface show the workspace the server
    // does not hand out at all.
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
  // A failure leaves the previous state standing: it is stale, but usable.
  // Signing out because a follow-up request failed would be worse.
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
