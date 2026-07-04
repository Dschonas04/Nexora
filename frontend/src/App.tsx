import { Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./auth";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Workspace from "./pages/Workspace";
import PublicPage from "./pages/PublicPage";

export default function App() {
  const { user, loading } = useAuth();
  if (loading) return <div className="empty-state">Lädt…</div>;
  return (
    <Routes>
      <Route path="/share/:token" element={<PublicPage />} />
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login />} />
      <Route path="/register" element={user ? <Navigate to="/" replace /> : <Register />} />
      <Route path="/*" element={user ? <Workspace /> : <Navigate to="/login" replace />} />
    </Routes>
  );
}
