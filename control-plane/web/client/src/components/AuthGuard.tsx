import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { useAuth } from "../contexts/AuthContext";
import { setGlobalApiKey } from "../services/api";

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { apiKey, setApiKey, isAuthenticated, authRequired, iamEnabled, iamUser } = useAuth();
  const [inputKey, setInputKey] = useState("");
  const [error, setError] = useState("");
  const [validating, setValidating] = useState(false);

  useEffect(() => {
    setGlobalApiKey(apiKey);
  }, [apiKey]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setValidating(true);

    try {
      const response = await fetch("/api/ui/v1/dashboard/summary", {
        headers: { "X-API-Key": inputKey },
      });

      if (response.ok) {
        setApiKey(inputKey);
        setGlobalApiKey(inputKey);
      } else {
        setError("Invalid API key");
      }
    } catch {
      setError("Connection failed");
    } finally {
      setValidating(false);
    }
  };

  const handleIAMLogin = () => {
    // Redirect to the server-side OAuth login endpoint.
    window.location.href = "/auth/login";
  };

  if (!authRequired || isAuthenticated) {
    return <>{children}</>;
  }

  return (
    <div className="flex items-center justify-center min-h-screen bg-background">
      <div className="p-8 bg-card rounded-lg shadow-lg max-w-md w-full">
        <h2 className="text-2xl font-semibold mb-2">Hanzo Bot Control Plane</h2>
        <p className="text-muted-foreground mb-6">Sign in to continue</p>

        {/* IAM / Hanzo sign-in button */}
        {iamEnabled && (
          <>
            <button
              type="button"
              onClick={handleIAMLogin}
              className="w-full bg-primary text-primary-foreground p-3 rounded-md font-medium hover:opacity-90 transition-opacity flex items-center justify-center gap-2 mb-4"
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                xmlns="http://www.w3.org/2000/svg"
                className="shrink-0"
              >
                <path
                  d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
              Sign in with Hanzo
            </button>

            <div className="relative my-6">
              <div className="absolute inset-0 flex items-center">
                <div className="w-full border-t border-muted" />
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-card px-2 text-muted-foreground">or use API key</span>
              </div>
            </div>
          </>
        )}

        {/* API key form (always available as fallback) */}
        <form onSubmit={handleSubmit}>
          <input
            type="password"
            value={inputKey}
            onChange={(e) => setInputKey(e.target.value)}
            placeholder="API Key"
            className="w-full p-3 border rounded-md mb-4 bg-background"
            disabled={validating}
            autoFocus={!iamEnabled}
          />

          {error && <p className="text-destructive mb-4">{error}</p>}

          <button
            type="submit"
            className="w-full bg-secondary text-secondary-foreground p-3 rounded-md font-medium disabled:opacity-50"
            disabled={validating || !inputKey}
          >
            {validating ? "Validating..." : "Connect with API Key"}
          </button>
        </form>
      </div>
    </div>
  );
}
