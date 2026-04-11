import { createContext, useContext, useState, useEffect } from "react";
import { jwtDecode } from "./jwtDecode";
import { setAuthCallbacks } from "./api";

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [accessToken, setAccessToken] = useState(() => localStorage.getItem("access_token"));
  const [refreshToken, setRefreshToken] = useState(() => localStorage.getItem("refresh_token"));
  const [user, setUser] = useState(() => {
    const t = localStorage.getItem("access_token");
    return t ? jwtDecode(t) : null;
  });

  function saveTokens(access, refresh) {
    localStorage.setItem("access_token", access);
    localStorage.setItem("refresh_token", refresh);
    setAccessToken(access);
    setRefreshToken(refresh);
    setUser(jwtDecode(access));
  }

  function logout() {
    localStorage.removeItem("access_token");
    localStorage.removeItem("refresh_token");
    setAccessToken(null);
    setRefreshToken(null);
    setUser(null);
  }

  useEffect(() => {
    setAuthCallbacks({
      getRefreshToken: () => localStorage.getItem("refresh_token"),
      saveTokens,
      onLogout: logout,
    });
  }, []);

  return (
    <AuthContext.Provider value={{ accessToken, refreshToken, user, saveTokens, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
