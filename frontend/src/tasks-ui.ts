// ============================================================
// TASKS-UI.TS — Notes-style task view controller
// ============================================================

import { getAuthState, clearAuth } from './auth';
import { openModal } from './auth-ui';
import {
  fetchTasks,
  createTask,
  updateTask,
  deleteTask,
  exportTasks,
  downloadBlob,
  Task,
  ExportFormat,
  ReminderFields,
  UnauthorizedError,
} from './tasks-service';

// ─── Module state ─────────────────────────────────────────

let initialized = false;
let draftText = '';
let draftDate = '';
const editingDrafts = new Map<string, { text: string; date: string; reminder: ReminderFields }>();
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
  const [y, m, d] = iso.split('-');
  if (!y || !m || !d) return iso;
  const monthNames = [
    'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
  ];
  const mi = parseInt(m, 10) - 1;
  return `${monthNames[mi] ?? m} ${parseInt(d, 10)}, ${y}`;
}

// ─── Reminder helpers ────────────────────────────────────

function describeReminder(
  enabled: boolean,
  startDate: string,
  time: string,
  timezone: string,
  dueDate: string,
): string {
  if (!enabled) return 'Disabled';
  const effectiveStart = startDate || dueDate;
  const effectiveTime = time || '08:00';
  const effectiveTz = timezone || 'Asia/Kolkata';
  const tzShort = shortTimezone(effectiveTz);
  const datePart = effectiveStart ? `Starts ${formatDueDate(effectiveStart)}` : 'Starts on due date';
  return `${datePart}, daily at ${effectiveTime} (${tzShort})`;
}

function shortTimezone(iana: string): string {
  switch (iana) {
    case 'Asia/Kolkata': return 'IST';
    case 'Asia/Dubai': return 'GST';
    case 'Asia/Singapore': return 'SGT';
    case 'Asia/Tokyo': return 'JST';
    case 'Europe/London': return 'UK';
    case 'Europe/Paris': return 'CET';
    case 'America/New_York': return 'EST';
    case 'America/Chicago': return 'CST';
    case 'America/Los_Angeles': return 'PST';
    case 'Australia/Sydney': return 'AEST';
    case 'UTC': return 'UTC';
    default: return iana.split('/').pop() ?? iana;
  }
}

function readNewTaskReminder(): ReminderFields {
  const enabled = $<HTMLInputElement>('tasks-new-reminder-enabled')?.checked ?? true;
  const startDate = $<HTMLInputElement>('tasks-new-reminder-start-date')?.value ?? '';
  const time = $<HTMLInputElement>('tasks-new-reminder-time')?.value ?? '';
  const timezone = $<HTMLSelectElement>('tasks-new-reminder-timezone')?.value ?? '';

  const reminder: ReminderFields = { reminder_enabled: enabled };
  if (startDate) reminder.reminder_start_date = startDate;
  if (time) reminder.reminder_time = time;
  if (timezone) reminder.reminder_timezone = timezone;
  return reminder;
}

function resetNewTaskReminderInputs(): void {
  const enabled = $<HTMLInputElement>('tasks-new-reminder-enabled');
  const startDate = $<HTMLInputElement>('tasks-new-reminder-start-date');
  const time = $<HTMLInputElement>('tasks-new-reminder-time');
  const timezone = $<HTMLSelectElement>('tasks-new-reminder-timezone');

  if (enabled) enabled.checked = true;
  if (startDate) startDate.value = '';
  if (time) time.value = '';
  if (timezone) timezone.value = '';

  updateNewTaskReminderSummary();
}

function updateNewTaskReminderSummary(): void {
  const enabled = $<HTMLInputElement>('tasks-new-reminder-enabled')?.checked ?? true;
  const startDate = $<HTMLInputElement>('tasks-new-reminder-start-date')?.value ?? '';
  const time = $<HTMLInputElement>('tasks-new-reminder-time')?.value ?? '';
  const timezone = $<HTMLSelectElement>('tasks-new-reminder-timezone')?.value ?? '';
  const dueDate = $<HTMLInputElement>('tasks-new-date')?.value ?? '';

  const summary = $<HTMLSpanElement>('tasks-new-reminder-summary');
  if (!summary) return;

  if (!enabled) {
    summary.textContent = '(disabled for this task)';
    return;
  }

  if (!startDate && !time && !timezone) {
    summary.textContent = '(default: starts on due date, daily at 08:00)';
    return;
  }

  summary.textContent = `(${describeReminder(true, startDate, time, timezone, dueDate)})`;
}

