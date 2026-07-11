export interface Tool {
  id: string;
  name: string;
  base_url: string;
  icon_url: string;
  allowed_groups: string[];
  health_check_url?: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface ExchangeResponse {
  launch_url: string;
}

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'same-origin',
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });

  if (res.status === 401) {
    window.location.href = '/auth/login';
    throw new ApiError('Unauthorized', 401);
  }

  if (!res.ok) {
    throw new ApiError(`Request failed: ${res.status}`, res.status);
  }

  return res.json() as Promise<T>;
}

export function fetchTools(): Promise<Tool[]> {
  return request<Tool[]>('/api/tools');
}

export function exchangeToken(toolId: string): Promise<ExchangeResponse> {
  return request<ExchangeResponse>('/api/auth/exchange', {
    method: 'POST',
    body: JSON.stringify({ tool_id: toolId }),
  });
}
