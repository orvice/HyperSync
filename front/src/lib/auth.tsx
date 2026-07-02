import { createContext, useContext, useState, useEffect, type ReactNode } from "react";

interface AuthContextType {
  token: string | null;
  login: (token: string, expiresAt: bigint) => void;
  logout: () => void;
  isAuthenticated: boolean;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => {
    const stored = localStorage.getItem("token");
    const expiresAt = localStorage.getItem("token_expires_at");
    if (stored && expiresAt) {
      const expiry = Number(expiresAt);
      if (Date.now() / 1000 < expiry) {
        return stored;
      }
      localStorage.removeItem("token");
      localStorage.removeItem("token_expires_at");
    }
    return null;
  });

  const login = (newToken: string, expiresAt: bigint) => {
    localStorage.setItem("token", newToken);
    localStorage.setItem("token_expires_at", expiresAt.toString());
    setToken(newToken);
  };

  const logout = () => {
    localStorage.removeItem("token");
    localStorage.removeItem("token_expires_at");
    setToken(null);
  };

  useEffect(() => {
    const handleStorage = (e: StorageEvent) => {
      if (e.key === "token") {
        setToken(e.newValue);
      }
    };
    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, []);

  return (
    <AuthContext.Provider value={{ token, login, logout, isAuthenticated: !!token }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
