import React from "react";
import ApiReadiness from "@/components/api-readiness";

export default function Home() {
  return (
    <main className="min-h-screen flex flex-col justify-between p-6 sm:p-12 max-w-5xl mx-auto">
      <header className="border-b border-slate-200 pb-6 mb-8 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <span className="inline-block px-3 py-1 bg-green-100 text-green-800 text-xs font-semibold rounded-full mb-2">
            MVP Baseline • SWW-001
          </span>
          <h1 className="text-3xl sm:text-4xl font-extrabold text-slate-900 tracking-tight">
            School Without Walls
          </h1>
          <p className="text-slate-600 text-sm sm:text-base mt-1">
            Configurable, multi-tenant digital school operating system and learning continuity platform.
          </p>
        </div>
        <ApiReadiness />
      </header>

      <section className="grid grid-cols-1 md:grid-cols-2 gap-6 my-auto">
        <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm">
          <h2 className="text-lg font-bold text-slate-900 mb-2">
            Multi-Tenant Educational Architecture
          </h2>
          <p className="text-sm text-slate-600 mb-4">
            Authoritative PostgreSQL state with strict composite tenant keys and row-level authorization.
            Zero cross-school data leakage by design.
          </p>
          <ul className="text-xs text-slate-500 space-y-1">
            <li>✓ Go REST API Monolith (`/api/v1`)</li>
            <li>✓ Next.js App Router with Same-Origin Proxy</li>
            <li>✓ Standard JSON Envelopes: <code className="bg-slate-100 px-1 py-0.5 rounded text-slate-800">&#123; data, meta &#125;</code></li>
          </ul>
        </div>

        <div className="bg-white p-6 rounded-xl border border-slate-200 shadow-sm">
          <h2 className="text-lg font-bold text-slate-900 mb-2">
            Curriculum & Stages
          </h2>
          <p className="text-sm text-slate-600 mb-4">
            Nigerian-first NERDC basic curriculum baseline with optional, version-tracked British enrichment.
          </p>
          <div className="grid grid-cols-2 gap-2 text-xs font-medium">
            <div className="bg-slate-50 p-2 rounded border border-slate-200 text-center text-slate-700">
              Early Years (EYFS)
            </div>
            <div className="bg-slate-50 p-2 rounded border border-slate-200 text-center text-slate-700">
              Primary (1-6)
            </div>
            <div className="bg-slate-50 p-2 rounded border border-slate-200 text-center text-slate-700">
              Junior Secondary (JSS)
            </div>
            <div className="bg-slate-50 p-2 rounded border border-slate-200 text-center text-slate-700">
              Senior Secondary (SS)
            </div>
          </div>
        </div>
      </section>

      <footer className="border-t border-slate-200 pt-6 mt-8 flex flex-col sm:flex-row items-center justify-between text-xs text-slate-500 gap-2">
        <p>© 2026 School Without Walls. All rights reserved.</p>
        <div className="flex gap-4">
          <a href="/api/health" className="hover:text-slate-800 underline">
            Web Health Probe
          </a>
          <span className="text-slate-300">|</span>
          <span className="text-slate-600">Timezone: Africa/Lagos</span>
        </div>
      </footer>
    </main>
  );
}
