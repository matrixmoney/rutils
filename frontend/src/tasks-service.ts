// ============================================================
// TASKS-SERVICE.TS — API client for the /tasks endpoints
// ============================================================

import { apiGet } from './auth';

// ─── Types ─────────────────────────────────────────────────

export interface Task {
  id: string;
  text: string;
  due_date: string;   // YYYY-MM-DD
  created_at: string; // RFC3339
  updated_at: string; // RFC3339
}

export interface TasksListResponse {
  success: boolean;
  tasks: Task[];
}

export interface TaskResponse {
  success: boolean;
  task: Task;
}

export interface TaskMessageResponse {
  success: boolean;
  message: string;
}

// ─── API base resolution (mirrors auth.ts / formatters.ts) ─

function resolveBase(): string {
  const isLocal =
    window.location.hostname === 'localhost' ||
    window.location.hostname === '127.0.0.1';
  if (isLocal) {
    return window.location.port === '8080' ? '' : 'http://localhost:8080';
  }
  return 'https://utils.api.srilakshmiretail.in';
}

function authHeaders(token?: string): HeadersInit {
  const h: HeadersInit = { 'Content-Type': 'application/json' };
  if (token) (h as Record<string, string>)['Authorization'] = `Bearer ${token}`;
  return h;
}

async function unwrap<T>(res: Response): Promise<T> {
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as { error?: string }).error ?? `Request failed (${res.status})`);
  }
  return data as T;
}

// ─── API calls ─────────────────────────────────────────────

/** GET /tasks — every saved task, newest first. */
export async function fetchTasks(token?: string): Promise<TasksListResponse> {
  return apiGet<TasksListResponse>('/tasks', token);
}

/** POST /tasks — creates a new task. */
export async function createTask(
  text: string,
  dueDate: string,
  token?: string,
): Promise<TaskResponse> {
  const res = await fetch(`${resolveBase()}/tasks`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify({ text, due_date: dueDate }),
  });
  return unwrap<TaskResponse>(res);
}

/** PUT /tasks/{id} — updates an existing task. */
export async function updateTask(
  id: string,
  text: string,
  dueDate: string,
  token?: string,
): Promise<TaskResponse> {
  const res = await fetch(`${resolveBase()}/tasks/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authHeaders(token),
    body: JSON.stringify({ text, due_date: dueDate }),
  });
  return unwrap<TaskResponse>(res);
}

/** DELETE /tasks/{id} — removes a task. */
export async function deleteTask(
  id: string,
  token?: string,
): Promise<TaskMessageResponse> {
  const res = await fetch(`${resolveBase()}/tasks/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authHeaders(token),
  });
  return unwrap<TaskMessageResponse>(res);
}