function wireNewTaskReminderPanel(): void {
  const toggle = $<HTMLButtonElement>('tasks-new-reminder-toggle');
  const panel = $<HTMLDivElement>('tasks-new-reminder-panel');
  const chevron = $<HTMLElement>('tasks-new-reminder-chevron');

  if (!toggle || !panel) return;
  if ((toggle as any)._wired) return;
  (toggle as any)._wired = true;

  toggle.addEventListener('click', () => {
    const isHidden = panel.classList.contains('hidden');
    if (isHidden) {
      panel.classList.remove('hidden');
      chevron?.classList.add('rotate-180');
    } else {
      panel.classList.add('hidden');
      chevron?.classList.remove('rotate-180');
    }
  });

  ['tasks-new-reminder-enabled', 'tasks-new-reminder-start-date',
   'tasks-new-reminder-time', 'tasks-new-reminder-timezone'].forEach((id) => {
    const el = $<HTMLElement>(id);
    if (!el) return;
    el.addEventListener('change', updateNewTaskReminderSummary);
    el.addEventListener('input', updateNewTaskReminderSummary);
  });

  $<HTMLInputElement>('tasks-new-date')?.addEventListener('change', updateNewTaskReminderSummary);

  $<HTMLButtonElement>('tasks-new-reminder-reset')?.addEventListener('click', () => {
    resetNewTaskReminderInputs();
  });
}

