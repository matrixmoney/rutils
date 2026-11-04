// ============================================================
// FORMATTERS.TS — File Repair / Formatter UI controller
//
// Single responsibility: drive the "Formatter" sidebar category
// (#formatters-view). That view has:
//
//   1. A format-tile grid (JSON / CSV / XML / …) for picking the
//      target format.
//
//   2. A repair form that accepts EITHER pasted text OR an uploaded
//      file, sends it to /repair, and lets the user copy the
//      repaired text OR download it as a file.
//
// Exports:
//   initFormatters()            — resolve DOM refs, wire listeners (idempotent)
//   renderFormattersCategory()  — called from main.ts when the user
//                                 clicks "Formatter" in the sidebar
// ============================================================

// ------------------------------------------------------------
// Supported formats (used by the format-tile grid)
// ------------------------------------------------------------
interface FormatterFormat {
  id: string;
  label: string;
  ext: string;
  icon: string;
  accept: string;
}

const FORMATTER_FORMATS: FormatterFormat[] = [
  { id: 'json', label: 'JSON', ext: '.json',  icon: '{}', accept: '.json' },
  { id: 'csv',  label: 'CSV',  ext: '.csv',   icon: '≣',  accept: '.csv' },
  { id: 'xml',  label: 'XML',  ext: '.xml',   icon: '<>', accept: '.xml' },
  { id: 'yaml', label: 'YAML', ext: '.yaml',  icon: 'Y:', accept: '.yaml,.yml' },
  { id: 'html', label: 'HTML', ext: '.html',  icon: '</>', accept: '.html,.htm' },
  { id: 'toml', label: 'TOML', ext: '.toml',  icon: 'T:', accept: '.toml' },
  { id: 'ini',  label: 'INI',  ext: '.ini',   icon: 'I:', accept: '.ini' },
  { id: 'md',   label: 'MD',   ext: '.md',    icon: 'M↓', accept: '.md,.markdown' },
];

// ------------------------------------------------------------
// Module state
// ------------------------------------------------------------
let initialized = false;
let activeFormat: string = '';

// Dual-input state
let inputMode: 'text' | 'file' = 'text';
let detectedFormatFromText: string = '';
let lastRepairedOutput: string = '';
let lastRepairedFormat: string = '';

// ------------------------------------------------------------
// Public API — idempotent init
// ------------------------------------------------------------
export function initFormatters(): void {
  if (initialized) return;

  const grid = document.getElementById('formatters-grid');
  if (!grid) {
    console.warn(
      '[formatters] #formatters-grid not found. ' +
      'Add the <div id="formatters-view"> block to index.html.',
    );
    return;
  }

  initialized = true;

  renderTiles(grid);
  wireInputTabs();
  wireTextInput();
  wireRepairForm();
  wireOutputActions();
}

// Called from main.ts every time the user clicks "Formatter" in the
// sidebar. Safe to call repeatedly.
export function renderFormattersCategory(): void {
  initFormatters();

  if (activeFormat) {
    const panel = document.getElementById('formatters-repair-panel');
    panel?.classList.remove('hidden');
  }
}

// ============================================================
// Format tiles
// ============================================================
function renderTiles(grid: HTMLElement): void {
  grid.innerHTML = '';

  FORMATTER_FORMATS.forEach(fmt => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.dataset.format = fmt.id;
    btn.className = [
      'formatters-tile',
      'flex', 'flex-col', 'items-center', 'justify-center', 'gap-1.5',
      'py-5', 'px-3', 'rounded-xl', 'border', 'border-gray-300',
      'bg-white', 'text-gray-700', 'font-medium', 'text-sm',
      'hover:bg-amber-50', 'hover:border-amber-400', 'hover:text-amber-700',
      'transition-colors', 'cursor-pointer',
    ].join(' ');

    btn.innerHTML = `
      <span class="text-2xl font-mono text-amber-600 leading-none">${escapeHtml(fmt.icon)}</span>
      <span class="mt-1">${escapeHtml(fmt.label)}</span>
      <span class="text-[11px] text-gray-400">${escapeHtml(fmt.ext)}</span>
    `;

    btn.addEventListener('click', () => selectFormat(fmt.id));

    grid.appendChild(btn);
  });
}

