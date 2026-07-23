export type Visibility = "public" | "private";

export type AuthUser = {
  id: string;
  username: string;
  role: "admin" | "user";
};

export type AccountMe = {
  id: string;
  username: string;
  role: "admin" | "user";
  createdAt: string;
  passwordChangedAt: string;
  stats: {
    fileCount: number;
    totalBytes: number;
    publicCount: number;
    privateCount: number;
    expiringSoonCount: number;
  };
  session: {
    createdAt: string;
    lastSeenAt: string;
    expiresAt: string;
    ip: string;
    userAgent: string;
  };
  retention: {
    authenticatedDays: number;
    anonymousDays: number;
    downloadExtensionDays: number;
    maxRetentionDays: number;
  };
  preferences: {
    defaultVisibility: Visibility;
  };
};

/** Upload outcome badge on a file record. */
export type FileStatus = "ready" | "reused";

/** Shared file link payload from upload / file APIs. */
export interface FileLinks {
  fileId: string;
  fileName: string;
  fileSize: number;
  url: string;
  longURL: string;
  downloadURL: string;
  longDownloadURL: string;
  visibility?: Visibility;
  status?: FileStatus;
  ownedByCurrentUser?: boolean;
  createdAt?: string;
  expiresAt?: string;
  /** Present once when upload creates a private file. */
  accessToken?: string;
}

export type UploadResult = FileLinks;

/** GET /api/files/{id} */
export interface FileMeta {
  fileId: string;
  fileName: string;
  fileSize: number;
  chunkSize: number;
  createdAt: string;
  expiresAt?: string;
  visibility?: Visibility;
  status?: FileStatus;
  ownedByCurrentUser?: boolean;
}

/** Owner list item from GET /api/files */
export interface OwnedFile extends FileLinks {
  chunkSize?: number;
  createdAt: string;
  expiresAt: string;
  visibility: Visibility;
  status?: FileStatus;
}

export interface FileInspect {
  fileId: string;
  fileName: string;
  fileSize: number;
  chunkSize: number;
  chunkCount: number;
  createdAt: string;
  expiresAt?: string;
  visibility?: Visibility;
  status?: FileStatus;
  ownedByCurrentUser?: boolean;
  views: number;
  downloads: number;
  ranges: number;
  bytesServed: number;
  uniqueVisitors: number;
  lastAccessAt?: string;
  url: string;
  longURL: string;
  downloadURL: string;
  longDownloadURL: string;
}

/** Visibility PATCH / rotate responses (token shown once). */
export type TokenRevealResult = {
  visibility?: Visibility;
  accessToken?: string;
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** Absolute URL on the public API (no Next proxy). */
export function apiURL(path: string): string {
  const injected =
    typeof globalThis !== "undefined"
      ? (globalThis as { __DISCLOUD_API__?: string }).__DISCLOUD_API__
      : undefined;
  const base = (injected || "http://localhost:8080").replace(/\/$/, "");
  const p = path.startsWith("/") ? path : `/${path}`;
  return `${base}${p}`;
}

function apiPath(path: string, query?: Record<string, string | undefined>): string {
  const u = new URL(apiURL(path));
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== "") u.searchParams.set(k, v);
    }
  }
  return u.toString();
}

/** Build /f/... URL with optional download + private token (URLSearchParams). */
export function buildFileURL(opts: {
  fileId: string;
  fileName?: string;
  download?: boolean;
  token?: string;
}): string {
  const path = opts.fileName
    ? `/f/${opts.fileId}/${opts.fileName}`
    : `/f/${opts.fileId}`;
  return apiPath(path, {
    download: opts.download ? "1" : undefined,
    token: opts.token,
  });
}

/** Web inspect path `/i/{id}` with optional token (path + query only). */
export function buildInspectPath(fileId: string, token?: string): string {
  const u = new URL(`/i/${fileId}`, "http://local.invalid");
  if (token) u.searchParams.set("token", token);
  return `${u.pathname}${u.search}`;
}

/** Strip ?token= from a URL string (best-effort). */
export function stripTokenFromURL(raw: string): string {
  try {
    const u = new URL(raw);
    u.searchParams.delete("token");
    return u.toString();
  } catch {
    return raw;
  }
}

/** Rebuild share links from the runtime API origin (API_URL inject). */
export function withPublicURLs(result: UploadResult): UploadResult {
  const id = result.fileId;
  const name = result.fileName;
  const token = result.accessToken;
  return {
    ...result,
    url: buildFileURL({ fileId: id, token }),
    longURL: buildFileURL({ fileId: id, fileName: name, token }),
    downloadURL: buildFileURL({ fileId: id, download: true, token }),
    longDownloadURL: buildFileURL({
      fileId: id,
      fileName: name,
      download: true,
      token,
    }),
  };
}

