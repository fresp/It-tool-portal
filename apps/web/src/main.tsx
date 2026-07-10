import React from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

const rootElement = document.getElementById('root');

if (rootElement === null) {
  throw new Error('Root element not found');
}

createRoot(rootElement).render(
  <React.StrictMode>
    <main className="min-h-screen bg-slate-950 px-6 py-16 text-slate-100">
      <section className="mx-auto flex max-w-4xl flex-col gap-8 rounded-3xl border border-slate-800 bg-slate-900/70 p-10 shadow-2xl shadow-slate-950/60">
        <div className="space-y-3">
          <p className="text-sm font-semibold uppercase tracking-[0.28em] text-cyan-300">
            IT Tools Portal
          </p>
          <h1 className="text-4xl font-semibold tracking-tight text-white md:text-6xl">
            SSO app launcher scaffold
          </h1>
          <p className="max-w-2xl text-lg leading-8 text-slate-300">
            This placeholder confirms the React, Vite, Tailwind CSS, and Go embedded asset pipeline is wired for future tool-list work.
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <div className="rounded-2xl border border-slate-800 bg-slate-950/60 p-5">
            <p className="text-sm text-slate-400">Frontend</p>
            <p className="mt-2 text-xl font-semibold text-white">React 18 + Vite</p>
          </div>
          <div className="rounded-2xl border border-slate-800 bg-slate-950/60 p-5">
            <p className="text-sm text-slate-400">Backend</p>
            <p className="mt-2 text-xl font-semibold text-white">Go + Gin</p>
          </div>
          <div className="rounded-2xl border border-slate-800 bg-slate-950/60 p-5">
            <p className="text-sm text-slate-400">Runtime</p>
            <p className="mt-2 text-xl font-semibold text-white">Single binary</p>
          </div>
        </div>
      </section>
    </main>
  </React.StrictMode>,
);