function selectFormat(formatId: string): void {
  activeFormat = formatId;
  const fmt = FORMATTER_FORMATS.find(f => f.id === formatId);

  // Highlight the active tile.
  document.querySelectorAll<HTMLElement>('.formatters-tile').forEach(tile => {
    const isActive = tile.dataset.format === formatId;
    tile.classList.toggle('bg-amber-100', isActive);
    tile.classList.toggle('border-amber-500', isActive);
    tile.classList.toggle('text-amber-800', isActive);
    tile.classList.toggle('ring-2', isActive);
    tile.classList.toggle('ring-amber-200', isActive);
  });

  // Reveal the repair panel.
  const panel = document.getElementById('formatters-repair-panel');
  panel?.classList.remove('hidden');

  // Update the header text.
  const label = document.getElementById('formatters-active-format');
  if (label && fmt) label.textContent = fmt.label;

  // Update the file input's accept attribute for the OS picker.
  const fileInput = document.getElementById('formatters-repair-file') as HTMLInputElement | null;
  if (fileInput && fmt) {
    fileInput.accept = fmt.accept;
    fileInput.value = '';
  }

  // Reset any prior result.
  document.getElementById('formatters-repair-report')?.classList.add('hidden');
  hideOutputSection();

  const status = document.getElementById('formatters-repair-status');
  if (status) {
    status.classList.add('hidden');
    status.textContent = '';
  }

  // Re-evaluate the detected badge now that a format was chosen.
  updateDetectedBadge();
}

// ============================================================
// Dual-input tab switcher (Paste Text / Upload File)
// ============================================================
function wireInputTabs(): void {
  const tabText = document.getElementById('formatters-repair-tab-text') as HTMLButtonElement | null;
  const tabFile = document.getElementById('formatters-repair-tab-file') as HTMLButtonElement | null;

  if (!tabText || !tabFile) return;
  if ((tabText as any)._wired) return;
  (tabText as any)._wired = true;

  tabText.addEventListener('click', () => switchInputMode('text'));
  tabFile.addEventListener('click', () => switchInputMode('file'));
}

function switchInputMode(mode: 'text' | 'file'): void {
  inputMode = mode;

  const tabText = document.getElementById('formatters-repair-tab-text') as HTMLButtonElement;
  const tabFile = document.getElementById('formatters-repair-tab-file') as HTMLButtonElement;
  const textPanel = document.getElementById('formatters-repair-text-panel') as HTMLDivElement;
  const filePanel = document.getElementById('formatters-repair-file-panel') as HTMLDivElement;

  const activeCls = ['bg-white', 'text-amber-700', 'shadow-sm'];
  const inactiveCls = ['text-gray-600', 'hover:text-amber-700'];

  [tabText, tabFile].forEach(btn => btn.classList.remove(...activeCls, ...inactiveCls));

  if (mode === 'text') {
    tabText.classList.add(...activeCls);
    tabFile.classList.add(...inactiveCls);
    textPanel.classList.remove('hidden');
    filePanel.classList.add('hidden');
  } else {
    tabFile.classList.add(...activeCls);
    tabText.classList.add(...inactiveCls);
    filePanel.classList.remove('hidden');
    textPanel.classList.add('hidden');
  }

  updateDetectedBadge();
}

// ============================================================
// Text input — live format sniffing + clear button
// ============================================================
function wireTextInput(): void {
  const textarea = document.getElementById('formatters-repair-text-input') as HTMLTextAreaElement | null;
  const clearBtn = document.getElementById('formatters-repair-text-clear') as HTMLButtonElement | null;

  if (textarea && !(textarea as any)._wired) {
    (textarea as any)._wired = true;
    textarea.addEventListener('input', () => {
      detectedFormatFromText = sniffTextFormat(textarea.value);
      updateDetectedBadge();
    });
  }

  if (clearBtn && !(clearBtn as any)._wired) {
    (clearBtn as any)._wired = true;
    clearBtn.addEventListener('click', () => {
      if (!textarea) return;
      textarea.value = '';
      detectedFormatFromText = '';
      updateDetectedBadge();
      textarea.focus();
    });
  }

  // Mirror file-input changes into the badge as well.
  const fileInput = document.getElementById('formatters-repair-file') as HTMLInputElement | null;
  if (fileInput && !(fileInput as any)._wiredChange) {
    (fileInput as any)._wiredChange = true;
    fileInput.addEventListener('change', () => updateDetectedBadge());
  }
}

