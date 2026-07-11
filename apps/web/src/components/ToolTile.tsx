import { type Tool, exchangeToken, type ApiError } from '../api/client';
import { useState } from 'react';

interface ToolTileProps {
  tool: Tool;
}

export function ToolTile({ tool }: ToolTileProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleLaunch() {
    setLoading(true);
    setError(null);

    try {
      const { launch_url } = await exchangeToken(tool.id);
      window.open(launch_url, '_blank', 'noopener,noreferrer');
    } catch (err) {
      const apiErr = err as ApiError;
      if (apiErr.status !== 401) {
        setError('Failed to launch tool. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <button
      type="button"
      onClick={handleLaunch}
      disabled={loading}
      className="group relative flex flex-col items-center gap-3 rounded-2xl border border-slate-800 bg-slate-900/60 p-6 text-center transition hover:border-cyan-500/50 hover:bg-slate-800/60 disabled:opacity-60"
    >
      {loading && (
        <div className="absolute inset-0 z-10 flex items-center justify-center rounded-2xl bg-slate-900/80">
          <svg
            className="h-8 w-8 animate-spin text-cyan-400"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            data-testid="spinner"
          >
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
          </svg>
        </div>
      )}

      <img
        src={tool.icon_url}
        alt=""
        className="h-12 w-12 rounded-lg object-contain"
        onError={(e) => {
          (e.target as HTMLImageElement).style.display = 'none';
        }}
      />

      <span className="text-sm font-medium text-white group-hover:text-cyan-300">
        {tool.name}
      </span>

      {error && (
        <span className="text-xs text-red-400" data-testid="tile-error">
          {error}
        </span>
      )}
    </button>
  );
}
