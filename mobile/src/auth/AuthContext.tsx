import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { api, ApiError, setUnauthorizedHandler } from '@/api/client';
import type { User } from '@/api/types';
import { clearToken, setToken } from './tokenStorage';

type AuthResponse = User & { token: string };

type AuthContextValue = {
  user: User | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string) => Promise<void>;
  loginWithGoogleIdToken: (idToken: string) => Promise<void>;
  /** fullName is only ever non-empty on the very first sign-in: Apple
   * gives it to the app once and never repeats it. */
  loginWithAppleIdentityToken: (identityToken: string, fullName: string) => Promise<void>;
  loginWithSessionToken: (token: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Signs the account out of every device (revokes all session tokens
   * server-side), then signs out locally. */
  logoutEverywhere: () => Promise<void>;
  /** Reloads the signed-in user, e.g. once they confirmed their email. */
  refreshUser: () => Promise<void>;
  /** Permanently deletes the signed-in user and all their data, then signs
   * out locally. Throws (ApiError) if the server refuses. */
  deleteAccount: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // A 401 from any request (expired/invalid token) drops us back to the
    // login screen the same way a web session cookie expiring would.
    setUnauthorizedHandler(() => setUser(null));
    return () => setUnauthorizedHandler(null);
  }, []);

  useEffect(() => {
    api
      .get<User>('/auth/me')
      .then(setUser)
      .catch((err) => {
        if (!(err instanceof ApiError && err.status === 401)) throw err;
      })
      .finally(() => setLoading(false));
  }, []);

  const login = async (email: string, password: string) => {
    const res = await api.post<AuthResponse>('/auth/login', { email, password });
    await setToken(res.token);
    setUser(res);
  };

  const register = async (email: string, password: string, displayName: string) => {
    const res = await api.post<AuthResponse>('/auth/register', {
      email,
      password,
      display_name: displayName,
    });
    await setToken(res.token);
    setUser(res);
  };

  // Native counterpart to the web app's server-redirect Google login (see
  // backend's GoogleLogin/GoogleCallback) — expo-auth-session runs the OAuth
  // dance client-side (see auth/google.ts) and hands us the resulting ID
  // token directly, which the backend verifies itself (GoogleMobileLogin)
  // rather than us trusting whatever the token claims.
  const loginWithGoogleIdToken = async (idToken: string) => {
    const res = await api.post<AuthResponse>('/auth/google/mobile', { id_token: idToken });
    await setToken(res.token);
    setUser(res);
  };

  const loginWithAppleIdentityToken = async (identityToken: string, fullName: string) => {
    const res = await api.post<AuthResponse>('/auth/apple/mobile', {
      identity_token: identityToken,
      full_name: fullName,
    });
    await setToken(res.token);
    setUser(res);
  };

  // Native counterpart to the web app's server-redirect Robine ID login
  // (see backend's RobineLogin/RobineCallback) — Robine has no native
  // SDK/ID-token shortcut like Google's, so the whole OIDC dance runs
  // server-side behind an in-app browser (see auth/robine.ts) and the
  // backend hands back a session token it already fully vouches for,
  // rather than a raw provider ID token we'd still need verified.
  const loginWithSessionToken = async (token: string) => {
    await setToken(token);
    const me = await api.get<User>('/auth/me');
    setUser(me);
  };

  const logout = async () => {
    await api.post('/auth/logout').catch(() => {
      // Best-effort — the backend clears its cookie, but for a native
      // client the token is worthless the moment we drop it below.
    });
    await clearToken();
    setUser(null);
  };

  const logoutEverywhere = async () => {
    await api.post('/auth/logout-all');
    await clearToken();
    setUser(null);
  };

  const refreshUser = async () => {
    setUser(await api.get<User>('/auth/me'));
  };

  const deleteAccount = async () => {
    await api.delete('/auth/me');
    await clearToken();
    setUser(null);
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        loading,
        login,
        register,
        loginWithGoogleIdToken,
        loginWithAppleIdentityToken,
        loginWithSessionToken,
        logout,
        logoutEverywhere,
        refreshUser,
        deleteAccount,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
