/**
 * Standard typed API response envelope matching Go backend contracts.
 */
export interface ApiResponse<T> {
  data: T;
  meta: {
    request_id: string;
  };
}

export interface ApiError {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
    request_id: string;
  };
}

export interface HealthStatus {
  status: string;
  service: string;
  version: string;
  environment: string;
  uptime_seconds: number;
}

export interface ReadyStatus {
  status: string;
  database: string;
}

/**
 * Basic fetch wrapper with error handling for standard JSON envelopes.
 */
export async function apiFetch<T>(
  endpoint: string,
  init?: RequestInit
): Promise<ApiResponse<T>> {
  const res = await fetch(endpoint, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      ...init?.headers,
    },
  });

  const body = await res.json();

  if (!res.ok) {
    const errorBody = body as ApiError;
    throw new Error(
      errorBody.error?.message || `Request failed with status ${res.status}`
    );
  }

  return body as ApiResponse<T>;
}
