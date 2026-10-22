// ============================================================
// TASKS-UI.TS — Notes-style task view controller
//
// Reads from and writes to the #tasks-view block in index.html.
// Idempotent init: initTasks() can be called many times without
// re-wiring listeners.
//
// Responsibilities:
//   1. Render the "new task" card (textarea + date input + SEND)
//   2. Save a new task via tasks-service.createTask()
//   3. List saved tasks; toggle each card into edit mode on demand
//   4. Save an edit via tasks-service.updateTask()
//   5. Delete a task via tasks-service.deleteTask()
//
// The HTML skeleton (view container + form + list) lives in
// index.html. This file only wires behavior.
// ============================================================

import { getAuthState } from './auth';
import {
  fetchTasks,
  createTask,
  updateTask,
  deleteTask,
  Task,
} from './tasks-service';

// ─── Module state ─────────────────────────────────────────

let initialized = false;

// Draft state for the "new task" card
let draftText = '';
let draftDate = '';   // YYYY-MM-DD

// Per-card editing state: taskId → { text, date }
const editingDrafts = new Map<string, { text: string; date: string }>();

let allTasks: Task[] = [];

// ─── DOM helpers ──────────────────────────────────────────

function $<T extends HTMLElement>(id: string): T | null {
  return document.getElementById(id) as T | null;
}

function todayISO(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function formatDueDate(iso: string): string {
  if (!iso) return '';
  // iso is YYYY-MM-DD
  const [y, m, d] = iso.split('-');
  if (!y || !m || !d) return iso;
  const monthNames = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  const mi = parseInt(m, 10) - 1;
  return `${monthNames[mi] ?? m} ${parseInt(d, 10)}, ${y}`;
}

function toast(message: string, isError = false): void {
  const existing = document.getElementById('tasks-toast');
  if (existing) existing.remove();

  const el = document.createElement('div');
  el.id = 'tasks-toast';
  el.className = [
    'fixed bottom-6 left-1/2 -translate-x-1/2 z-[300]',
    'text-sm font-medium px-5 py-3 rounded-full shadow-2xl',
    'flex items-center gap-2',
    'opacity-0 translate-y-4 transition-all duration-300',
    isError
      ? 'bg-red-600 text-white'
      : 'bg-gray-900 text-white',
  ].join(' ');
  el.innerHTML = isError
    ? `<span>${escapeHtml(message)}</span>`
    : `<svg class="w-4 h-4 text-green-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg><span>${escapeHtml(message)}</span>`;

  document.body.appendChild(el);
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      el.classList.remove('opacity-0', 'translate-y-4');
    });
  });
  setTimeout(() => {
    el.classList.add('opacity-0', 'translate-y-4');
    setTimeout(() => el.remove(), 300);
  }, 3200);
}

// ─── Public API ───────────────────────────────────────────

/**
 * Idempotent initializer. Safe to call many times.
 */
export function initTasks(): void {
  if (initialized) return;

  const root = document.getElementById('tasks-view');
  if (!root) {
    // View not present in this build — silently skip.
    return;
  }

  initialized = true;

  // Seed the draft date to today.
  if (!draftDate) draftDate = todayISO();

  wireNewTaskForm();
  wireTasksContainer();

  const refreshBtn = document.getElementById('tasks-refresh-btn') as HTMLButtonElement | null;
  if (refreshBtn && !(refreshBtn as any)._wired) {
    (refreshBtn as any)._wired = true;
    refreshBtn.addEventListener('click', () => {
      void loadSavedTasks();
    });
  }

  // Initial load of saved tasks.
  void loadSavedTasks();
}

/**
 * Called from main.ts when the "Tasks" sidebar entry is clicked.
 * Refreshes the list, but is safe to call even if init has not run yet.
 */
export function renderTasksCategory(): void {
  initTasks();
  void loadSavedTasks();
}

// ─── New-task form ────────────────────────────────────────

function wireNewTaskForm(): void {
  const textarea = $<HTMLTextAreaElement>('tasks-new-text');
  const dateInput = $<HTMLInputElement>('tasks-new-date');
  const sendBtn = $<HTMLButtonElement>('tasks-new-send');

  if (!textarea || !dateInput || !sendBtn) return;
  if ((textarea as any)._wired) return;
  (textarea as any)._wired = true;

  textarea.value = draftText;
  dateInput.value = draftDate;

  textarea.addEventListener('input', () => {
    draftText = textarea.value;
  });
  dateInput.addEventListener('change', () => {
    draftDate = dateInput.value || todayISO();
  });

  sendBtn.addEventListener('click', () => {
    void handleCreate();
  });
}

