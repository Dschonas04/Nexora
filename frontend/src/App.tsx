// Route table and the signed-in / signed-out split.
import { Routes, Route, Navigate } from "react-router";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Workspace from "./pages/Workspace";
import PublicPage from "./pages/PublicPage";

export default function App() {
  const { user, loading } = useAuth();
  // Wait for the session check before routing. Without this, a signed-in user
  // reloading the page would be bounced to the login screen for a moment.
  //
  // Faded in with a delay: the session check is mostly through within a few
  // milliseconds, and a word that only flashes is not read by anybody anyway, it
  // merely registers as a twitch.
  if (loading) return <div className="empty-state spaet">Lädt…</div>;
  return (
    <Routes>
      {/* Public links stay reachable without a session, so this route comes
          before the redirect below. */}
      <Route path="/share/:token" element={<PublicPage />} />
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login />} />
      <Route path="/register" element={user ? <Navigate to="/" replace /> : <Register />} />
      {/* Everything else is the app, or the login screen. Workspace owns its
          own nested routes. replace keeps the redirect out of the history, so
          the back button does not bounce between login and app. */}
      <Route path="/*" element={user ? <Workspace /> : <Navigate to="/login" replace />} />
    </Routes>
  );
}
