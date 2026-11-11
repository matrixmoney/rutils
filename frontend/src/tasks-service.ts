// ============================================================
// TASKS-SERVICE.TS — API client + export helpers for /tasks
// ============================================================

import { apiGet } from './auth';

// ─── Types ─────────────────────────────────────────────────

export interface Task {
  id: string;
  /**
   * Owner of the task. Populated server-side from the JWT; never
   * sent in a request body. Included here so the client-side
   * export can (optionally) surface it in the CSV.
   */
  user_id: string;
  text: string;
  due_date: string;   // YYYY-MM-DD
  created_at: string; // RFC3339
  updated_at: string; // RFC3339

  // ── Reminder fields (new) ─────────────────────────────────
  reminder_enabled: boolean;
  reminder_start_date: string;
  reminder_time: string;
  reminder_timezone: string;
  last_reminder_sent_at?: string;
}

export interface TasksListResponse {
  success: boolean;
  tasks: Task[];
}

export interface TaskResponse {
  success: boolean;
  task: Task;
  email_sent: boolean;
}

export interface TaskMessageResponse {
  success: boolean;
  message: string;
  email_sent?: boolean;
}

export interface ReminderFields {
  reminder_enabled?: boolean;
  reminder_start_date?: string; // YYYY-MM-DD, or "" to reset
  reminder_time?: string;       // "HH:MM", or "" to reset
  reminder_timezone?: string;   // IANA, or "" to reset
}

// ─── Custom Error Class ─────────────────────────────────────
// This allows the UI layer to specifically catch 401 Unauthorized 
// errors and prompt the user to log in, rather than failing silently.
export class UnauthorizedError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'UnauthorizedError';
  }
}

// ─── API base resolution ────────────────────────────────────

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
    const msg = (data as { error?: string }).error ?? `Request failed (${res.status})`;
    if (res.status === 401) {
      throw new UnauthorizedError(msg);
    }
    throw new Error(msg);
  }
  return data as T;
}

// ─── API calls ─────────────────────────────────────────────

/** GET /tasks — every task owned by the caller, newest first. */
export async function fetchTasks(token?: string): Promise<TasksListResponse> {
  try {
    return await apiGet<TasksListResponse>('/tasks', token);
  } catch (err: any) {
    // apiGet from auth.ts throws a generic Error. We intercept it
    // here to see if it was a 401, so we can throw our custom class.
    if (err.message && err.message.includes('Unauthorized')) {
       throw new UnauthorizedError(err.message);
    }
    throw err;
  }
}

/**
 * POST /tasks — creates a new task owned by the caller.
 */
export async function createTask(
  text: string,
  dueDate: string,
  token?: string,
  sendEmail: boolean = true,
  reminder: ReminderFields = {},
): Promise<TaskResponse> {
  const payload: Record<string, unknown> = {
    text,
    due_date: dueDate,
    send_email: sendEmail,
  };

  if ('reminder_enabled' in reminder) payload['reminder_enabled'] = reminder.reminder_enabled;
  if ('reminder_start_date' in reminder) payload['reminder_start_date'] = reminder.reminder_start_date;
  if ('reminder_time' in reminder) payload['reminder_time'] = reminder.reminder_time;
  if ('reminder_timezone' in reminder) payload['reminder_timezone'] = reminder.reminder_timezone;

  const res = await fetch(`${resolveBase()}/tasks`, {
    method: 'POST',
    headers: authHeaders(token),
    body: JSON.stringify(payload),
  });
  return unwrap<TaskResponse>(res);
}

/**
 * PUT /tasks/{id} — updates an existing task owned by the caller.
 */
export async function updateTask(
  id: string,
  text: string,
  dueDate: string,
  token?: string,
  sendEmail: boolean = false,
  reminder: ReminderFields = {},
): Promise<TaskResponse> {
  const payload: Record<string, unknown> = {
    text,
    due_date: dueDate,
    send_email: sendEmail,
  };

  if ('reminder_enabled' in reminder) payload['reminder_enabled'] = reminder.reminder_enabled;
  if ('reminder_start_date' in reminder) payload['reminder_start_date'] = reminder.reminder_start_date;
  if ('reminder_time' in reminder) payload['reminder_time'] = reminder.reminder_time;
  if ('reminder_timezone' in reminder) payload['reminder_timezone'] = reminder.reminder_timezone;

  const res = await fetch(`${resolveBase()}/tasks/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: authHeaders(token),
    body: JSON.stringify(payload),
  });
  return unwrap<TaskResponse>(res);
}

/**
 * DELETE /tasks/{id} — removes a task owned by the caller.
 */
export async function deleteTask(
  id: string,
  token?: string,
  sendEmail: boolean = true,
): Promise<TaskMessageResponse> {
  const query = sendEmail ? '?send_email=true' : '?send_email=false';
  const res = await fetch(
    `${resolveBase()}/tasks/${encodeURIComponent(id)}${query}`,
    {
      method: 'DELETE',
      headers: authHeaders(token),
    },
  );
  return unwrap<TaskMessageResponse>(res);
}

// ─── Export helpers (client-side only) ────────────────────

export type ExportFormat = 'csv' | 'json';

export function exportTasks(tasks: Task[], format: ExportFormat): Blob {
  if (format === 'json') {
    const payload = JSON.stringify(tasks, null, 2);
    return new Blob([payload], { type: 'application/json;charset=utf-8' });
  }

  const headers = ['id', 'text', 'due_date', 'created_at', 'updated_at'];
  const rows: string[] = [headers.map(csvEscape).join(',')];

  for (const t of tasks) {
    rows.push(
      [
        t.id,
        t.text,
        t.due_date,
        t.created_at,
        t.updated_at,
      ]
        .map(csvEscape)
        .join(','),
    );
  }

  const body = rows.join('\r\n') + '\r\n';
  return new Blob([body], { type: 'text/csv;charset=utf-8' });
}

function csvEscape(value: string): string {
  let v = value ?? '';
  if (/^[=+\-@]/.test(v)) {
    v = "'" + v;
  }
  return '"' + v.replace(/"/g, '""') + '"';
}

export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.style.display = 'none';
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  URL.revokeObjectURL(url);
  document.body.removeChild(a);
}