// ------------------------------------------------------------
// Update the "Detected Format" badge to match the current mode.
// ------------------------------------------------------------
function updateDetectedBadge(): void {
  const inlineBadge = document.getElementById('formatters-repair-text-detected');
  const outputMeta = document.getElementById('formatters-repair-output-meta');

  // In text mode, show the sniffed format inline under the textarea.
  if (inputMode === 'text') {
    const textarea = document.getElementById('formatters-repair-text-input') as HTMLTextAreaElement | null;
    const text = textarea?.value ?? '';

    if (!text.trim()) {
      if (inlineBadge) inlineBadge.textContent = 'Waiting for input…';
      return;
    }

    if (detectedFormatFromText) {
      if (inlineBadge) inlineBadge.textContent = `Detected: ${detectedFormatFromText.toUpperCase()} ✓`;
    } else {
      if (inlineBadge) inlineBadge.textContent = 'Format unclear — will be auto-detected by server';
    }
    return;
  }

  // In file mode, show the filename and detected extension.
  const fileInput = document.getElementById('formatters-repair-file') as HTMLInputElement | null;
  if (inlineBadge) {
    if (fileInput?.files && fileInput.files.length > 0) {
      const f = fileInput.files[0];
      const ext = (f.name.split('.').pop() || '').toLowerCase();
      const norm = ext === 'jpeg' ? 'jpg' : ext;
      inlineBadge.textContent = `File: ${f.name} (${formatBytes(f.size)}) — ${norm.toUpperCase()}`;
    } else {
      inlineBadge.textContent = 'No file selected yet';
    }
  }

  // Keep the output meta element quiet until a repair completes.
  void outputMeta;
}

// ------------------------------------------------------------
// Lightweight client-side format sniffer
// ------------------------------------------------------------
function sniffTextFormat(text: string): string {
  const t = text.trim();
  if (!t) return '';

  // INI: starts with [section] on the very first line, and has at least
  // one `key=value` or `key:value` line. Must be checked BEFORE the
  // JSON-array sniff, since `[section]` starts with the same byte as a
  // JSON array.
  if (/^\[[^\]\n]{1,80}\]\s*$/m.test(t.split('\n')[0] ?? '')) {
    // Confirm it looks like an INI body: some line has = or :
    if (/^\s*[A-Za-z0-9_.-]+\s*[=:]/m.test(t)) {
      return 'ini';
    }
  }

  // JSON: starts with { or [
  if (t.startsWith('{') || t.startsWith('[')) return 'json';

  // HTML: starts with <!doctype html or <html
  if (/^<!doctype\s+html/i.test(t)) return 'html';
  if (/^<html[\s>]/i.test(t)) return 'html';

  // XML: starts with <?xml or a tag
  if (/^<\?xml/i.test(t)) return 'xml';
  if (/^<[A-Za-z!?]/.test(t)) return 'xml';

  // YAML
  if (/^---\s*$/m.test(t.split('\n')[0] ?? '')) return 'yaml';
  {
    const lines = t.split('\n').slice(0, 10);
    const kvCount = lines.filter(l => /^\s*[A-Za-z0-9_.-]+\s*:\s*/.test(l)).length;
    const hasList = lines.some(l => /^\s*-\s+/.test(l));
    if (kvCount >= 2 || (kvCount >= 1 && hasList)) return 'yaml';
  }

  // TOML / INI (no-section fallback)
  if (/^\s*[A-Za-z0-9_.-]+\s*=\s*\S/m.test(t)) {
    const hasTomlSignal =
      /^\s*[A-Za-z0-9_.-]+\s*=\s*"/m.test(t) ||
      /^\s*[A-Za-z0-9_.-]+\s*=\s*(true|false)\s*$/m.test(t) ||
      /^\s*[A-Za-z0-9_.-]+\s*=\s*\[/m.test(t);
    if (hasTomlSignal) return 'toml';
    return 'ini';
  }

  // Markdown
  if (/^#{1,6}\s+/.test(t)) return 'md';
  if (/\*\*[^*]+\*\*/.test(t) || /\[[^\]]+\]\([^)]+\)/.test(t)) return 'md';

  // CSV
  {
    const csvLines = t.split('\n').filter(l => l.trim()).slice(0, 5);
    if (csvLines.length >= 2) {
      const headerCommas = (csvLines[0].match(/,/g) || []).length;
      const dataHasComma = csvLines.slice(1).some(l => l.includes(','));
      if (headerCommas > 0 && dataHasComma) return 'csv';
    }
  }

  return 'txt';
}

// ============================================================
// Repair form submission — handles BOTH text and file inputs
// ============================================================
function wireRepairForm(): void {
  const form = document.getElementById('formatters-repair-form') as HTMLFormElement | null;
  if (!form) return;
  if ((form as any)._wired) return;
  (form as any)._wired = true;

  form.addEventListener('submit', handleRepairSubmit);
}