async function handleCreate(): Promise<void> {
  const textarea = $<HTMLTextAreaElement>('tasks-new-text');
  const dateInput = $<HTMLInputElement>('tasks-new-date');
  const sendBtn = $<HTMLButtonElement>('tasks-new-send');

  if (!textarea || !dateInput || !sendBtn) return;

  const text = textarea.value.trim();
  const date = dateInput.value || todayISO();

  if (!text) {
    toast('Please write your task first.', true);
    textarea.focus();
    return;
  }

  sendBtn.disabled = true;
  const originalLabel = sendBtn.textContent || 'SEND';
  sendBtn.textContent = 'Sending…';

  try {
    const { token } = getAuthState();
    // ⬅️ CHANGED: capture the response so we can read `email_sent`.
    const res = await createTask(text, date, token || undefined);

    // Reset the draft.
    draftText = '';
    draftDate = todayISO();
    textarea.value = '';
    dateInput.value = draftDate;

    // ⬅️ NEW: choose the toast wording based on whether the confirmation
    // email actually went out. The task is always saved either way; only
    // the email is best-effort.
    if (res.email_sent) {
      toast('Task saved & email sent ✅');
    } else {
      toast('Task saved (email could not be sent)', true);
    }

    await loadSavedTasks();
  } catch (err) {
    toast(`Save failed: ${(err as Error).message}`, true);
  } finally {
    sendBtn.disabled = false;
    sendBtn.textContent = originalLabel;
  }
}

// ─── Saved-task list ──────────────────────────────────────

function wireTasksContainer(): void {
  const container = $<HTMLDivElement>('tasks-saved-list');
  if (!container) return;
  if ((container as any)._wired) return;
  (container as any)._wired = true;

  // Event delegation — one listener for all card buttons.
  container.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;

    const editBtn = target.closest<HTMLButtonElement>('[data-task-edit]');
    if (editBtn) {
      const id = editBtn.dataset.taskEdit!;
      startEditing(id);
      return;
    }

    const cancelBtn = target.closest<HTMLButtonElement>('[data-task-cancel]');
    if (cancelBtn) {
      const id = cancelBtn.dataset.taskCancel!;
      editingDrafts.delete(id);
      renderSavedTasks();
      return;
    }

    const saveBtn = target.closest<HTMLButtonElement>('[data-task-save]');
    if (saveBtn) {
      const id = saveBtn.dataset.taskSave!;
      void commitEdit(id);
      return;
    }

    const deleteBtn = target.closest<HTMLButtonElement>('[data-task-delete]');
    if (deleteBtn) {
      const id = deleteBtn.dataset.taskDelete!;
      void handleDelete(id);
      return;
    }
  });

  // Delegated input listeners for edit-mode textareas / date inputs.
  container.addEventListener('input', (e) => {
    const target = e.target as HTMLElement;
    if (target instanceof HTMLTextAreaElement && target.dataset.taskEditText) {
      const id = target.dataset.taskEditText;
      const draft = editingDrafts.get(id);
      if (draft) draft.text = target.value;
    }
  });
  container.addEventListener('change', (e) => {
    const target = e.target as HTMLElement;
    if (target instanceof HTMLInputElement && target.dataset.taskEditDate) {
      const id = target.dataset.taskEditDate;
      const draft = editingDrafts.get(id);
      if (draft) draft.date = target.value;
    }
  });
}

async function loadSavedTasks(): Promise<void> {
  const listEl = $<HTMLDivElement>('tasks-saved-list');
  const emptyEl = $<HTMLDivElement>('tasks-empty-state');
  const loadingEl = $<HTMLDivElement>('tasks-loading');
  if (!listEl) return;

  loadingEl?.classList.remove('hidden');
  emptyEl?.classList.add('hidden');

  try {
    const { token } = getAuthState();
    const res = await fetchTasks(token || undefined);
    allTasks = res.tasks || [];
    renderSavedTasks();
    loadingEl?.classList.add('hidden');
  } catch (err) {
    loadingEl?.classList.add('hidden');
    listEl.innerHTML = '';
    if (emptyEl) emptyEl.classList.remove('hidden');
    // Surface the error but don't spam the toast on every refresh.
    console.error('[tasks] failed to load:', err);
  }
}

function renderSavedTasks(): void {
  const listEl = $<HTMLDivElement>('tasks-saved-list');
  const emptyEl = $<HTMLDivElement>('tasks-empty-state');
  if (!listEl) return;

  if (allTasks.length === 0) {
    listEl.innerHTML = '';
    emptyEl?.classList.remove('hidden');
    const countEl = document.getElementById('tasks-count-label');
    if (countEl) countEl.textContent = '0 tasks';
    return;
  }
  emptyEl?.classList.add('hidden');

  const countEl = document.getElementById('tasks-count-label');
  if (countEl) {
    countEl.textContent = `${allTasks.length} task${allTasks.length === 1 ? '' : 's'}`;
  }

  listEl.innerHTML = allTasks
    .map((task) => {
      const isEditing = editingDrafts.has(task.id);
      return isEditing ? renderEditCard(task) : renderViewCard(task);
    })
    .join('');
}

