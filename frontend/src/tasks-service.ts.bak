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

/** GET /tasks — every task owned by the caller, newest first. */
export async function fetchTasks(token?: string): Promise<TasksListResponse> {
  return apiGet<TasksListResponse>('/tasks', token);
}

/** POST /tasks — creates a new task owned by the caller. */
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

/** PUT /tasks/{id} — updates an existing task owned by the caller. */
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

/** DELETE /tasks/{id} — removes a task owned by the caller. */
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

// ─── Export helpers (client-side only) ────────────────────

export type ExportFormat = 'csv' | 'json';

/**
 * Serialise the given tasks to a downloadable Blob in the requested
 * format. The caller (tasks-ui.ts) passes `allTasks`, which is
 * already user-scoped by the backend — so the export is automatically
 * user-scoped for free.
 *
 * CSV rules:
 *  - header row: id,text,due_date,created_at,updated_at
 *  - cells are RFC 4180 quoted (double quotes escaped as "")
 *  - a leading ' = + - @ in any cell is prefixed with ' to prevent
 *    spreadsheet formula injection (OWASP recommendation)
 *  - line endings are \r\n (RFC 4180 compliance)
 *
 * JSON rules:
 *  - a plain array of the task objects as returned by the API
 *  - pretty-printed with 2-space indentation
 */
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

  // \r\n is what Excel and Google Sheets expect for CSV.
  const body = rows.join('\r\n') + '\r\n';
  return new Blob([body], { type: 'text/csv;charset=utf-8' });
}

/**
 * RFC 4180-compliant CSV escaping + spreadsheet formula-injection
 * mitigation. A cell is wrapped in double quotes and any internal
 * double quote is doubled. Cells whose first non-whitespace
 * character is one of = + - @ are prefixed with a single quote so
 * that Excel/Sheets treat them as text, not formulas.
 */
function csvEscape(value: string): string {
  let v = value ?? '';

  // Formula-injection guard: only matters for the very first
  // character. Prefix with ' if it is one of the risky openers.
  if (/^[=+\-@]/.test(v)) {
    v = "'" + v;
  }

  // Always wrap in quotes for maximum compatibility.
  return '"' + v.replace(/"/g, '""') + '"';
}

/**
 * Trigger a browser download of `blob` with the given filename.
 * Kept here so callers don't need to duplicate the anchor dance.
 */
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