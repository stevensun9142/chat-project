import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { AuthProvider, useAuth } from "./auth";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Rooms from "./pages/Rooms";

function ProtectedRoute({ children }) {
  const { accessToken } = useAuth();
  return accessToken ? children : <Navigate to="/login" />;
}

function GuestRoute({ children }) {
  const { accessToken } = useAuth();
  return accessToken ? <Navigate to="/" /> : children;
}

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<GuestRoute><Login /></GuestRoute>} />
          <Route path="/register" element={<GuestRoute><Register /></GuestRoute>} />
          <Route path="/" element={<ProtectedRoute><Rooms /></ProtectedRoute>} />
          <Route path="/rooms/:roomId" element={<ProtectedRoute><Rooms /></ProtectedRoute>} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
