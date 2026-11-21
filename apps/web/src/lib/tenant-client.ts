"use client";

import { getCSRFToken, setCSRFToken } from "@/lib/auth-client";

export interface Tenant {
  id: string;
  slug: string;
  name: string;
  status: "PENDING_SETUP" | "ACTIVE" | "SUSPENDED" | "ARCHIVED";
  locale: string;
  timezone: string;
  branding: Record<string, unknown>;
  education_stages: string[];
  created_at: string;
  updated_at: string;
  current_role?: string;
  can_invite_staff?: boolean;
  can_manage_academics?: boolean;
  can_read_lessons?: boolean;
}

export interface ProvisionTenantRequest {
  slug: string;
  name: string;
  owner_email: string;
  locale: string;
  timezone: string;
}

export interface TenantSettingsRequest {
  name: string;
  locale: string;
  timezone: string;
  branding: Record<string, unknown>;
  education_stages: string[];
}

export interface CapturedMessage {
  to: string;
  subject: string;
  body: string;
  created_at: string;
}

export type StaffInvitationRole = "SCHOOL_ADMIN" | "PRINCIPAL";

export function canInviteStaff(tenant: Pick<Tenant, "status" | "can_invite_staff">): boolean {
  return tenant.status === "ACTIVE" && tenant.can_invite_staff === true;
}

export function isStaffInvitationRole(role: string): role is StaffInvitationRole {
  return role === "SCHOOL_ADMIN" || role === "PRINCIPAL";
}

type StaffInvitationResult = "sent" | "invalid" | "conflict" | "unauthenticated" | "forbidden" | "unconfirmed" | "rate-limited";

interface ApiEnvelope<T> {
  data: T;
}

interface ApiFailure {
  error?: {
    code?: string;
    message?: string;
  };
}

async function request<T>(endpoint: string, options: RequestInit = {}, expectedStatus?: number): Promise<T> {
  const headers = new Headers(options.headers || {});
  headers.set("Accept", "application/json");
  headers.set("Content-Type", "application/json");
  const method = (options.method || "GET").toUpperCase();
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    const token = getCSRFToken();
    if (token) headers.set("X-CSRF-Token", token);
  }
  const response = await fetch(endpoint, {
    ...options,
    headers,
    credentials: "same-origin",
  });
  if (expectedStatus !== undefined && response.status !== expectedStatus) {
    throw Object.assign(new Error("Unexpected response status"), { status: response.status });
  }
  let payload: ApiEnvelope<T> | ApiFailure;
  try {
    payload = await response.json() as ApiEnvelope<T> | ApiFailure;
  } catch {
    throw new Error(`The server returned an invalid response (${response.status}).`);
  }
  if (!response.ok) {
    const failure = payload as ApiFailure;
    const error = new Error(failure.error?.message || `Request failed with status ${response.status}`) as Error & {
      code?: string;
      status?: number;
    };
    error.code = failure.error?.code;
    error.status = response.status;
    throw error;
  }
  return (payload as ApiEnvelope<T>).data;
}

export async function sendStaffInvitation(
  slug: string,
  invitation: { email: string; role: StaffInvitationRole },
): Promise<StaffInvitationResult> {
  try {
    const receipt = await request<{
      id: string;
      email: string;
      role: StaffInvitationRole;
      expires_at: string;
    }>(`/api/v1/tenants/${encodeURIComponent(slug)}/staff-invitations`, {
      method: "POST",
      body: JSON.stringify({ email: invitation.email, role: invitation.role }),
    }, 201);

    return typeof receipt?.id === "string" && receipt.id.length > 0
      && receipt.email === invitation.email.toLowerCase()
      && receipt.role === invitation.role
      && typeof receipt.expires_at === "string" && Number.isFinite(Date.parse(receipt.expires_at))
      ? "sent" : "unconfirmed";
  } catch (error: unknown) {
    const status = error instanceof Error && "status" in error ? error.status : undefined;
    switch (status) {
      case 429: return "rate-limited";
      case 422: return "invalid";
      case 409: return "conflict";
      case 401: return "unauthenticated";
      case 403:
      case 404: return "forbidden";
      default: return "unconfirmed";
    }
  }
}

export async function verifyStaffInvitationAccess(slug: string, signal: AbortSignal) {
  const options = { cache: "no-store" as const, signal };
  const [school, session] = await Promise.all([
    request<Tenant>(`/api/v1/tenants/${encodeURIComponent(slug)}/settings`, options, 200),
    request<{ csrf_token: string }>("/api/v1/auth/me", options, 200),
  ]);
  return { school, csrfToken: session?.csrf_token };
}

export async function provisionTenant(requestBody: ProvisionTenantRequest): Promise<Tenant> {
  return request<Tenant>("/api/v1/platform/tenants", {
    method: "POST",
    body: JSON.stringify(requestBody),
  });
}

export async function loadCapturedMessages(): Promise<CapturedMessage[]> {
  const result = await request<{ messages: CapturedMessage[] }>("/api/v1/platform/mail-capture");
  return result.messages;
}

export async function updateTenantSettings(slug: string, requestBody: TenantSettingsRequest): Promise<Tenant> {
  return request<Tenant>(`/api/v1/tenants/${encodeURIComponent(slug)}/settings`, {
    method: "PATCH",
    body: JSON.stringify(requestBody),
  });
}

export function establishCSRFToken(token: string): void {
  setCSRFToken(token);
}

export async function listMySchools(): Promise<Tenant[]> {
  const result = await request<{ schools: Tenant[] }>("/api/v1/my/schools");
  return result.schools;
}
