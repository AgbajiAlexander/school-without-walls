export interface AuthUser {
  id: string;
  email: string;
  full_name: string;
  status: "PENDING_ACTIVATION" | "ACTIVE" | "SUSPENDED";
  created_at: string;
}

export interface AuthSession {
  id: string;
  created_at: string;
  expires_at: string;
}

export interface CurrentUser extends AuthUser {
  session_created_at: string;
  session_expires_at: string;
}

export interface LoginResult {
  user: AuthUser;
  csrf_token: string;
}

export interface InvitationDetails {
  invited_email: string;
  expires_at: string;
  tenant_slug?: string;
  tenant_name?: string;
}

interface ApiEnvelope<T> {
  data: T;
}

interface ApiFailure {
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
  };
}

let storedCSRFToken: string | null = null;

export function setCSRFToken(token: string | null): void {
  storedCSRFToken = token;
}

export function getCSRFToken(): string | null {
  return storedCSRFToken;
}

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {});
  headers.set("Content-Type", "application/json");
  headers.set("Accept", "application/json");

  const method = (options.method || "GET").toUpperCase();
  const csrf = getCSRFToken();
  if (csrf && !["GET", "HEAD", "OPTIONS", "TRACE"].includes(method)) {
    headers.set("X-CSRF-Token", csrf);
  }

  const res = await fetch(endpoint, {
    ...options,
    headers,
    credentials: "same-origin",
  });

  let payload: ApiEnvelope<T> | ApiFailure;
  try {
    payload = await res.json() as ApiEnvelope<T> | ApiFailure;
  } catch {
    throw new Error(`The server returned an invalid response (${res.status}).`);
  }

  if (!res.ok) {
    const failure = payload as ApiFailure;
    const err = new Error(failure.error?.message || `Request failed with status ${res.status}`) as Error & {
      code?: string;
      details?: unknown;
      status?: number;
    };
    err.code = failure.error?.code;
    err.details = failure.error?.details;
    err.status = res.status;
    throw err;
  }

  return (payload as ApiEnvelope<T>).data;
}

export async function login(email: string, password: string): Promise<LoginResult> {
  const data = await request<LoginResult>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  setCSRFToken(data.csrf_token);
  return data;
}

export async function logout(): Promise<void> {
  await request<{ message: string }>("/api/v1/auth/logout", { method: "POST" });
  setCSRFToken(null);
}

export async function getMe(): Promise<CurrentUser> {
  const data = await request<{
    user: AuthUser;
    session: AuthSession;
    csrf_token: string;
  }>("/api/v1/auth/me");
  setCSRFToken(data.csrf_token);
  return {
    ...data.user,
    session_created_at: data.session.created_at,
    session_expires_at: data.session.expires_at,
  };
}

export async function verifyInvitation(token: string): Promise<InvitationDetails> {
  return request<InvitationDetails>(`/api/v1/invitations/${encodeURIComponent(token)}/verify`);
}

export async function acceptInvitation(
  token: string,
  fullName: string,
  password: string
): Promise<LoginResult> {
  const data = await request<LoginResult>("/api/v1/invitations/accept", {
    method: "POST",
    body: JSON.stringify({ token, full_name: fullName, password }),
  });
  setCSRFToken(data.csrf_token);
  return data;
}

export function forgotPassword(email: string): Promise<{ message: string }> {
  return request<{ message: string }>("/api/v1/auth/forgot-password", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export function resetPassword(token: string, newPassword: string): Promise<{ message: string }> {
  return request<{ message: string }>("/api/v1/auth/reset-password", {
    method: "POST",
    body: JSON.stringify({ token, new_password: newPassword }),
  });
}