async function readMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { message?: string };
    if (body.message) return body.message;
  } catch {
    /* ignore */
  }
  return `Request failed (${res.status})`;
}

/** Credentialed fetch against the API. JSON body when `json` is set. */
export async function apiFetch<T>(
  path: string,
  init?: RequestInit & { json?: unknown },
): Promise<T> {
  const { json, headers: hdrs, ...rest } = init ?? {};
  const headers = new Headers(hdrs);
  let body = rest.body;
  if (json !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(json);
  }
  const res = await fetch(apiURL(path), {
    ...rest,
    body,
    headers,
    credentials: "include",
    cache: "no-store",
  });
  if (!res.ok) throw new ApiError(res.status, await readMessage(res));
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

export async function fetchMe(): Promise<AuthUser | null> {
  const res = await fetch(apiURL("/api/auth/me"), {
    credentials: "include",
    cache: "no-store",
  });
  if (res.status === 401) return null;
  if (!res.ok) throw new ApiError(res.status, await readMessage(res));
  const body = (await res.json()) as AccountMe;
  return { id: body.id, username: body.username, role: body.role };
}

export async function fetchAccountMe(): Promise<AccountMe> {
  return apiFetch<AccountMe>("/api/auth/me");
}

export async function signUp(username: string, password: string): Promise<AuthUser> {
  return apiFetch<AuthUser>("/api/auth/signup", {
    method: "POST",
    json: { username, password },
  });
}

export async function signIn(username: string, password: string): Promise<AuthUser> {
  return apiFetch<AuthUser>("/api/auth/signin", {
    method: "POST",
    json: { username, password },
  });
}

export async function signOut(): Promise<void> {
  await apiFetch<void>("/api/auth/signout", { method: "POST" });
}

export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  await apiFetch<void>("/api/auth/password", {
    method: "POST",
    json: { currentPassword, newPassword },
  });
}

export async function updatePreferences(prefs: {
  defaultVisibility: Visibility;
}): Promise<{ preferences: { defaultVisibility: Visibility } }> {
  return apiFetch("/api/auth/preferences", {
    method: "PATCH",
    json: prefs,
  });
}

export async function listMyFiles(opts?: {
  limit?: number;
  offset?: number;
}): Promise<{ files: OwnedFile[] }> {
  const u = new URL(apiURL("/api/files"));
  if (opts?.limit != null) u.searchParams.set("limit", String(opts.limit));
  if (opts?.offset != null) u.searchParams.set("offset", String(opts.offset));
  const res = await fetch(u.toString(), {
    credentials: "include",
    cache: "no-store",
  });
  if (!res.ok) throw new ApiError(res.status, await readMessage(res));
  return (await res.json()) as { files: OwnedFile[] };
}

export async function setFileVisibility(
  fileId: string,
  visibility: Visibility,
): Promise<TokenRevealResult> {
  return apiFetch<TokenRevealResult>(`/api/files/${fileId}/visibility`, {
    method: "PATCH",
    json: { visibility },
  });
}

export async function rotateAccessToken(
  fileId: string,
): Promise<TokenRevealResult> {
  return apiFetch<TokenRevealResult>(
    `/api/files/${fileId}/access-token/rotate`,
    { method: "POST" },
  );
}

export async function deleteFile(fileId: string): Promise<void> {
  await apiFetch<void>(`/api/files/${fileId}`, { method: "DELETE" });
}

export async function fetchFileMeta(
  fileId: string,
  init?: RequestInit & { token?: string },
): Promise<FileMeta> {
  const { token, ...rest } = init ?? {};
  const res = await fetch(apiPath(`/api/files/${fileId}`, { token }), {
    credentials: "include",
    cache: "no-store",
    ...rest,
  });
  if (res.status === 404) throw new Error("File not found on server");
  if (!res.ok) throw new Error(await readMessage(res));
  return (await res.json()) as FileMeta;
}

export async function fetchFileInspect(
  fileId: string,
  init?: RequestInit & { token?: string },
): Promise<FileInspect> {
  const { token, ...rest } = init ?? {};
  const res = await fetch(apiPath(`/api/files/${fileId}/inspect`, { token }), {
    credentials: "include",
    cache: "no-store",
    ...rest,
  });
  if (res.status === 404) throw new Error("File not found on server");
  if (!res.ok) throw new Error(await readMessage(res));
  return (await res.json()) as FileInspect;
}
