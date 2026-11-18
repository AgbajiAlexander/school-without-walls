"use client";

import { getCSRFToken } from "@/lib/auth-client";

export type EducationStage =
  | "EARLY_YEARS"
  | "PRIMARY"
  | "JUNIOR_SECONDARY"
  | "SENIOR_SECONDARY";

export type StructureStatus = "ACTIVE" | "ARCHIVED";

export interface GradeLevel {
  id: string;
  stage: EducationStage;
  code: string;
  name: string;
  sort_order: number;
  status: StructureStatus;
  created_at: string;
  updated_at: string;
}

export interface SchoolClass {
  id: string;
  session_id: string;
  grade_level_id: string;
  code: string;
  name: string;
  status: StructureStatus;
  created_at: string;
  updated_at: string;
}

export interface ClassArm {
  id: string;
  class_id: string;
  code: string;
  name: string;
  status: StructureStatus;
  created_at: string;
  updated_at: string;
}

export interface GradeLevelInput {
  stage: EducationStage;
  code: string;
  name: string;
  sort_order: number;
}

export type GradeLevelUpdate = Partial<GradeLevelInput>;

export interface ClassInput {
  session_id: string;
  grade_level_id: string;
  code: string;
  name: string;
}

export type ClassUpdate = Partial<Pick<ClassInput, "code" | "name">>;

export interface ClassArmInput {
  class_id: string;
  code: string;
  name: string;
}

export type ClassArmUpdate = Partial<Pick<ClassArmInput, "code" | "name">>;

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

function resourceID(value: string, label: string): string {
  if (!value.trim()) {
    throw new Error(`${label} is required.`);
  }

  return encodeURIComponent(value);
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
    payload = (await response.json()) as ApiEnvelope<T> | ApiFailure;
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

async function listResource<T>(
  path: string,
  property: string,
): Promise<T[]> {
  const data = await request<Record<string, unknown>>(path);
  const values = data?.[property];

  if (!Array.isArray(values)) {
    throw new Error(`Invalid ${property.replaceAll("_", " ")} response.`);
  }

  return values as T[];
}

export function listGradeLevels(slug: string): Promise<GradeLevel[]> {
  return listResource<GradeLevel>(
    `${schoolPath(slug)}/grade-levels`,
    "grade_levels",
  );
}

export function createGradeLevel(
  slug: string,
  input: GradeLevelInput,
): Promise<GradeLevel> {
  return request<GradeLevel>(
    `${schoolPath(slug)}/grade-levels`,
    { method: "POST", body: JSON.stringify(input) },
  );
}

export function updateGradeLevel(
  slug: string,
  id: string,
  input: GradeLevelUpdate,
): Promise<GradeLevel> {
  return request<GradeLevel>(
    `${schoolPath(slug)}/grade-levels/${resourceID(id, "Grade level ID")}`,
    { method: "PATCH", body: JSON.stringify(input) },
  );
}

export function archiveGradeLevel(
  slug: string,
  id: string,
): Promise<GradeLevel> {
  return request<GradeLevel>(
    `${schoolPath(slug)}/grade-levels/${resourceID(id, "Grade level ID")}/archive`,
    { method: "POST" },
  );
}

export function listClasses(
  slug: string,
  sessionID: string,
): Promise<SchoolClass[]> {
  const query = `session_id=${resourceID(sessionID, "Session ID")}`;

  return listResource<SchoolClass>(
    `${schoolPath(slug)}/classes?${query}`,
    "classes",
  );
}

export function createClass(
  slug: string,
  input: ClassInput,
): Promise<SchoolClass> {
  return request<SchoolClass>(
    `${schoolPath(slug)}/classes`,
    { method: "POST", body: JSON.stringify(input) },
  );
}

export function updateClass(
  slug: string,
  id: string,
  input: ClassUpdate,
): Promise<SchoolClass> {
  return request<SchoolClass>(
    `${schoolPath(slug)}/classes/${resourceID(id, "Class ID")}`,
    { method: "PATCH", body: JSON.stringify(input) },
  );
}

export function archiveClass(
  slug: string,
  id: string,
): Promise<SchoolClass> {
  return request<SchoolClass>(
    `${schoolPath(slug)}/classes/${resourceID(id, "Class ID")}/archive`,
    { method: "POST" },
  );
}

export function listClassArms(
  slug: string,
  classID: string,
): Promise<ClassArm[]> {
  const query = `class_id=${resourceID(classID, "Class ID")}`;

  return listResource<ClassArm>(
    `${schoolPath(slug)}/class-arms?${query}`,
    "class_arms",
  );
}

export function createClassArm(
  slug: string,
  input: ClassArmInput,
): Promise<ClassArm> {
  return request<ClassArm>(
    `${schoolPath(slug)}/class-arms`,
    { method: "POST", body: JSON.stringify(input) },
  );
}

export function updateClassArm(
  slug: string,
  id: string,
  input: ClassArmUpdate,
): Promise<ClassArm> {
  return request<ClassArm>(
    `${schoolPath(slug)}/class-arms/${resourceID(id, "Class arm ID")}`,
    { method: "PATCH", body: JSON.stringify(input) },
  );
}

export function archiveClassArm(
  slug: string,
  id: string,
): Promise<ClassArm> {
  return request<ClassArm>(
    `${schoolPath(slug)}/class-arms/${resourceID(id, "Class arm ID")}/archive`,
    { method: "POST" },
  );
}