// ─── Auth Error Handler ───────────────────────────────────
// Gracefully handles a 401 Unauthorized from the backend by
// clearing the invalid token and prompting the user to sign in again.
function handleAuthError(err: any): void {
  if (err instanceof UnauthorizedError || (err.message && err.message.includes('401'))) {
    clearAuth();
    openModal('login');
    toast('Session expired. Please sign in again.', true);
  }
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
    isError ? 'bg-red-600 text-white' : 'bg-gray-900 text-white',
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

export function initTasks(): void {
  if (initialized) return;

  const root = document.getElementById('tasks-view');
  if (!root) return;

  initialized = true;
  if (!draftDate) draftDate = todayISO();

  wireNewTaskForm();
  wireNewTaskReminderPanel();
  wireTasksContainer();
  wireExportButton();

  const refreshBtn = document.getElementById('tasks-refresh-btn') as HTMLButtonElement | null;
  if (refreshBtn && !(refreshBtn as any)._wired) {
    (refreshBtn as any)._wired = true;
    refreshBtn.addEventListener('click', () => {
      void loadSavedTasks();
    });
  }

  void loadSavedTasks();
}

export function renderTasksCategory(): void {
  initTasks();
  void loadSavedTasks();
}

// ─── New-task form ────────────────────────────────────────

function wireNewTaskForm(): void {
  const textarea = $<HTMLTextAreaElement>('tasks-new-text');
  const dateInput = $<HTMLInputElement>('tasks-new-date');
  const addBtn = $<HTMLButtonElement>('tasks-new-add');
  const emailBtn = $<HTMLButtonElement>('tasks-new-email');

  if (!textarea || !dateInput || !addBtn || !emailBtn) return;
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

  addBtn.addEventListener('click', () => { void handleCreate(false); });
  emailBtn.addEventListener('click', () => { void handleCreate(true); });
}

async function handleCreate(sendEmail: boolean): Promise<void> {
  const textarea = $<HTMLTextAreaElement>('tasks-new-text');
  const dateInput = $<HTMLInputElement>('tasks-new-date');
  const addBtn = $<HTMLButtonElement>('tasks-new-add');
  const emailBtn = $<HTMLButtonElement>('tasks-new-email');

  if (!textarea || !dateInput || !addBtn || !emailBtn) return;

  const text = textarea.value.trim();
  const date = dateInput.value || todayISO();

  if (!text) {
    toast('Please write your task first.', true);
    textarea.focus();
    return;
  }

  const reminder = readNewTaskReminder();

  addBtn.disabled = true;
  emailBtn.disabled = true;

  const originalAddLabel = addBtn.innerHTML;
  const originalEmailLabel = emailBtn.innerHTML;

  if (sendEmail) emailBtn.innerHTML = 'Sending…';
  else addBtn.innerHTML = 'Saving…';

  try {
    const { token } = getAuthState();
    const res = await createTask(text, date, token || undefined, sendEmail, reminder);

    draftText = '';
    draftDate = todayISO();
    textarea.value = '';
    dateInput.value = draftDate;
    resetNewTaskReminderInputs();

    if (!sendEmail) toast('Task saved ✅');
    else if (res.email_sent) toast('Task saved & email sent ✅');
    else toast('Task saved (email could not be sent)', true);

    await loadSavedTasks();
  } catch (err) {
    if (err instanceof UnauthorizedError) {
      handleAuthError(err);
    } else {
      toast(`Save failed: ${(err as Error).message}`, true);
    }
  } finally {
    addBtn.disabled = false;
    emailBtn.disabled = false;
    addBtn.innerHTML = originalAddLabel;
    emailBtn.innerHTML = originalEmailLabel;
  }
}

// ─── Saved-task list ──────────────────────────────────────

function wireTasksContainer(): void {
  const container = $<HTMLDivElement>('tasks-saved-list');
  if (!container) return;
  if ((container as any)._wired) return;
  (container as any)._wired = true;

  container.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;

    const editBtn = target.closest<HTMLButtonElement>('[data-task-edit]');
    if (editBtn) { startEditing(editBtn.dataset.taskEdit!); return; }

    const cancelBtn = target.closest<HTMLButtonElement>('[data-task-cancel]');
    if (cancelBtn) { editingDrafts.delete(cancelBtn.dataset.taskCancel!); renderSavedTasks(); return; }

    const saveBtn = target.closest<HTMLButtonElement>('[data-task-save]');
    if (saveBtn) { void commitEdit(saveBtn.dataset.taskSave!, 'silent'); return; }

    const emailBtn = target.closest<HTMLButtonElement>('[data-task-email]');
    if (emailBtn) { void commitEdit(emailBtn.dataset.taskEmail!, 'email'); return; }

    const deleteBtn = target.closest<HTMLButtonElement>('[data-task-delete]');
    if (deleteBtn) { void handleDelete(deleteBtn.dataset.taskDelete!); return; }
  });

  container.addEventListener('input', (e) => {
    const target = e.target as HTMLElement;
    if (target instanceof HTMLTextAreaElement && target.dataset.taskEditText) {
      const draft = editingDrafts.get(target.dataset.taskEditText);
      if (draft) draft.text = target.value;
    } else if (target instanceof HTMLInputElement && target.dataset.taskEditDate) {
      const draft = editingDrafts.get(target.dataset.taskEditDate);
      if (draft) draft.date = target.value;
    } else if (target instanceof HTMLInputElement && target.dataset.taskEditReminderEnabled) {
      const draft = editingDrafts.get(target.dataset.taskEditReminderEnabled);
      if (draft) draft.reminder.reminder_enabled = target.checked;
    } else if (target instanceof HTMLInputElement && target.dataset.taskEditReminderStart) {
      const draft = editingDrafts.get(target.dataset.taskEditReminderStart);
      if (draft) draft.reminder.reminder_start_date = target.value;
    } else if (target instanceof HTMLInputElement && target.dataset.taskEditReminderTime) {
      const draft = editingDrafts.get(target.dataset.taskEditReminderTime);
      if (draft) draft.reminder.reminder_time = target.value;
    } else if (target instanceof HTMLSelectElement && target.dataset.taskEditReminderTimezone) {
      const draft = editingDrafts.get(target.dataset.taskEditReminderTimezone);
      if (draft) draft.reminder.reminder_timezone = target.value;
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
    updateExportButtonState();
  } catch (err) {
    loadingEl?.classList.add('hidden');
    listEl.innerHTML = '';
    if (emptyEl) emptyEl.classList.remove('hidden');
    
    if (err instanceof UnauthorizedError) {
      handleAuthError(err);
    } else {
      console.error('[tasks] failed to load:', err);
    }
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
    updateExportButtonState();
    return;
  }
  emptyEl?.classList.add('hidden');

  const countEl = document.getElementById('tasks-count-label');
  if (countEl) countEl.textContent = `${allTasks.length} task${allTasks.length === 1 ? '' : 's'}`;

  listEl.innerHTML = allTasks.map((task) => {
    return editingDrafts.has(task.id) ? renderEditCard(task) : renderViewCard(task);
  }).join('');

  updateExportButtonState();
}

function renderViewCard(task: Task): string {
  const reminderText = describeReminder(
    task.reminder_enabled, task.reminder_start_date, task.reminder_time, task.reminder_timezone, task.due_date,
  );
  const reminderIconClass = task.reminder_enabled ? 'text-indigo-600' : 'text-gray-400';

  return `
    <div class="task-card task-card--saved bg-white border border-gray-200 rounded-2xl p-5 shadow-sm">
      <div class="flex items-start gap-3">
        <div class="w-9 h-9 shrink-0 rounded-full bg-indigo-100 flex items-center justify-center">
          <svg class="w-4 h-4 text-indigo-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/></svg>
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-semibold text-gray-900 whitespace-pre-wrap break-words">${escapeHtml(task.text)}</p>
          <p class="mt-2 text-xs text-gray-500 flex items-center gap-1.5">
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
            Due ${escapeHtml(formatDueDate(task.due_date))}
          </p>
          <p class="mt-1 text-xs ${reminderIconClass} flex items-center gap-1.5">
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9"/></svg>
            ${escapeHtml(reminderText)}
          </p>
        </div>
        <div class="flex items-center gap-1.5 shrink-0">
          <button type="button" data-task-edit="${escapeHtml(task.id)}" class="px-2.5 py-1.5 rounded-lg text-xs font-semibold text-indigo-700 bg-indigo-50 hover:bg-indigo-100 transition-colors">Edit</button>
          <button type="button" data-task-delete="${escapeHtml(task.id)}" class="p-1.5 rounded-lg text-red-600 hover:bg-red-50 transition-colors" aria-label="Delete task"><svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6M1 7h22M8 7V5a2 2 0 012-2h4a2 2 0 012 2v2"/></svg></button>
        </div>
      </div>
    </div>
  `;
}

function renderEditCard(task: Task): string {
  const draft = editingDrafts.get(task.id) ?? {
    text: task.text, date: task.due_date,
    reminder: {
      reminder_enabled: task.reminder_enabled,
      reminder_start_date: task.reminder_start_date,
      reminder_time: task.reminder_time,
      reminder_timezone: task.reminder_timezone,
    },
  };
  const r = draft.reminder;

  return `
    <div class="task-card task-card--editing bg-amber-50 border border-amber-200 rounded-2xl p-4 shadow-sm">
      <p class="text-[11px] font-semibold text-amber-700 uppercase tracking-wide mb-2">Editing task</p>
      <textarea data-task-edit-text="${escapeHtml(task.id)}" rows="3" class="w-full bg-white border border-gray-300 rounded-xl py-2 px-3 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500 resize-y" placeholder="Write your task…">${escapeHtml(draft.text)}</textarea>
      <details class="mt-3 bg-white border border-amber-100 rounded-xl px-3 py-2">
        <summary class="text-xs font-semibold text-amber-800 cursor-pointer select-none">⏰ Reminder Settings</summary>
        <div class="mt-3 space-y-2">
          <label class="flex items-center gap-2 text-xs text-gray-800"><input type="checkbox" data-task-edit-reminder-enabled="${escapeHtml(task.id)}" ${r.reminder_enabled ? 'checked' : ''} class="w-4 h-4 text-amber-600 bg-white border-gray-300 rounded focus:ring-amber-500" /> Enable daily reminder email</label>
          <div class="grid grid-cols-2 gap-2">
            <div><label class="block text-[11px] font-semibold text-gray-600 mb-1">Start date</label><input type="date" data-task-edit-reminder-start="${escapeHtml(task.id)}" value="${escapeHtml(r.reminder_start_date || '')}" class="w-full bg-white border border-gray-300 rounded-lg py-1.5 px-2 text-xs focus:outline-none focus:ring-2 focus:ring-amber-500" /></div>
            <div><label class="block text-[11px] font-semibold text-gray-600 mb-1">Time (24h)</label><input type="time" data-task-edit-reminder-time="${escapeHtml(task.id)}" value="${escapeHtml(r.reminder_time || '')}" class="w-full bg-white border border-gray-300 rounded-lg py-1.5 px-2 text-xs focus:outline-none focus:ring-2 focus:ring-amber-500" /></div>
          </div>
          <div>
            <label class="block text-[11px] font-semibold text-gray-600 mb-1">Timezone</label>
            <select data-task-edit-reminder-timezone="${escapeHtml(task.id)}" class="w-full bg-white border border-gray-300 rounded-lg py-1.5 px-2 text-xs focus:outline-none focus:ring-2 focus:ring-amber-500">
              <option value="" ${!r.reminder_timezone ? 'selected' : ''}>Default (Asia/Kolkata)</option>
              <option value="Asia/Kolkata" ${r.reminder_timezone === 'Asia/Kolkata' ? 'selected' : ''}>Asia/Kolkata (IST, UTC+5:30)</option>
              <option value="Asia/Dubai" ${r.reminder_timezone === 'Asia/Dubai' ? 'selected' : ''}>Asia/Dubai (GST, UTC+4)</option>
              <option value="Asia/Singapore" ${r.reminder_timezone === 'Asia/Singapore' ? 'selected' : ''}>Asia/Singapore (SGT, UTC+8)</option>
              <option value="Asia/Tokyo" ${r.reminder_timezone === 'Asia/Tokyo' ? 'selected' : ''}>Asia/Tokyo (JST, UTC+9)</option>
              <option value="Europe/London" ${r.reminder_timezone === 'Europe/London' ? 'selected' : ''}>Europe/London (GMT/BST)</option>
              <option value="Europe/Paris" ${r.reminder_timezone === 'Europe/Paris' ? 'selected' : ''}>Europe/Paris (CET/CEST)</option>
              <option value="America/New_York" ${r.reminder_timezone === 'America/New_York' ? 'selected' : ''}>America/New_York (EST/EDT)</option>
              <option value="America/Chicago" ${r.reminder_timezone === 'America/Chicago' ? 'selected' : ''}>America/Chicago (CST/CDT)</option>
              <option value="America/Los_Angeles" ${r.reminder_timezone === 'America/Los_Angeles' ? 'selected' : ''}>America/Los_Angeles (PST/PDT)</option>
              <option value="Australia/Sydney" ${r.reminder_timezone === 'Australia/Sydney' ? 'selected' : ''}>Australia/Sydney (AEST/AEDT)</option>
              <option value="UTC" ${r.reminder_timezone === 'UTC' ? 'selected' : ''}>UTC</option>
            </select>
          </div>
        </div>
      </details>
      <div class="mt-3 flex flex-col sm:flex-row gap-3 items-stretch sm:items-center">
        <label class="flex items-center gap-2 text-xs font-semibold text-gray-700"><span>Due date</span><input type="date" data-task-edit-date="${escapeHtml(task.id)}" value="${escapeHtml(draft.date)}" class="bg-white border border-gray-300 rounded-lg py-1.5 px-2 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500" /></label>
        <div class="flex flex-col sm:flex-row gap-2 sm:ml-auto w-full sm:w-auto">
          <button type="button" data-task-cancel="${escapeHtml(task.id)}" class="w-full sm:w-auto px-3 py-1.5 rounded-lg text-xs font-semibold text-gray-700 bg-white border border-gray-300 hover:bg-gray-50 transition-colors">Cancel</button>
          <button type="button" data-task-save="${escapeHtml(task.id)}" class="w-full sm:w-auto px-3 py-1.5 rounded-lg text-xs font-semibold text-amber-700 bg-white border border-amber-300 hover:bg-amber-50 transition-colors">Update</button>
          <button type="button" data-task-email="${escapeHtml(task.id)}" class="w-full sm:w-auto px-3 py-1.5 rounded-lg text-xs font-bold text-white bg-amber-600 hover:bg-amber-700 transition-colors">Email</button>
        </div>
      </div>
    </div>
  `;
}

function startEditing(id: string): void {
  const task = allTasks.find((t) => t.id === id);
  if (!task) return;
  editingDrafts.set(id, {
    text: task.text, date: task.due_date,
    reminder: {
      reminder_enabled: task.reminder_enabled,
      reminder_start_date: task.reminder_start_date,
      reminder_time: task.reminder_time,
      reminder_timezone: task.reminder_timezone,
    },
  });
  renderSavedTasks();
}

async function commitEdit(id: string, mode: 'silent' | 'email'): Promise<void> {
  const draft = editingDrafts.get(id);
  if (!draft) return;

  const text = draft.text.trim();
  const date = draft.date || todayISO();

  if (!text) { toast('Task text cannot be empty.', true); return; }

  const sendEmail = mode === 'email';

  try {
    const { token } = getAuthState();
    const res = await updateTask(id, text, date, token || undefined, sendEmail, draft.reminder);
    editingDrafts.delete(id);

    if (!sendEmail) toast('Task updated ✅');
    else if (res.email_sent) toast('Task updated & email sent ✅');
    else toast('Task updated (email could not be sent)', true);

    await loadSavedTasks();
  } catch (err) {
    if (err instanceof UnauthorizedError) {
      handleAuthError(err);
    } else {
      toast(`Update failed: ${(err as Error).message}`, true);
    }
  }
}

async function handleDelete(id: string): Promise<void> {
  if (!confirm('Delete this task?')) return;
  try {
    const { token } = getAuthState();
    const res = await deleteTask(id, token || undefined, true);

    if (res.email_sent === true) toast('Task deleted & email sent ✅');
    else if (res.email_sent === false) toast('Task deleted (email could not be sent)', true);
    else toast('Task deleted');

    await loadSavedTasks();
  } catch (err) {
    if (err instanceof UnauthorizedError) {
      handleAuthError(err);
    } else {
      toast(`Delete failed: ${(err as Error).message}`, true);
    }
  }
}

// ─── Export feature ───────────────────────────────────────

function wireExportButton(): void {
  const btn = $<HTMLButtonElement>('tasks-export-btn');
  const menu = $<HTMLDivElement>('tasks-export-menu');
  const csvBtn = $<HTMLButtonElement>('tasks-export-csv');
  const jsonBtn = $<HTMLButtonElement>('tasks-export-json');

  if (!btn || !menu || !csvBtn || !jsonBtn) return;
  if ((btn as any)._wired) return;
  (btn as any)._wired = true;

  btn.addEventListener('click', (e) => {
    e.stopPropagation();
    menu.classList.toggle('hidden');
  });

  document.addEventListener('click', (e) => {
    if (!menu.contains(e.target as Node) && e.target !== btn) {
      menu.classList.add('hidden');
    }
  });

  csvBtn.addEventListener('click', () => { menu.classList.add('hidden'); void handleExport('csv'); });
  jsonBtn.addEventListener('click', () => { menu.classList.add('hidden'); void handleExport('json'); });

  updateExportButtonState();
}

function updateExportButtonState(): void {
  const btn = $<HTMLButtonElement>('tasks-export-btn');
  if (!btn) return;
  const hasTasks = allTasks.length > 0;
  btn.disabled = !hasTasks;
  btn.classList.toggle('opacity-50', !hasTasks);
  btn.classList.toggle('cursor-not-allowed', !hasTasks);
}

function handleExport(format: ExportFormat): void {
  if (allTasks.length === 0) { toast('No tasks to export yet.', true); return; }

  try {
    const blob = exportTasks(allTasks, format);
    const stamp = todayISO();
    const filename = `my-tasks-${stamp}.${format}`;
    downloadBlob(blob, filename);
    toast(`Exported ${allTasks.length} task${allTasks.length === 1 ? '' : 's'} as ${format.toUpperCase()} ✅`);
  } catch (err) {
    toast(`Export failed: ${(err as Error).message}`, true);
  }
}

// ─── Boot-time draft init ─────────────────────────────────

if (!draftDate) draftDate = todayISO();