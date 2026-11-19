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

interface ApiEnvelope<T> {
  data: T;
}

interface ApiFailure {
  error?: {
    code?: string;
    message?: string;
  };
}

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
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