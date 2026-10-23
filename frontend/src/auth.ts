// ============================================================
// AUTH.TS — Authentication API client & state manager
// ============================================================

export interface UserResponse {
  id: string;
  name: string;
  email: string;
  must_reset_password: boolean;
  created_at: string;
}

export interface AuthState {
  token: string | null;
  user: UserResponse | null;
}

// ─── Storage helpers ────────────────────────────────────────
const TOKEN_KEY = 'rutils_token';
const USER_KEY  = 'rutils_user';

export function saveAuth(token: string, user: UserResponse): void {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearAuth(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

export function getAuthState(): AuthState {
  const token = localStorage.getItem(TOKEN_KEY);
  const rawUser = localStorage.getItem(USER_KEY);
  const user = rawUser ? (JSON.parse(rawUser) as UserResponse) : null;
  return { token, user };
}

export function isLoggedIn(): boolean {
  return !!localStorage.getItem(TOKEN_KEY);
}

// ─── API base ────────────────────────────────────────────────
export function getApiBase(): string {
  const isLocal =
    window.location.hostname === 'localhost' ||
    window.location.hostname === '127.0.0.1';
  if (isLocal) {
    if (window.location.port === '8080') return '';
    return 'http://localhost:8080';
  }
  return 'https://utils.api.srilakshmiretail.in';
}

export async function apiPost<T>(path: string, body: unknown, token?: string): Promise<T> {
  const headers: HeadersInit = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${getApiBase()}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });

  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    throw new Error((data as { error?: string }).error ?? `Request failed (${res.status})`);
  }
  return data as T;
}

export async function apiGet<T>(path: string, token?: string): Promise<T> {
  const headers: HeadersInit = {};
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(`${getApiBase()}${path}`, { method: 'GET', headers });
  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    throw new Error((data as { error?: string }).error ?? `Request failed (${res.status})`);
  }
  return data as T;
}

// ─── Auth API calls ──────────────────────────────────────────
export interface LoginResponse {
  token: string;
  user: UserResponse;
  must_reset_password: boolean;
  message: string;
}

export async function apiRegister(name: string, email: string, password: string, confirmPassword: string): Promise<UserResponse> {
  return apiPost<UserResponse>('/auth/register', {
    name, email, password, confirm_password: confirmPassword,
  });
}

export async function apiLogin(email: string, password: string): Promise<LoginResponse> {
  return apiPost<LoginResponse>('/auth/login', { email, password });
}

export async function apiLogout(token: string): Promise<void> {
  await apiPost('/auth/logout', {}, token);
}

export async function apiForgotPassword(email: string): Promise<void> {
  await apiPost('/auth/forgot-password', { email });
}

export async function apiResetPassword(token: string, newPassword: string, confirmPassword: string): Promise<void> {
  await apiPost('/auth/reset-password', {
    new_password: newPassword, confirm_password: confirmPassword,
  }, token);
}

export async function apiChangePassword(token: string, currentPassword: string, newPassword: string, confirmPassword: string): Promise<void> {
  await apiPost('/auth/change-password', {
    current_password: currentPassword,
    new_password: newPassword,
    confirm_password: confirmPassword,
  }, token);
}

export async function apiGetMe(token: string): Promise<UserResponse> {
  return apiGet<UserResponse>('/auth/me', token);
}

