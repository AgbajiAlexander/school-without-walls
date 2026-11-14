"use client";

import { useEffect, useState } from "react";
import type { ApiReadinessState } from "@/lib/api-readiness";

type ReadinessStatus = "checking" | ApiReadinessState;

const STATUS_COPY: Record<ReadinessStatus, string> = {
  checking: "Checking API…",
  ready: "API ready",
  unavailable: "API unavailable",
  timeout: "Request timed out",
};

const STATUS_COLOR: Record<ReadinessStatus, string> = {
  checking: "bg-slate-400 animate-pulse",
  ready: "bg-emerald-500",
  unavailable: "bg-red-500",
  timeout: "bg-amber-500",
};

export default function ApiReadiness() {
  const [status, setStatus] = useState<ReadinessStatus>("checking");

  useEffect(() => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 4000);

    fetch("/api/readiness", {
      cache: "no-store",
      signal: controller.signal,
      headers: { Accept: "application/json" },
    })
      .then(async (response) => {
        const body = await response.json() as { status?: ReadinessStatus };
        if (body.status === "ready" && response.ok) {
          setStatus("ready");
        } else if (body.status === "timeout" || response.status === 504) {
          setStatus("timeout");
        } else {
          setStatus("unavailable");
        }
      })
      .catch(() => {
        setStatus(controller.signal.aborted ? "timeout" : "unavailable");
      })
      .finally(() => clearTimeout(timeout));

    return () => {
      clearTimeout(timeout);
      controller.abort();
    };
  }, []);

  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy={status === "checking"}
      className="flex items-center gap-2"
    >
      <span className={`h-3 w-3 rounded-full ${STATUS_COLOR[status]}`} aria-hidden="true" />
      <span className="text-xs font-medium text-slate-700">{STATUS_COPY[status]}</span>
    </div>
  );
}