function renderViewCard(task: Task): string {
  return `
    <div class="task-card task-card--saved bg-white border border-gray-200 rounded-2xl p-5 shadow-sm">
      <div class="flex items-start gap-3">
        <div class="w-9 h-9 shrink-0 rounded-full bg-indigo-100 flex items-center justify-center">
          <svg class="w-4 h-4 text-indigo-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/>
          </svg>
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-semibold text-gray-900 whitespace-pre-wrap break-words">${escapeHtml(task.text)}</p>
          <p class="mt-2 text-xs text-gray-500 flex items-center gap-1.5">
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/>
            </svg>
            Due ${escapeHtml(formatDueDate(task.due_date))}
          </p>
        </div>
        <div class="flex items-center gap-1.5 shrink-0">
          <button type="button" data-task-edit="${escapeHtml(task.id)}"
            class="px-2.5 py-1.5 rounded-lg text-xs font-semibold text-indigo-700 bg-indigo-50 hover:bg-indigo-100 transition-colors">
            Edit
          </button>
          <button type="button" data-task-delete="${escapeHtml(task.id)}"
            class="p-1.5 rounded-lg text-red-600 hover:bg-red-50 transition-colors"
            aria-label="Delete task">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M8 7V5a2 2 0 012-2h4a2 2 0 012 2v2"/>
            </svg>
          </button>
        </div>
      </div>
    </div>
  `;
}

function renderEditCard(task: Task): string {
  const draft = editingDrafts.get(task.id) ?? { text: task.text, date: task.due_date };
  return `
    <div class="task-card task-card--editing bg-amber-50 border border-amber-200 rounded-2xl p-4 shadow-sm">
      <p class="text-[11px] font-semibold text-amber-700 uppercase tracking-wide mb-2">Editing task</p>
      <textarea
        data-task-edit-text="${escapeHtml(task.id)}"
        rows="3"
        class="w-full bg-white border border-gray-300 rounded-xl py-2 px-3 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500 resize-y"
        placeholder="Write your task…">${escapeHtml(draft.text)}</textarea>
      <div class="mt-3 flex flex-col sm:flex-row gap-3 items-stretch sm:items-center">
        <label class="flex items-center gap-2 text-xs font-semibold text-gray-700">
          <span>Due date</span>
          <input
            type="date"
            data-task-edit-date="${escapeHtml(task.id)}"
            value="${escapeHtml(draft.date)}"
            class="bg-white border border-gray-300 rounded-lg py-1.5 px-2 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500"
          />
        </label>
        <div class="flex gap-2 ml-auto">
          <button type="button" data-task-cancel="${escapeHtml(task.id)}"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold text-gray-700 bg-white border border-gray-300 hover:bg-gray-50 transition-colors">
            Cancel
          </button>
          <button type="button" data-task-save="${escapeHtml(task.id)}"
            class="task-send-btn px-4 py-1.5 rounded-lg text-xs font-bold text-white bg-amber-600 hover:bg-amber-700 transition-colors">
            SEND
          </button>
        </div>
      </div>
    </div>
  `;
}

function startEditing(id: string): void {
  const task = allTasks.find((t) => t.id === id);
  if (!task) return;
  editingDrafts.set(id, { text: task.text, date: task.due_date });
  renderSavedTasks();
}

async function commitEdit(id: string): Promise<void> {
  const draft = editingDrafts.get(id);
  if (!draft) return;

  const text = draft.text.trim();
  const date = draft.date || todayISO();

  if (!text) {
    toast('Task text cannot be empty.', true);
    return;
  }

  try {
    const { token } = getAuthState();
    await updateTask(id, text, date, token || undefined);
    editingDrafts.delete(id);
    toast('Task updated ✅');
    await loadSavedTasks();
  } catch (err) {
    toast(`Update failed: ${(err as Error).message}`, true);
  }
}

async function handleDelete(id: string): Promise<void> {
  if (!confirm('Delete this task?')) return;
  try {
    const { token } = getAuthState();
    await deleteTask(id, token || undefined);
    toast('Task deleted');
    await loadSavedTasks();
  } catch (err) {
    toast(`Delete failed: ${(err as Error).message}`, true);
  }
}

// Keep `draftDate` initialised on first import, in case main.ts
// imports this module before initTasks() runs.
if (!draftDate) draftDate = todayISO();