import { useEffect, useState } from 'react';
import { fetchTools, type Tool } from '../api/client';
import { ToolTile } from '../components/ToolTile';

export function ToolList() {
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    fetchTools()
      .then((data) => {
        if (!cancelled) {
          setTools(data);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError('Failed to load tools. Please try again.');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-950">
        <svg
          className="h-10 w-10 animate-spin text-cyan-400"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
          data-testid="page-spinner"
        >
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
        </svg>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-950">
        <div className="rounded-2xl border border-red-800/50 bg-red-900/20 p-6 text-center">
          <p className="text-red-400">{error}</p>
        </div>
      </div>
    );
  }

  if (tools.length === 0) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-950">
        <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-8 text-center">
          <p className="text-lg text-slate-400">No tools available for your account.</p>
        </div>
      </div>
    );
  }

  return (
    <main className="min-h-screen bg-slate-950 px-6 py-16">
      <section className="mx-auto max-w-5xl">
        <div className="mb-10 space-y-2">
          <p className="text-sm font-semibold uppercase tracking-[0.28em] text-cyan-300">
            IT Tools Portal
          </p>
          <h1 className="text-3xl font-semibold tracking-tight text-white">
            Your Tools
          </h1>
          <p className="text-slate-400">
            Click a tool to open it with single sign-on.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
          {tools.map((tool) => (
            <ToolTile key={tool.id} tool={tool} />
          ))}
        </div>
      </section>
    </main>
  );
}
