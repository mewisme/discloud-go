import { fetchMe, signOut as apiSignOut, type AuthUser } from "@/lib/api";

type Listener = () => void;

/** undefined = not loaded yet; null = signed out */
let user: AuthUser | null | undefined = undefined;
let loadPromise: Promise<AuthUser | null> | null = null;
const listeners = new Set<Listener>();

function emit(): void {
  for (const l of listeners) l();
}

export function subscribeAuth(onChange: Listener): () => void {
  listeners.add(onChange);
  return () => listeners.delete(onChange);
}

export function getAuthSnapshot(): AuthUser | null | undefined {
  return user;
}

export function getAuthServerSnapshot(): AuthUser | null | undefined {
  return undefined;
}

export function setAuthUser(next: AuthUser | null): void {
  user = next;
  loadPromise = Promise.resolve(next);
  emit();
}

/** Load session once; subsequent calls reuse the result until refresh/clear. */
export function ensureAuth(): Promise<AuthUser | null> {
  if (user !== undefined) return Promise.resolve(user);
  if (!loadPromise) {
    loadPromise = fetchMe()
      .then((u) => {
        user = u;
        emit();
        return u;
      })
      .catch(() => {
        user = null;
        emit();
        return null;
      });
  }
  return loadPromise;
}

export async function refreshAuth(): Promise<AuthUser | null> {
  user = undefined;
  loadPromise = null;
  emit();
  return ensureAuth();
}

export async function signOutAndClear(): Promise<void> {
  try {
    await apiSignOut();
  } finally {
    setAuthUser(null);
  }
}
