import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ToolList } from './ToolList';

const mockTools = [
  {
    id: 'tool-1',
    name: 'Status Monitor',
    base_url: 'https://status.example.com',
    icon_url: 'https://example.com/icon.png',
    allowed_groups: ['group-a'],
    health_check_url: null,
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'tool-2',
    name: 'CMS',
    base_url: 'https://cms.example.com',
    icon_url: 'https://example.com/cms.png',
    allowed_groups: ['group-a'],
    health_check_url: null,
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
];

// Mock fetch globally
const fetchSpy = vi.fn();

beforeEach(() => {
  vi.restoreAllMocks();
  vi.stubGlobal('fetch', fetchSpy);
});

describe('ToolList', () => {
  it('shows loading spinner initially', () => {
    fetchSpy.mockReturnValue(new Promise(() => {})); // Never resolves
    render(<ToolList />);
    expect(screen.getByTestId('page-spinner')).toBeInTheDocument();
  });

  it('renders tool tiles on success', async () => {
    fetchSpy.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockTools),
    });

    render(<ToolList />);

    await waitFor(() => {
      expect(screen.getByText('Status Monitor')).toBeInTheDocument();
      expect(screen.getByText('CMS')).toBeInTheDocument();
    });
  });

  it('shows empty state when no tools returned', async () => {
    fetchSpy.mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve([]),
    });

    render(<ToolList />);

    await waitFor(() => {
      expect(screen.getByText('No tools available for your account.')).toBeInTheDocument();
    });
  });

  it('shows error state when fetch fails', async () => {
    fetchSpy.mockResolvedValue({
      ok: false,
      status: 500,
      json: () => Promise.resolve({ error: 'internal' }),
    });

    render(<ToolList />);

    await waitFor(() => {
      expect(screen.getByText('Failed to load tools. Please try again.')).toBeInTheDocument();
    });
  });

  it('launches tool on tile click', async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockTools),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ launch_url: 'https://status.example.com/sso-callback?token=abc' }),
      });

    const openSpy = vi.fn();
    vi.stubGlobal('open', openSpy);

    const user = userEvent.setup();
    render(<ToolList />);

    await waitFor(() => {
      expect(screen.getByText('Status Monitor')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Status Monitor'));

    await waitFor(() => {
      expect(openSpy).toHaveBeenCalledWith(
        'https://status.example.com/sso-callback?token=abc',
        '_blank',
        'noopener,noreferrer',
      );
    });
  });

  it('shows per-tile error when exchange fails', async () => {
    fetchSpy
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(mockTools),
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ error: 'forbidden' }),
      });

    const user = userEvent.setup();
    render(<ToolList />);

    await waitFor(() => {
      expect(screen.getByText('Status Monitor')).toBeInTheDocument();
    });

    await user.click(screen.getByText('Status Monitor'));

    await waitFor(() => {
      expect(screen.getByTestId('tile-error')).toHaveTextContent('Failed to launch tool. Please try again.');
    });
  });

  it('redirects to login on 401', async () => {
    const assignSpy = vi.fn();
    Object.defineProperty(window, 'location', {
      value: { href: '/', assign: assignSpy },
      writable: true,
    });

    fetchSpy.mockResolvedValue({
      ok: false,
      status: 401,
      json: () => Promise.resolve({ error: 'unauthorized' }),
    });

    render(<ToolList />);

    await waitFor(() => {
      expect(window.location.href).toBe('/auth/login');
    });
  });
});
