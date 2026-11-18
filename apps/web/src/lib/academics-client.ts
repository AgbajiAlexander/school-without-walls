"use client";

import { getCSRFToken } from "@/lib/auth-client";

export type SessionStatus = "DRAFT" | "ACTIVE" | "ARCHIVED";

export interface AcademicSession {
  id: string;
  name: string;
  starts_on: string;
  ends_on: string;
  status: SessionStatus;
  created_at: string;
  updated_at: string;
}

export interface AcademicTerm {
  id: string;
  session_id: string;
  sequence: number;
  name: string;
  starts_on: string;
  ends_on: string;
  created_at: string;
  updated_at: string;
}

export interface SessionInput {
  name: string;
  starts_on: string;
  ends_on: string;
}

export interface TermInput {
  sequence: number;
  name: string;
  starts_on: string;
  ends_on: string;
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

function schoolPath(slug: string): string {
  if (!slug.trim()) {
    throw new Error("A school identifier is required.");
  }

  return `/api/v1/tenants/${encodeURIComponent(slug)}`;
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const method = (options.method ?? "GET").toUpperCase();
  const headers = new Headers(options.headers);

  headers.set("Accept", "application/json");

  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json");
  }

  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    const token = getCSRFToken();

    if (!token) {
      throw new Error("Session verification is required before saving.");
    }

    headers.set("X-CSRF-Token", token);
  }

  const response = await fetch(path, {
    ...options,
    headers,
    credentials: "same-origin",
    cache: "no-store",
  });

  let payload: ApiEnvelope<T> | ApiFailure;

  try {
    payload = await response.json() as ApiEnvelope<T> | ApiFailure;
  } catch {
    throw new Error(
      `The server returned an invalid response (${response.status}).`,
    );
  }

  if (!response.ok) {
    const failure = payload as ApiFailure;
    const error = new Error(
      failure.error?.message ??
      `Request failed with status ${response.status}`,
    ) as Error & { status?: number; code?: string };

    error.status = response.status;
    error.code = failure.error?.code;
    throw error;
  }

  if (!("data" in payload)) {
    throw new Error("The server response is missing its data envelope.");
  }

  return payload.data;
}

export async function listAcademicSessions(
  slug: string,
): Promise<AcademicSession[]> {
  const data = await request<{ academic_sessions: AcademicSession[] }>(
    `${schoolPath(slug)}/academic-sessions`,
  );

  if (!Array.isArray(data.academic_sessions)) {
    throw new Error("Invalid academic sessions response.");
  }

  return data.academic_sessions;
}

export async function listAcademicTerms(
  slug: string,
  sessionID: string,
): Promise<AcademicTerm[]> {
  const data = await request<{ terms: AcademicTerm[] }>(
    `${schoolPath(slug)}/academic-sessions/${encodeURIComponent(sessionID)}/terms`,
  );

  if (!Array.isArray(data.terms)) {
    throw new Error("Invalid academic terms response.");
  }

  return data.terms;
}

export function createAcademicSession(
  slug: string,
  input: SessionInput,
): Promise<AcademicSession> {
  return request<AcademicSession>(
    `${schoolPath(slug)}/academic-sessions`,
    { method: "POST", body: JSON.stringify(input) },
  );
}

export async function configureAcademicTerms(
  slug: string,
  sessionID: string,
  terms: TermInput[],
): Promise<AcademicTerm[]> {
  const data = await request<{ terms: AcademicTerm[] }>(
    `${schoolPath(slug)}/academic-sessions/${encodeURIComponent(sessionID)}/terms`,
    { method: "PUT", body: JSON.stringify({ terms }) },
  );

  if (!Array.isArray(data.terms)) {
    throw new Error("Invalid academic terms response.");
  }

  return data.terms;
}

export function activateAcademicSession(
  slug: string,
  sessionID: string,
): Promise<AcademicSession> {
  return request<AcademicSession>(
    `${schoolPath(slug)}/academic-sessions/${encodeURIComponent(sessionID)}/activate`,
    { method: "POST" },
  );
}

export function archiveAcademicSession(
  slug: string,
  sessionID: string,
): Promise<AcademicSession> {
  return request<AcademicSession>(
    `${schoolPath(slug)}/academic-sessions/${encodeURIComponent(sessionID)}/archive`,
    { method: "POST" },
  );
}