async function handleRepairSubmit(e: Event): Promise<void> {
  e.preventDefault();

  
  const describe  = document.getElementById('formatters-repair-describe') as HTMLInputElement;
  const statusEl  = document.getElementById('formatters-repair-status') as HTMLParagraphElement;
  const reportBox = document.getElementById('formatters-repair-report') as HTMLDivElement;

  statusEl.classList.remove('hidden', 'text-red-600', 'text-green-600');
  statusEl.classList.add('text-gray-500');
  reportBox.classList.add('hidden');
  hideOutputSection();

  if (!activeFormat) {
    statusEl.textContent = 'Pick a format above first.';
    statusEl.classList.replace('text-gray-500', 'text-red-600');
    return;
  }

  // ---------- Assemble the multipart payload ----------
  const fd = new FormData();
  let inferredFormat = activeFormat;
  let sourceLabel = '';

  if (inputMode === 'text') {
    const textarea = document.getElementById('formatters-repair-text-input') as HTMLTextAreaElement;
    const raw = textarea.value;

    if (!raw.trim()) {
      statusEl.textContent = 'Please paste some text first.';
      statusEl.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    inferredFormat = detectedFormatFromText || sniffTextFormat(raw) || activeFormat || 'txt';
    const filename = `pasted.${inferredFormat}`;
    const blob = new Blob([raw], { type: 'text/plain' });
    const file = new File([blob], filename, { type: 'text/plain' });

    fd.append('file', file);
    sourceLabel = `${inferredFormat.toUpperCase()} (${raw.length} chars)`;

  } else {
    const fileInput = document.getElementById('formatters-repair-file') as HTMLInputElement;

    if (!fileInput.files || fileInput.files.length === 0) {
      statusEl.textContent = 'Please select a file to repair.';
      statusEl.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    const f = fileInput.files[0];
    inferredFormat = (f.name.split('.').pop() || activeFormat || 'txt').toLowerCase();
    fd.append('file', f);
    sourceLabel = `${f.name} (${formatBytes(f.size)})`;
  }

 
  if (describe.checked) fd.append('describe', 'true');

  // ---------- Send the request ----------
  try {
    statusEl.textContent = `Repairing ${sourceLabel}…`;

       const apiUrl = getFormatApiUrl();
    const res = await fetch(apiUrl, { method: 'POST', body: fd });

    if (!res.ok) {
      const errText = await res.text().catch(() => '');
      throw new Error(errText || `Server error: ${res.status}`);
    }

    const ct = (res.headers.get('content-type') || '').toLowerCase();

    let outputText = '';
    let reportPayload: ReportPayload = {};

    if (ct.includes('application/json')) {
      const json = await res.json();
      outputText = json.output ?? '';
      reportPayload = {
        format:   json.format,
        level:    json.level,
        changed:  json.changed,
        applied:  json.applied,
        warnings: json.warnings,
      };
    } else {
      // describe=false → raw stream returned. Read as text so we can
      // still populate the output textarea for preview.
      outputText = await res.text();
      reportPayload = {
        format:  res.headers.get('X-Format-Format')  ?? inferredFormat,
        level:   'auto',
        changed: res.headers.get('X-Format-Changed') === 'true',
        applied: parseAppliedHeader(res.headers.get('X-Format-Applied')),
      };
    }

    lastRepairedOutput = outputText;
    lastRepairedFormat = (reportPayload.format || inferredFormat || 'txt').toLowerCase();

    populateReport(reportPayload);
    showOutputSection(outputText, lastRepairedFormat);

    statusEl.textContent = reportPayload.changed
      ? 'Repair complete — see output below.'
      : 'File was already valid — no changes made.';
    statusEl.classList.replace('text-gray-500', 'text-green-600');

  } catch (err) {
    console.error('[formatters] repair error:', err);
    statusEl.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
    statusEl.classList.replace('text-gray-500', 'text-red-600');
  }
}

// ============================================================
// Repair report card
// ============================================================
interface ReportPayload {
  format?: string;
  level?: string;
  changed?: boolean;
  applied?: string[];
  warnings?: string[];
}

function populateReport(json: ReportPayload): void {
  const reportBox = document.getElementById('formatters-repair-report');
  reportBox?.classList.remove('hidden');

  setText('formatters-result-format',  (json.format ?? '—').toString());
  setText('formatters-result-level',   (json.level  ?? '—').toString());
  setText('formatters-result-changed', json.changed ? 'Yes' : 'No');

  const fixesWrap = document.getElementById('formatters-result-fixes');
  if (fixesWrap) {
    fixesWrap.innerHTML = '';
    const list = json.applied && json.applied.length > 0
      ? json.applied
      : ['No fixes applied'];

    list.forEach(fix => {
      const chip = document.createElement('span');
      chip.className = 'inline-flex items-center gap-1 bg-white border border-amber-200 text-amber-800 text-xs font-medium px-3 py-1 rounded-full';
      chip.textContent = fix;
      fixesWrap.appendChild(chip);
    });
  }

  const warnWrap = document.getElementById('formatters-result-warnings-wrap');
  const warnList = document.getElementById('formatters-result-warnings');
  if (warnWrap && warnList) {
    warnList.innerHTML = '';
    if (json.warnings && json.warnings.length > 0) {
      json.warnings.forEach(w => {
        const li = document.createElement('li');
        li.textContent = w;
        warnList.appendChild(li);
      });
      warnWrap.classList.remove('hidden');
    } else {
      warnWrap.classList.add('hidden');
    }
  }
}

// ============================================================
// Output section — show repaired text + wire the two actions
// ============================================================
function showOutputSection(text: string, format: string): void {
  const section = document.getElementById('formatters-repair-output-section');
  const textarea = document.getElementById('formatters-repair-output-text') as HTMLTextAreaElement | null;
  const meta = document.getElementById('formatters-repair-output-meta');

  if (textarea) textarea.value = text;
  if (meta) {
    const lines = text ? text.split('\n').length : 0;
    meta.textContent = `${text.length} chars • ${lines} line${lines === 1 ? '' : 's'} • .${format}`;
  }
  section?.classList.remove('hidden');
}

function hideOutputSection(): void {
  document.getElementById('formatters-repair-output-section')?.classList.add('hidden');
  const textarea = document.getElementById('formatters-repair-output-text') as HTMLTextAreaElement | null;
  if (textarea) textarea.value = '';
}

function wireOutputActions(): void {
  const copyBtn = document.getElementById('formatters-repair-copy-btn') as HTMLButtonElement | null;
  const copyLbl = document.getElementById('formatters-repair-copy-btn-label') as HTMLSpanElement | null;
  const dlBtn = document.getElementById('formatters-repair-download-btn') as HTMLButtonElement | null;

  if (copyBtn && !(copyBtn as any)._wired) {
    (copyBtn as any)._wired = true;
    copyBtn.addEventListener('click', async () => {
      if (!lastRepairedOutput) return;

      const ok = await copyToClipboard(lastRepairedOutput);
      if (copyLbl) {
        const original = copyLbl.textContent || 'Copy Text';
        copyLbl.textContent = ok ? '✓ Copied' : 'Copy failed';
        copyBtn.disabled = true;
        setTimeout(() => {
          copyLbl.textContent = original;
          copyBtn.disabled = false;
        }, 1500);
      }
    });
  }

  if (dlBtn && !(dlBtn as any)._wired) {
    (dlBtn as any)._wired = true;
    dlBtn.addEventListener('click', () => {
      if (!lastRepairedOutput) return;
      const ext = lastRepairedFormat || activeFormat || 'txt';
      downloadText(lastRepairedOutput, `repaired.${ext}`);
    });
  }
}

// ============================================================
// Helpers
// ============================================================

function getFormatApiUrl(): string {
  const isLocal =
    window.location.hostname === 'localhost' ||
    window.location.hostname === '127.0.0.1';

  return isLocal
    ? 'http://localhost:8080/format'
    : 'https://utils.api.srilakshmiretail.in/format';
}

function setText(id: string, text: string): void {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
}

function downloadText(content: string, filename: string): void {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8;' });
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

// Parses the compact X-Repair-Applied header emitted by the backend:
//   "json;level=normal;changed=true;fixed=trailing-comma,single-quotes;warn=1"
function parseAppliedHeader(header: string | null): string[] {
  if (!header || header === 'none') return [];

  const parts = header.split(';');
  for (const part of parts) {
    if (part.startsWith('fixed=')) {
      return part
        .slice('fixed='.length)
        .split(',')
        .map(s => s.trim())
        .filter(Boolean);
    }
  }
  return [];
}

// Escapes HTML in user-facing strings so stray `<` doesn't break layout.
function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// Human-readable byte size
function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }
  return `${size.toFixed(size < 10 ? 1 : 0)} ${units[i]}`;
}

// Copy helper — self-contained so this module has no cross-file deps.
async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      /* fall through to legacy path */
    }
  }

  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    ta.style.top = '0';
    ta.setAttribute('readonly', '');
    document.body.appendChild(ta);
    ta.select();
    ta.setSelectionRange(0, ta.value.length);
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    if (ok) return true;
  } catch {
    /* fall through */
  }

  return false;
}