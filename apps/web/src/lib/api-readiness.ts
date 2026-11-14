export type ApiReadinessState = "ready" | "unavailable" | "timeout";

interface ReadinessResponse {
  data?: {
    status?: string;
  };
}

export async function checkApiReadiness(
  apiBase: string,
  timeoutMs = 2000,
  fetcher: typeof fetch = fetch,
): Promise<ApiReadinessState> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetcher(new URL("/ready", apiBase), {
      cache: "no-store",
      signal: controller.signal,
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      return "unavailable";
    }

    const body = await response.json() as ReadinessResponse;
    return body.data?.status === "ready" ? "ready" : "unavailable";
  } catch {
    return controller.signal.aborted ? "timeout" : "unavailable";
  } finally {
    clearTimeout(timeout);
  }
}
