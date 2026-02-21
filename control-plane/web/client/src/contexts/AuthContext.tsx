import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { setGlobalApiKey } from "../services/api";

interface IAMUser {
  id: string;
  name: string;
  displayName: string;
  email: string;
  avatar: string;
  owner: string;
}

interface AuthContextType {
  apiKey: string | null;
  setApiKey: (key: string | null) => void;
  isAuthenticated: boolean;
  authRequired: boolean;
  iamEnabled: boolean;
  iamUser: IAMUser | null;
  clearAuth: () => void;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);
const STORAGE_KEY = "af_api_key";

// Simple obfuscation for localStorage; not meant as real security.
const encryptKey = (key: string): string => btoa(key.split("").reverse().join(""));
const decryptKey = (value: string): string => {
  try {
    return atob(value).split("").reverse().join("");
  } catch {
    return "";
  }
};

// Initialize global API key from localStorage BEFORE any React rendering
// This ensures API calls made during initial render have the key
const initStoredKey = (() => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const key = decryptKey(stored);
      if (key) {
        setGlobalApiKey(key);
        return key;
      }
    }
  } catch {
    // localStorage might not be available
  }
  return null;
})();

export function AuthProvider({ children }: { children: ReactNode }) {
  // Initialize with pre-loaded key so it's available immediately
  const [apiKey, setApiKeyState] = useState<string | null>(initStoredKey);
  const [authRequired, setAuthRequired] = useState(false);
  const [iamEnabled, setIamEnabled] = useState(false);
  const [iamUser, setIamUser] = useState<IAMUser | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const checkAuth = async () => {
      try {
        // Step 1: Check IAM userinfo endpoint (handles both IAM session cookies and Bearer tokens).
        const userinfoResp = await fetch("/api/v1/auth/userinfo", {
          credentials: "same-origin",
        });

        if (userinfoResp.ok) {
          const data = await userinfoResp.json();
          setIamEnabled(data.iam_enabled ?? false);

          if (data.authenticated) {
            if (data.method === "iam" && data.user) {
              setIamUser(data.user);
              setAuthRequired(true);
              setLoading(false);
              return;
            }
            if (data.method === "api_key") {
              // Authenticated via API key (sent via cookie or header somehow).
              setAuthRequired(true);
              setLoading(false);
              return;
            }
          }

          // Not authenticated via IAM - fall through to API key check below.
          if (data.iam_enabled) {
            setIamEnabled(true);
          }
        } else if (userinfoResp.status === 401) {
          // Check if IAM is enabled from the response body.
          try {
            const errData = await userinfoResp.json();
            if (errData.iam_enabled) {
              setIamEnabled(true);
            }
          } catch {
            // Ignore JSON parse errors.
          }
        }

        // Step 2: Fall back to API key check (existing behavior).
        const stored = localStorage.getItem(STORAGE_KEY);
        const storedKey = stored ? decryptKey(stored) : null;

        // Clean up invalid stored key.
        if (stored && !storedKey) {
          localStorage.removeItem(STORAGE_KEY);
        }

        const headers: HeadersInit = {};
        if (storedKey) {
          headers["X-API-Key"] = storedKey;
        }

        const response = await fetch("/api/ui/v1/dashboard/summary", { headers });

        if (response.ok) {
          if (storedKey) {
            setApiKeyState(storedKey);
            setGlobalApiKey(storedKey);
            setAuthRequired(true);
          } else {
            setAuthRequired(false);
          }
        } else if (response.status === 401) {
          setAuthRequired(true);
          setGlobalApiKey(null);
          if (stored) {
            localStorage.removeItem(STORAGE_KEY);
          }
        }
      } catch (err) {
        console.error("Auth check failed:", err);
        setAuthRequired(true);
      } finally {
        setLoading(false);
      }
    };

    void checkAuth();
  }, []);

  const setApiKey = (key: string | null) => {
    setApiKeyState(key);
    setGlobalApiKey(key);
    if (key) {
      localStorage.setItem(STORAGE_KEY, encryptKey(key));
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  };

  const clearAuth = () => {
    setApiKeyState(null);
    setGlobalApiKey(null);
    setIamUser(null);
    localStorage.removeItem(STORAGE_KEY);
  };

  const logout = async () => {
    try {
      await fetch("/auth/logout", { method: "POST", credentials: "same-origin" });
    } catch {
      // Ignore logout errors.
    }
    clearAuth();
    // Force reload to clear all state.
    window.location.href = "/ui/";
  };

  if (loading) {
    return <div className="flex items-center justify-center min-h-screen">Loading...</div>;
  }

  const isAuthenticated = iamUser !== null || (!authRequired || !!apiKey);

  return (
    <AuthContext.Provider
      value={{
        apiKey,
        setApiKey,
        isAuthenticated,
        authRequired,
        iamEnabled,
        iamUser,
        clearAuth,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return ctx;
}
