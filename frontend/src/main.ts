import './style.css'
import { initAuth, openModal } from './auth-ui'
import { initFormatters, renderFormattersCategory } from './formatters'
import { initTasks, renderTasksCategory } from './tasks-ui'
import { getAuthState, isLoggedIn, clearAuth } from './auth'
import { initGPA } from './gpa-ui'

// ============================================================
// EMI Calculator — shared types (module scope)
// ============================================================

/** Calculation mode for the EMI view. */
type EmiMode = 'banker' | 'borrower';

/** One row of the amortization schedule, as returned by the API. */
interface EMIAmortRow {
  month: number;
  openingBalance: number;
  principalPaid: number;
  interestPaid: number;
  totalPaid: number;
  closingBalance: number;
}

/** Snapshot of a completed banker-mode EMI calculation. */
interface EMISnapshot {
  principal: number;
  annualRate: number;
  tenureMonths: number;
  tenureInput: number;
  tenureUnit: string;
  monthlyRatePercent: number;
  emi: number;
  totalInterest: number;
  totalPayment: number;
  principalPercent: number;
  interestPercent: number;
  amortization: EMIAmortRow[];
}

/** Snapshot of a completed borrower-mode loan-tenure calculation. */
interface LoanTenureSnapshot {
  principal: number;
  annualRate: number;
  monthlyPayment: number;
  tenureMonths: number;
  tenureYears: number;
  monthlyRatePercent: number;
  totalInterest: number;
  totalPayment: number;
  principalPercent: number;
  interestPercent: number;
  amortization: EMIAmortRow[];
}

// ============================================================
// TOOL REGISTRY — single source of truth for navigation
// ============================================================
type ToolId =
  | 'file' | 'pdf' | 'image'  | 'measure' | 'time' | 'railway'
  | 'bmi' | 'age' | 'percentage' | 'emi' | 'tax' | 'interest' | 'scientific' | 'gpa'
  | 'formatters'
  | 'tasks';

type CategoryId = 'conversion' | 'calculations' | 'formatters' | 'tasks';

interface ToolDef {
  id: ToolId;
  label: string;
  viewId: string;
}

const TOOLS: Record<CategoryId, ToolDef[]> = {
  conversion: [
    { id: 'file', label: 'File', viewId: 'file-converter-view' },
    { id: 'pdf', label: 'PDF Size', viewId: 'pdf-converter-view' },
    { id: 'image', label: 'Image', viewId: 'image-compressor-view' },
    { id: 'measure', label: 'Measurements', viewId: 'measure-converter-view' },
    { id: 'time', label: 'Time Zones', viewId: 'time-converter-view' },
    { id: 'railway', label: 'Railway', viewId: 'railway-converter-view' },
  ],
  calculations: [
    { id: 'bmi', label: 'BMI', viewId: 'hr-calculator-view' },
    { id: 'age', label: 'Age', viewId: 'age-calculator-view' },
    { id: 'percentage', label: 'Percentage', viewId: 'percentage-calculator-view' },
    { id: 'emi', label: 'EMI', viewId: 'emi-calculator-view' },
    { id: 'tax', label: 'Tax / GST', viewId: 'tax-calculator-view' },
    { id: 'interest', label: 'SI / CI', viewId: 'interest-calculator-view' },
    { id: 'scientific', label: 'Scientific', viewId: 'scientific-calculator-view' },
     { id: 'gpa', label: 'CGPA', viewId: 'gpa-calculator-view' },
  ],
  formatters: [
    { id: 'formatters', label: 'Formatter', viewId: 'formatters-view' },
  ],
    tasks: [
    { id: 'tasks', label: 'Tasks', viewId: 'tasks-view' },
  ],
};

const CATEGORY_LABELS: Record<CategoryId, string> = {
  conversion: 'Conversion',
  calculations: 'Calculations',
  formatters: 'Formatter',
  tasks: 'Tasks',
};

// ============================================================
// DOM REFERENCES — navigation
// ============================================================
const sidebar = document.getElementById('sidebar') as HTMLElement | null;
const mobileMenuBtn = document.getElementById('mobile-menu-btn') as HTMLButtonElement | null;
const mobileMenuClose = document.getElementById('mobile-menu-close') as HTMLButtonElement | null;
const mobileOverlay = document.getElementById('mobile-overlay') as HTMLDivElement | null;

const btnCategoryConversion = document.getElementById('btn-category-conversion') as HTMLButtonElement;
const btnCategoryCalculations = document.getElementById('btn-category-calculations') as HTMLButtonElement;
const btnCategoryFormatters = document.getElementById('btn-category-formatters') as HTMLButtonElement;
const btnCategoryTasks = document.getElementById('btn-category-tasks') as HTMLButtonElement;
const categoryTitle = document.getElementById('category-title') as HTMLHeadingElement;
const breadcrumbTool = document.getElementById('breadcrumb-tool') as HTMLSpanElement;
const tabStrip = document.getElementById('tab-strip') as HTMLDivElement;

const allToolViews = Array.from(
  document.querySelectorAll<HTMLDivElement>('.tool-view')
);

// ============================================================
// STATE
// ============================================================
let currentCategory: CategoryId = 'conversion';
let currentTool: ToolId = 'file';

// ============================================================
// MOBILE SIDEBAR TOGGLE
// ============================================================
function openMobileMenu() {
  if (!sidebar) return;
  sidebar.classList.remove('-translate-x-full');
  sidebar.classList.add('translate-x-0');
  mobileOverlay?.classList.remove('hidden');
}

function closeMobileMenu() {
  if (!sidebar) return;
  sidebar.classList.add('-translate-x-full');
  sidebar.classList.remove('translate-x-0');
  mobileOverlay?.classList.add('hidden');
}

mobileMenuBtn?.addEventListener('click', openMobileMenu);
mobileMenuClose?.addEventListener('click', closeMobileMenu);
mobileOverlay?.addEventListener('click', closeMobileMenu);

[btnCategoryConversion, btnCategoryCalculations, btnCategoryFormatters, btnCategoryTasks].forEach(btn => {
  btn?.addEventListener('click', () => {
    if (window.innerWidth < 768) {
      setTimeout(closeMobileMenu, 150);
    }
  });
});

// ============================================================
// NAVIGATION — categories & tabs
// ============================================================
function renderTabs(category: CategoryId) {
  tabStrip.innerHTML = '';
  const tools = TOOLS[category];

  tools.forEach(tool => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.dataset.tool = tool.id;
    btn.dataset.toolLabel = tool.label;

    const active = tool.id === currentTool;
    btn.className = [
      'whitespace-nowrap',
      'px-4',
      'py-2',
      'rounded-full',
      'text-sm',
      'font-medium',
      'transition-colors',
      'border',
      active
        ? 'bg-indigo-600 text-white border-indigo-600 shadow-sm'
        : 'bg-white text-gray-600 border-gray-300 hover:bg-indigo-50 hover:text-indigo-700 hover:border-indigo-300',
    ].join(' ');

    btn.textContent = tool.label;

    btn.addEventListener('click', () => {
      setTool(tool.id);
    });

    tabStrip.appendChild(btn);
  });
}

function highlightCategoryButtons() {
  const activeClasses = ['bg-indigo-800', 'text-white'];
  const inactiveClasses = ['text-indigo-200', 'hover:bg-indigo-800', 'hover:text-white'];

  const pairs: [HTMLButtonElement | null, CategoryId][] = [
    [btnCategoryConversion, 'conversion'],
    [btnCategoryCalculations, 'calculations'],
    [btnCategoryFormatters, 'formatters'],
    [btnCategoryTasks, 'tasks'],
  ];

  pairs.forEach(([btn, cat]) => {
    if (!btn) return;
    const isActive = currentCategory === cat;
    btn.classList.remove(...activeClasses, ...inactiveClasses);
    if (isActive) {
      btn.classList.add(...activeClasses);
    } else {
      btn.classList.add(...inactiveClasses);
    }
  });
}

function setTool(toolId: ToolId) {
  const toolsInCategory = TOOLS[currentCategory];
  const toolExists = toolsInCategory.some(t => t.id === toolId);

  if (!toolExists) {
    for (const cat of Object.keys(TOOLS) as CategoryId[]) {
      if (TOOLS[cat].some(t => t.id === toolId)) {
        currentCategory = cat;
        break;
      }
    }
  }

  currentTool = toolId;

  categoryTitle.textContent = CATEGORY_LABELS[currentCategory];
  const toolDef = TOOLS[currentCategory].find(t => t.id === currentTool)!;
  breadcrumbTool.textContent = toolDef.label;

  renderTabs(currentCategory);

  allToolViews.forEach(view => {
    if (view.id === toolDef.viewId) {
      view.classList.remove('hidden');
    } else {
      view.classList.add('hidden');
    }
  });

  highlightCategoryButtons();

  if (toolDef.viewId === 'formatters-view') {
    initFormatters();
    renderFormattersCategory();
  }

  if (toolDef.viewId === 'tasks-view') {
    initTasks();
    renderTasksCategory();
  }

  const activeTab = tabStrip.querySelector<HTMLButtonElement>(
    `button[data-tool="${currentTool}"]`
  );
  activeTab?.scrollIntoView({ behavior: 'smooth', inline: 'center', block: 'nearest' });

  const viewport = document.querySelector('main > .flex-1.overflow-y-auto');
  viewport?.scrollTo({ top: 0, behavior: 'smooth' });
}

function setCategory(category: CategoryId, preserveTool = true) {
  currentCategory = category;

  const tools = TOOLS[category];
  let landingTool: ToolId;

  if (preserveTool && tools.some(t => t.id === currentTool)) {
    landingTool = currentTool;
  } else {
    landingTool = tools[0].id;
  }

  setTool(landingTool);
}

// ============================================================
// CATEGORY BUTTON LISTENERS
// ============================================================
btnCategoryConversion.addEventListener('click', () => setCategory('conversion'));
btnCategoryCalculations.addEventListener('click', () => setCategory('calculations'));
btnCategoryFormatters.addEventListener('click', () => setCategory('formatters'));
btnCategoryTasks.addEventListener('click', () => setCategory('tasks'));

document.querySelector('#breadcrumb > span:first-child')?.addEventListener('click', () => {
  setCategory(currentCategory, true);
});

// ============================================================
// INITIAL RENDER & ROUTE HANDLING
// ============================================================
initFormatters();
initTasks();
initAuth();
initGPA();
setCategory('conversion', false);

// ============================================================
// ------------------------------------------------------------
// EVERYTHING BELOW IS EXISTING CONVERTER LOGIC
// ------------------------------------------------------------
// ============================================================

// ----------------------------------------------------
// FILE CONVERTER LOGIC
// ----------------------------------------------------
const toSelect = document.getElementById('to-type') as HTMLSelectElement;
const fileInput = document.getElementById('file-input') as HTMLInputElement;
const convertForm = document.getElementById('convert-form') as HTMLFormElement;
const statusMessage = document.getElementById('status-message') as HTMLParagraphElement;
const detectionBox = document.getElementById('detection-box') as HTMLDivElement;
const detectedFormatText = document.getElementById('detected-format-text') as HTMLSpanElement;

let detectedFromType = '';

fileInput.addEventListener('change', () => {
  if (fileInput.files && fileInput.files.length > 0) {
    const file = fileInput.files[0];
    const ext = file.name.split('.').pop()?.toLowerCase();

    if (ext) {
      detectedFromType = ext;
      detectedFormatText.textContent = ext.toUpperCase() + ' (Detected)';
      detectionBox.classList.replace('bg-gray-100', 'bg-indigo-50');
      detectionBox.classList.replace('border-gray-200', 'border-indigo-200');
      detectedFormatText.classList.replace('text-gray-500', 'text-indigo-700');
    } else {
      resetDetectionBox();
    }
  } else {
    resetDetectionBox();
  }
});

function resetDetectionBox() {
  detectedFromType = '';
  detectedFormatText.textContent = 'Auto-detected on upload';
  detectionBox.classList.replace('bg-indigo-50', 'bg-gray-100');
  detectionBox.classList.replace('border-indigo-200', 'border-gray-200');
  detectedFormatText.classList.replace('text-indigo-700', 'text-gray-500');
}

convertForm.addEventListener('submit', async (e) => {
  e.preventDefault();

  statusMessage.classList.remove('hidden', 'text-red-600', 'text-green-600');
  statusMessage.classList.add('text-gray-500');

  if (!fileInput.files || fileInput.files.length === 0 || !detectedFromType) {
    statusMessage.textContent = 'Please select a valid file first.';
    statusMessage.classList.add('text-red-600');
    return;
  }

  const file = fileInput.files[0];
  const toType = toSelect.value;

  if (detectedFromType === toType) {
    statusMessage.textContent = 'Source and target formats cannot be the same.';
    statusMessage.classList.add('text-red-600');
    return;
  }

  const formData = new FormData();
  formData.append('file', file);
  formData.append('fromType', detectedFromType);
  formData.append('toType', toType);

  try {
    statusMessage.textContent = `Converting ${detectedFromType.toUpperCase()} to ${toType.toUpperCase()}...`;

    const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    const apiUrl = isLocal
      ? 'http://localhost:8080/convert'
      : 'https://utils.api.srilakshmiretail.in/convert';

    const response = await fetch(apiUrl, {
      method: 'POST',
      body: formData,
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(errText || `Server error: ${response.status}`);
    }

    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.style.display = 'none';
    a.href = url;
    a.download = `converted.${toType}`;
    document.body.appendChild(a);
    a.click();
    window.URL.revokeObjectURL(url);
    document.body.removeChild(a);

    statusMessage.textContent = `Success! File converted to ${toType.toUpperCase()}.`;
    statusMessage.classList.replace('text-gray-500', 'text-green-600');
  } catch (error) {
    console.error('Conversion error:', error);
    statusMessage.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
    statusMessage.classList.replace('text-gray-500', 'text-red-600');
  }
});

// ----------------------------------------------------
// PDF SIZE CONVERTER LOGIC
// ----------------------------------------------------
const pdfForm = document.getElementById('pdf-form') as HTMLFormElement;
const pdfFileInput = document.getElementById('pdf-file-input') as HTMLInputElement;
const pdfConversionType = document.getElementById('pdf-conversion-type') as HTMLSelectElement;
const pdfTargetSize = document.getElementById('pdf-target-size') as HTMLInputElement;
const pdfDataType = document.getElementById('pdf-data-type') as HTMLSelectElement;
const pdfStatusMessage = document.getElementById('pdf-status-message') as HTMLParagraphElement;

pdfForm.addEventListener('submit', async (e) => {
  e.preventDefault();

  pdfStatusMessage.classList.remove('hidden', 'text-red-600', 'text-green-600');
  pdfStatusMessage.classList.add('text-gray-500');

  if (!pdfFileInput.files || pdfFileInput.files.length === 0) {
    pdfStatusMessage.textContent = 'Please select a PDF file first.';
    pdfStatusMessage.classList.add('text-red-600');
    return;
  }

  const file = pdfFileInput.files[0];
  const formData = new FormData();
  formData.append('file', file);
  formData.append('conversionType', pdfConversionType.value);
  formData.append('dataType', pdfDataType.value);
  formData.append('targetSize', pdfTargetSize.value);

  try {
    pdfStatusMessage.textContent = `Converting PDF...`;

    const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    const apiUrl = isLocal
      ? 'http://localhost:8080/convert-pdf-size'
      : 'https://utils.api.srilakshmiretail.in/convert-pdf-size';

    const response = await fetch(apiUrl, {
      method: 'POST',
      body: formData,
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(errText || `Server error: ${response.status}`);
    }

    const contentDisposition = response.headers.get('Content-Disposition');
    let filename = 'converted.pdf';
    if (contentDisposition && contentDisposition.includes('filename=')) {
      filename = contentDisposition.split('filename=')[1].replace(/"/g, '');
    }

    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.style.display = 'none';
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    window.URL.revokeObjectURL(url);
    document.body.removeChild(a);

    pdfStatusMessage.textContent = `Success! PDF converted and downloading.`;
    pdfStatusMessage.classList.replace('text-gray-500', 'text-green-600');
  } catch (error) {
    console.error('PDF Conversion error:', error);
    pdfStatusMessage.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
    pdfStatusMessage.classList.replace('text-gray-500', 'text-red-600');
  }
});

// ----------------------------------------------------
// MEASUREMENT CONVERTER LOGIC
// ----------------------------------------------------
const unitsData: Record<string, string[]> = {
  length: ["meters", "kilometers", "centimeters", "millimeters", "miles", "yards", "feet", "inches"],
  weight: ["kilograms", "grams", "milligrams", "pounds", "ounces"],
  volume: ["liters", "milliliters", "gallons", "quarts", "pints", "fluid_ounces"],
  area: ["square_meters", "square_kilometers", "hectares", "acres", "square_feet", "square_miles"],
  time: ["seconds", "minutes", "hours", "days", "weeks"],
  temperature: ["celsius", "fahrenheit", "kelvin"],
  speed: ["meters_per_second", "kilometers_per_hour", "miles_per_hour", "feet_per_second", "knots"],
  data: ["bytes", "kilobytes", "megabytes", "gigabytes", "terabytes", "petabytes", "bits"],
  numeral: ["binary", "octal", "decimal", "hexadecimal"]
};

const measureCategory = document.getElementById('measure-category') as HTMLSelectElement;
const measureFrom = document.getElementById('measure-from') as HTMLSelectElement;
const measureTo = document.getElementById('measure-to') as HTMLSelectElement;
const measureValue = document.getElementById('measure-value') as HTMLInputElement;
const measureForm = document.getElementById('measure-form') as HTMLFormElement;
const measureResultBox = document.getElementById('measure-result-box') as HTMLDivElement;
const measureResultText = document.getElementById('measure-result-text') as HTMLSpanElement;
const measureStatusMessage = document.getElementById('measure-status-message') as HTMLParagraphElement;

function populateUnits(category: string) {
  measureFrom.innerHTML = '';
  measureTo.innerHTML = '';

  const units = unitsData[category] || [];
  units.forEach(unit => {
    const label = unit.split('_').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
    measureFrom.add(new Option(label, unit));
    measureTo.add(new Option(label, unit));
  });

  if (units.length > 1) {
    measureTo.selectedIndex = 1;
  }

  if (category === 'numeral') {
    measureValue.type = 'text';
    measureValue.placeholder = 'e.g., 1011 or FF';
  } else {
    measureValue.type = 'number';
    measureValue.placeholder = 'Enter value to convert';
  }
}

populateUnits(measureCategory.value);

measureCategory.addEventListener('change', () => {
  populateUnits(measureCategory.value);
  measureResultBox.classList.add('hidden');
});

measureForm.addEventListener('submit', async (e) => {
  e.preventDefault();

  measureStatusMessage.classList.remove('hidden', 'text-red-600', 'text-green-600');
  measureStatusMessage.classList.add('text-gray-500');
  measureStatusMessage.textContent = 'Converting...';
  measureResultBox.classList.add('hidden');

  const isNumeral = measureCategory.value === 'numeral';

  const payload = isNumeral ? {
    fromBase: measureFrom.value,
    toBase: measureTo.value,
    value: measureValue.value
  } : {
    category: measureCategory.value,
    fromUnit: measureFrom.value,
    toUnit: measureTo.value,
    value: parseFloat(measureValue.value)
  };

  try {
    const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';

    let apiPath = isNumeral ? '/convert-numeral' : '/convert-measurement';
    const apiUrl = isLocal
      ? `http://localhost:8080${apiPath}`
      : `https://utils.api.srilakshmiretail.in${apiPath}`;

    const response = await fetch(apiUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(errText || `Server error: ${response.status}`);
    }

    const data = await response.json();

    let formattedResult;
    if (isNumeral) {
      formattedResult = data.result;
    } else {
      formattedResult = Number.isInteger(data.result) ? data.result : Number(data.result.toFixed(6));
    }

    measureResultText.textContent = `${formattedResult}`;
    measureResultBox.classList.remove('hidden');
    measureStatusMessage.classList.add('hidden');

  } catch (error) {
    console.error('Measurement conversion error:', error);
    measureStatusMessage.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
    measureStatusMessage.classList.replace('text-gray-500', 'text-red-600');
  }
});

// ----------------------------------------------------
// TIME CONVERTER LOGIC
// ----------------------------------------------------
const timeForm = document.getElementById('time-form') as HTMLFormElement;
const timeDateInput = document.getElementById('time-date') as HTMLInputElement;
const timeHourInput = document.getElementById('time-hour') as HTMLSelectElement;
const timeMinuteInput = document.getElementById('time-minute') as HTMLSelectElement;
const timeAmpmInput = document.getElementById('time-ampm') as HTMLSelectElement;
const timeSourceTzInput = document.getElementById('time-source-tz') as HTMLSelectElement;
const timeDestTzInput = document.getElementById('time-dest-tz') as HTMLSelectElement;
const timeResultBox = document.getElementById('time-result-box') as HTMLDivElement;
const timeResultLocal = document.getElementById('time-result-local') as HTMLSpanElement;
const timeResultZone = document.getElementById('time-result-zone') as HTMLSpanElement;
const timeWarning = document.getElementById('time-warning') as HTMLParagraphElement;
const timeStatusMessage = document.getElementById('time-status-message') as HTMLParagraphElement;

const timezones = [
  "UTC", "America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
  "America/Phoenix", "America/Anchorage", "America/Honolulu", "America/Sao_Paulo",
  "America/Argentina/Buenos_Aires", "America/Bogota", "Europe/London", "Europe/Paris",
  "Europe/Berlin", "Europe/Rome", "Europe/Moscow", "Africa/Cairo", "Africa/Johannesburg",
  "Africa/Lagos", "Asia/Dubai", "Asia/Kolkata", "Asia/Dhaka", "Asia/Bangkok", "Asia/Singapore",
  "Asia/Hong_Kong", "Asia/Shanghai", "Asia/Tokyo", "Asia/Seoul", "Australia/Sydney",
  "Australia/Melbourne", "Australia/Brisbane", "Australia/Perth", "Pacific/Auckland", "Pacific/Fiji"
];

timezones.forEach(tz => {
  timeSourceTzInput.add(new Option(tz, tz));
  timeDestTzInput.add(new Option(tz, tz));
});

for (let h = 1; h <= 12; h++) {
  const hr = h.toString().padStart(2, '0');
  timeHourInput.add(new Option(hr, hr));
}
for (let m = 0; m <= 59; m++) {
  const min = m.toString().padStart(2, '0');
  timeMinuteInput.add(new Option(min, min));
}

timeSourceTzInput.value = "America/New_York";
timeDestTzInput.value = "Asia/Tokyo";

timeForm.addEventListener('submit', async (e) => {
  e.preventDefault();

  timeStatusMessage.classList.remove('hidden', 'text-red-600', 'text-green-600');
  timeStatusMessage.classList.add('text-gray-500');
  timeStatusMessage.textContent = 'Converting...';
  timeResultBox.classList.add('hidden');
  timeWarning.classList.add('hidden');

  const dateValue = timeDateInput.value;
  if (!dateValue) return;

  const dateParts = dateValue.split('-');

  let hour = parseInt(timeHourInput.value, 10);
  const minute = parseInt(timeMinuteInput.value, 10);
  const ampm = timeAmpmInput.value;

  if (ampm === "PM" && hour !== 12) hour += 12;
  if (ampm === "AM" && hour === 12) hour = 0;

  const payload = {
    year: parseInt(dateParts[0], 10),
    month: parseInt(dateParts[1], 10),
    day: parseInt(dateParts[2], 10),
    hour: hour,
    minute: minute,
    second: 0,
    source_tz: timeSourceTzInput.value,
    dest_tz: timeDestTzInput.value,
    ambiguous_policy: "first",
    non_existent_policy: "forward"
  };

  try {
    const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    const apiUrl = isLocal
      ? 'http://localhost:8080/convert-time'
      : 'https://utils.api.srilakshmiretail.in/convert-time';

    const response = await fetch(apiUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(errText || `Server error: ${response.status}`);
    }

    const data = await response.json();

    const destLocalParts = data.dest_time_local.split('T');
    const destDate = destLocalParts[0];
    const timeWithOffset = destLocalParts[1];

    const [hourStr, minStr] = timeWithOffset.split(':');
    let hour12 = parseInt(hourStr, 10);
    const ampmOut = hour12 >= 12 ? 'PM' : 'AM';
    hour12 = hour12 % 12;
    if (hour12 === 0) hour12 = 12;

    const destTime12 = `${hour12.toString().padStart(2, '0')}:${minStr} ${ampmOut}`;

    timeResultLocal.textContent = `${destDate} ${destTime12}`;

    let zoneText = `${data.dest_zone_name} (UTC ${data.dest_offset})`;
    if (data.is_next_day) zoneText += ' Next Day';
    if (data.is_prev_day) zoneText += ' Previous Day';

    timeResultZone.textContent = zoneText;

    if (data.warning) {
      timeWarning.textContent = data.warning;
      timeWarning.classList.remove('hidden');
    }

    timeResultBox.classList.remove('hidden');
    timeStatusMessage.classList.add('hidden');

  } catch (error) {
    console.error('Time conversion error:', error);
    timeStatusMessage.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
    timeStatusMessage.classList.replace('text-gray-500', 'text-red-600');
  }
});

// ----------------------------------------------------
// RAILWAY CONVERTER LOGIC
// ----------------------------------------------------
const rw12Hour = document.getElementById('railway-12-hour') as HTMLInputElement;
const rw12Min = document.getElementById('railway-12-min') as HTMLInputElement;
const rw12Ampm = document.getElementById('railway-12-ampm') as HTMLSelectElement;

const rw24Hour = document.getElementById('railway-24-hour') as HTMLInputElement;
const rw24Min = document.getElementById('railway-24-min') as HTMLInputElement;

const railwayStatusMessage = document.getElementById('railway-status-message') as HTMLParagraphElement;

let isUpdatingRailway = false;

async function sync12to24() {
  if (isUpdatingRailway) return;

  const h = parseInt(rw12Hour.value, 10);
  const m = parseInt(rw12Min.value, 10);
  if (isNaN(h) || isNaN(m)) return;

  isUpdatingRailway = true;

  const payload = {
    direction: "12to24",
    hour: h,
    minute: m,
    ampm: rw12Ampm.value
  };

  try {
    const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    const apiUrl = isLocal
      ? 'http://localhost:8080/convert-railway'
      : 'https://utils.api.srilakshmiretail.in/convert-railway';

    const res = await fetch(apiUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });

    if (res.ok) {
      const data = await res.json();
      rw24Hour.value = data.hour_24.toString().padStart(2, '0');
      rw24Min.value = data.minute.toString().padStart(2, '0');
      railwayStatusMessage.classList.add('hidden');
    } else {
      throw new Error(await res.text());
    }
  } catch (err) {
    console.error("Railway convert error", err);
    railwayStatusMessage.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown'}`;
    railwayStatusMessage.classList.remove('hidden');
  } finally {
    isUpdatingRailway = false;
  }
}

async function sync24to12() {
  if (isUpdatingRailway) return;

  const h = parseInt(rw24Hour.value, 10);
  const m = parseInt(rw24Min.value, 10);
  if (isNaN(h) || isNaN(m)) return;

  isUpdatingRailway = true;

  const payload = {
    direction: "24to12",
    hour: h,
    minute: m
  };

  try {
    const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
    const apiUrl = isLocal
      ? 'http://localhost:8080/convert-railway'
      : 'https://utils.api.srilakshmiretail.in/convert-railway';

    const res = await fetch(apiUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });

    if (res.ok) {
      const data = await res.json();
      rw12Hour.value = data.hour_12.toString().padStart(2, '0');
      rw12Min.value = data.minute.toString().padStart(2, '0');
      rw12Ampm.value = data.ampm;
      railwayStatusMessage.classList.add('hidden');
    } else {
      throw new Error(await res.text());
    }
  } catch (err) {
    console.error("Railway convert error", err);
    railwayStatusMessage.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown'}`;
    railwayStatusMessage.classList.remove('hidden');
  } finally {
    isUpdatingRailway = false;
  }
}

['input', 'change'].forEach(evt => {
  rw12Hour.addEventListener(evt, sync12to24);
  rw12Min.addEventListener(evt, sync12to24);
  rw12Ampm.addEventListener(evt, sync12to24);

  rw24Hour.addEventListener(evt, sync24to12);
  rw24Min.addEventListener(evt, sync24to12);
});

rw12Hour.value = "12";
rw12Min.value = "00";
rw12Ampm.value = "PM";
sync12to24();

// ----------------------------------------------------
// AGE CALCULATOR LOGIC
// ----------------------------------------------------
const ageForm = document.getElementById('age-form') as HTMLFormElement;
const ageDob = document.getElementById('age-dob') as HTMLInputElement;
const ageToday = document.getElementById('age-today') as HTMLInputElement;
const ageResultBox = document.getElementById('age-result-box') as HTMLDivElement;
const ageResultText = document.getElementById('age-result-text') as HTMLParagraphElement;
const ageNextBirthday = document.getElementById('age-next-birthday') as HTMLParagraphElement;
const ageSummaryText = document.getElementById('age-summary-text') as HTMLParagraphElement;
const ageStatusMessage = document.getElementById('age-status-message') as HTMLParagraphElement;

if (ageForm) {
  const today = new Date();
  const isoToday = today.toISOString().split('T')[0];
  ageToday.value = isoToday;

  ageForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    ageResultBox.classList.add('hidden');
    ageStatusMessage.classList.add('hidden');

    const dob = ageDob.value;
    const todayDate = ageToday.value;

    if (!dob || !todayDate) {
      ageStatusMessage.textContent = 'Please enter both dates.';
      ageStatusMessage.classList.remove('hidden');
      ageStatusMessage.classList.add('text-red-600');
      return;
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/calculate-age'
        : 'https://utils.api.srilakshmiretail.in/calculate-age';

      const response = await fetch(apiUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ dob, today: todayDate })
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(errText || `Server error: ${response.status}`);
      }

      const data = await response.json();
      const age = data.age;
      const nextBirthday = data.nextBirthday;
      const summary = data.summary;

      ageResultText.textContent = `${age.years} Years, ${age.months} Months, ${age.days} Days`;
      ageNextBirthday.textContent = `${nextBirthday.dayOfWeek} - ${nextBirthday.date} (${nextBirthday.month}/${nextBirthday.day})`;
      ageSummaryText.textContent = `${summary.years}y, ${summary.months}m, ${summary.weeks}w, ${summary.days}d, ${summary.hours}h, ${summary.minutes}m`;

      ageResultBox.classList.remove('hidden');
    } catch (error) {
      console.error('Age calculation error:', error);
      ageStatusMessage.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
      ageStatusMessage.classList.remove('hidden');
      ageStatusMessage.classList.add('text-red-600');
    }
  });
}

// ----------------------------------------------------
// BMI CALCULATOR LOGIC
// ----------------------------------------------------
const bmiForm = document.getElementById('bmi-form') as HTMLFormElement;
const bmiWeight = document.getElementById('bmi-weight') as HTMLInputElement;
const bmiWeightUnit = document.getElementById('bmi-weight-unit') as HTMLSelectElement;
const bmiHeight = document.getElementById('bmi-height') as HTMLInputElement;
const bmiHeightUnit = document.getElementById('bmi-height-unit') as HTMLSelectElement;
const bmiResultSection = document.getElementById('bmi-result-section') as HTMLDivElement;
const bmiResultValue = document.getElementById('bmi-result-value') as HTMLSpanElement;
const bmiResultCategory = document.getElementById('bmi-result-category') as HTMLSpanElement;
const bmiErrorMessage = document.getElementById('bmi-error-message') as HTMLDivElement;

if (bmiForm) {
  bmiForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    bmiResultSection.classList.add('hidden');
    bmiErrorMessage.classList.add('hidden');

    const payload = {
      weight: parseFloat(bmiWeight.value),
      weightUnit: bmiWeightUnit.value,
      height: parseFloat(bmiHeight.value),
      heightUnit: bmiHeightUnit.value
    };

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/calculate-bmi'
        : 'https://utils.api.srilakshmiretail.in/calculate-bmi';

      const response = await fetch(apiUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(errText || `Server error: ${response.status}`);
      }

      const data = await response.json();

      bmiResultValue.textContent = data.bmi.toFixed(2);
      bmiResultCategory.textContent = data.category;

      bmiResultValue.className = 'text-4xl font-extrabold';
      if (data.category === 'Underweight') {
        bmiResultValue.classList.add('text-blue-500');
      } else if (data.category === 'Normal weight') {
        bmiResultValue.classList.add('text-green-500');
      } else if (data.category === 'Overweight') {
        bmiResultValue.classList.add('text-yellow-500');
      } else {
        bmiResultValue.classList.add('text-red-500');
      }

      bmiResultSection.classList.remove('hidden');
    } catch (error) {
      console.error('BMI calculation error:', error);
      bmiErrorMessage.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
      bmiErrorMessage.classList.remove('hidden');
    }
  });
}

// ----------------------------------------------------
// PERCENTAGE CALCULATOR LOGIC
// ----------------------------------------------------

interface PctFieldConfig {
  label: string;
  placeholder: string;
}

interface PctOpConfig {
  field1: PctFieldConfig;
  field2?: PctFieldConfig;
  field3?: PctFieldConfig;
}

const PERCENTAGE_OPS: Record<string, PctOpConfig> = {
  percent_of: {
    field1: { label: 'Percent (%)', placeholder: 'e.g. 15' },
    field2: { label: 'Of number', placeholder: 'e.g. 200' },
  },
  what_percent: {
    field1: { label: 'Part', placeholder: 'e.g. 30' },
    field2: { label: 'Whole', placeholder: 'e.g. 200' },
  },
  is_percent_of_what: {
    field1: { label: 'Part', placeholder: 'e.g. 30' },
    field2: { label: 'Percent (%)', placeholder: 'e.g. 15' },
  },
  percent_change: {
    field1: { label: 'Old value', placeholder: 'e.g. 100' },
    field2: { label: 'New value', placeholder: 'e.g. 150' },
  },
  percent_increase: {
    field1: { label: 'Number', placeholder: 'e.g. 200' },
    field2: { label: 'Increase by (%)', placeholder: 'e.g. 15' },
  },
  percent_decrease: {
    field1: { label: 'Number', placeholder: 'e.g. 200' },
    field2: { label: 'Decrease by (%)', placeholder: 'e.g. 15' },
  },
  reverse_percent: {
    field1: { label: 'Final value', placeholder: 'e.g. 230' },
    field2: { label: 'Percent applied (%)', placeholder: 'e.g. 15' },
  },
  percent_difference: {
    field1: { label: 'Value A', placeholder: 'e.g. 100' },
    field2: { label: 'Value B', placeholder: 'e.g. 150' },
  },
  add_percent_points: {
    field1: { label: 'First percent (%)', placeholder: 'e.g. 5' },
    field2: { label: 'Second percent (%)', placeholder: 'e.g. 3' },
  },
  subtract_percent_points: {
    field1: { label: 'First percent (%)', placeholder: 'e.g. 5' },
    field2: { label: 'Second percent (%)', placeholder: 'e.g. 3' },
  },
  discount: {
    field1: { label: 'Original price', placeholder: 'e.g. 500' },
    field2: { label: 'Discount (%)', placeholder: 'e.g. 20' },
  },
  markup: {
    field1: { label: 'Cost price', placeholder: 'e.g. 100' },
    field2: { label: 'Markup (%)', placeholder: 'e.g. 20' },
  },
  profit_loss: {
    field1: { label: 'Cost price', placeholder: 'e.g. 100' },
    field2: { label: 'Selling price', placeholder: 'e.g. 120' },
  },
  percent_to_fraction: {
    field1: { label: 'Percent (%)', placeholder: 'e.g. 25' },
  },
  fraction_to_percent: {
    field1: { label: 'Numerator', placeholder: 'e.g. 1' },
    field2: { label: 'Denominator', placeholder: 'e.g. 4' },
  },
  decimal_to_percent: {
    field1: { label: 'Decimal', placeholder: 'e.g. 0.25' },
  },
  compound_percent: {
    field1: { label: 'Base amount', placeholder: 'e.g. 1000' },
    field2: { label: 'Percent per period (%)', placeholder: 'e.g. 10' },
    field3: { label: 'Number of periods', placeholder: 'e.g. 3' },
  },
  marks_percentage: {
    field1: { label: 'Obtained marks', placeholder: 'e.g. 85' },
    field2: { label: 'Total marks', placeholder: 'e.g. 100' },
  },
  cgpa_to_percent: {
    field1: { label: 'CGPA', placeholder: 'e.g. 8.0' },
  },
};

const PERCENTAGE_OP_NAMES: Record<string, string> = {
  percent_of: 'X% of Y',
  what_percent: 'X is what % of Y',
  is_percent_of_what: 'X is Y% of what number',
  percent_change: 'Percentage change',
  percent_increase: 'Increase a number by X%',
  percent_decrease: 'Decrease a number by X%',
  reverse_percent: 'Reverse percentage',
  percent_difference: 'Percentage difference',
  add_percent_points: 'Add percentage points',
  subtract_percent_points: 'Subtract percentage points',
  discount: 'Discount',
  markup: 'Markup',
  profit_loss: 'Profit / Loss %',
  percent_to_fraction: 'Percent → Fraction',
  fraction_to_percent: 'Fraction → Percent',
  decimal_to_percent: 'Decimal → Percent',
  compound_percent: 'Compound percentage',
  marks_percentage: 'Marks percentage',
  cgpa_to_percent: 'CGPA → Percent',
};

async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // fall through
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
    // fall through
  }

  return false;
}

function flashButtonLabel(btn: HTMLButtonElement, newLabel: string, restoreMs = 1500) {
  const original = btn.dataset.originalLabel ?? btn.textContent ?? '';
  btn.dataset.originalLabel = original;
  btn.textContent = newLabel;
  btn.disabled = true;
  setTimeout(() => {
    btn.textContent = original;
    btn.disabled = false;
  }, restoreMs);
}

const percentageForm = document.getElementById('percentage-form') as HTMLFormElement | null;

if (percentageForm) {
  const pctOperation = document.getElementById('percentage-operation') as HTMLSelectElement;
  const pctField1Wrap = document.getElementById('pct-field1-wrap') as HTMLDivElement;
  const pctField1Label = document.getElementById('pct-field1-label') as HTMLLabelElement;
  const pctField1 = document.getElementById('pct-field1') as HTMLInputElement;

  const pctField2Wrap = document.getElementById('pct-field2-wrap') as HTMLDivElement;
  const pctField2Label = document.getElementById('pct-field2-label') as HTMLLabelElement;
  const pctField2 = document.getElementById('pct-field2') as HTMLInputElement;

  const pctField3Wrap = document.getElementById('pct-field3-wrap') as HTMLDivElement;
  const pctField3Label = document.getElementById('pct-field3-label') as HTMLLabelElement;
  const pctField3 = document.getElementById('pct-field3') as HTMLInputElement;

  const pctStatusMessage = document.getElementById('percentage-status-message') as HTMLParagraphElement;
  const pctResultBox = document.getElementById('percentage-result-box') as HTMLDivElement;
  const pctResultFormatted = document.getElementById('percentage-result-formatted') as HTMLSpanElement;
  const pctExtraChips = document.getElementById('percentage-extra-chips') as HTMLDivElement;
  const pctStepsBox = document.getElementById('percentage-steps-box') as HTMLDivElement;
  const pctStepsList = document.getElementById('percentage-steps-list') as HTMLUListElement;
  const pctCopyBtn = document.getElementById('percentage-copy-btn') as HTMLButtonElement;
  const pctCopyFullBtn = document.getElementById('percentage-copy-full-btn') as HTMLButtonElement;

  let lastCalculation: {
    operation: string;
    opLabel: string;
    inputs: { label: string; value: number }[];
    result: string;
    steps: string[];
    extras: { label: string; value: string }[];
  } | null = null;

  function applyPercentageOpConfig(op: string) {
    const cfg = PERCENTAGE_OPS[op];
    if (!cfg) return;

    pctField1Label.textContent = cfg.field1.label;
    pctField1.placeholder = cfg.field1.placeholder;
    pctField1Wrap.classList.remove('hidden');

    if (cfg.field2) {
      pctField2Label.textContent = cfg.field2.label;
      pctField2.placeholder = cfg.field2.placeholder;
      pctField2Wrap.classList.remove('hidden');
      pctField2.required = true;
    } else {
      pctField2.value = '';
      pctField2.required = false;
      pctField2Wrap.classList.add('hidden');
    }

    if (cfg.field3) {
      pctField3Label.textContent = cfg.field3.label;
      pctField3.placeholder = cfg.field3.placeholder;
      pctField3Wrap.classList.remove('hidden');
      pctField3.required = true;
    } else {
      pctField3.value = '';
      pctField3.required = false;
      pctField3Wrap.classList.add('hidden');
    }

    pctResultBox.classList.add('hidden');
    pctStatusMessage.classList.add('hidden');
    lastCalculation = null;
  }

  pctOperation.addEventListener('change', () => {
    applyPercentageOpConfig(pctOperation.value);
  });

  applyPercentageOpConfig(pctOperation.value);

  pctCopyBtn.addEventListener('click', async () => {
    const text = pctResultFormatted.textContent || '';
    if (!text) return;
    const ok = await copyToClipboard(text);
    if (ok) {
      flashButtonLabel(pctCopyBtn, '✓ Result Copied');
    } else {
      flashButtonLabel(pctCopyBtn, '⚠ Copy failed');
    }
  });

  pctCopyFullBtn.addEventListener('click', async () => {
    if (!lastCalculation) return;

    const line = '─'.repeat(44);
    const buf: string[] = [];
    buf.push(line);
    buf.push('  PERCENTAGE CALCULATION');
    buf.push(line);
    buf.push(`  Operation:    ${lastCalculation.opLabel}`);

    lastCalculation.inputs.forEach((inp, i) => {
      buf.push(`  Input ${i + 1}:      ${inp.label} = ${inp.value}`);
    });

    buf.push('');
    buf.push('  Calculation:');
    lastCalculation.steps.forEach((s, i) => {
      buf.push(`    ${i + 1}. ${s}`);
    });

    if (lastCalculation.extras.length > 0) {
      buf.push('');
      buf.push('  Additional Info:');
      lastCalculation.extras.forEach(e => {
        buf.push(`    ${e.label}: ${e.value}`);
      });
    }

    buf.push('');
    buf.push(`  Result:       ${lastCalculation.result}`);
    buf.push(line);

    const ok = await copyToClipboard(buf.join('\n'));
    if (ok) {
      flashButtonLabel(pctCopyFullBtn, '✓ Full Copied');
    } else {
      flashButtonLabel(pctCopyFullBtn, '⚠ Copy failed');
    }
  });

  percentageForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    pctStatusMessage.classList.remove('hidden', 'text-red-600', 'text-green-600');
    pctStatusMessage.classList.add('text-gray-500');
    pctStatusMessage.textContent = 'Calculating...';
    pctResultBox.classList.add('hidden');

    const op = pctOperation.value;
    const cfg = PERCENTAGE_OPS[op];

    const payload: Record<string, unknown> = { operation: op };
    const capturedInputs: { label: string; value: number }[] = [];

    if (cfg.field1) {
      const v = parseFloat(pctField1.value);
      if (isNaN(v)) {
        pctStatusMessage.textContent = 'Please enter a valid number for ' + cfg.field1.label;
        pctStatusMessage.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      payload.value1 = v;
      capturedInputs.push({ label: cfg.field1.label, value: v });
    }

    if (cfg.field2) {
      const v = parseFloat(pctField2.value);
      if (isNaN(v)) {
        pctStatusMessage.textContent = 'Please enter a valid number for ' + cfg.field2.label;
        pctStatusMessage.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      payload.value2 = v;
      capturedInputs.push({ label: cfg.field2.label, value: v });
    }

    if (cfg.field3) {
      const v = parseFloat(pctField3.value);
      if (isNaN(v)) {
        pctStatusMessage.textContent = 'Please enter a valid number for ' + cfg.field3.label;
        pctStatusMessage.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      payload.value3 = v;
      capturedInputs.push({ label: cfg.field3.label, value: v });
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/calculate-percentage'
        : 'https://utils.api.srilakshmiretail.in/calculate-percentage';

      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || `Server error: ${res.status}`);
      }

      const data = await res.json();

      pctResultFormatted.textContent = data.formatted ?? String(data.result);

      pctExtraChips.innerHTML = '';
      const capturedExtras: { label: string; value: string }[] = [];
      if (data.extra && typeof data.extra === 'object') {
        Object.entries(data.extra).forEach(([k, v]) => {
          const keyLabel = k.replace(/([A-Z])/g, ' $1').replace(/^./, s => s.toUpperCase());
          const chip = document.createElement('span');
          chip.className = 'inline-flex items-center gap-1 bg-white border border-teal-200 text-teal-800 text-xs font-medium px-3 py-1 rounded-full';
          chip.textContent = `${keyLabel}: ${v}`;
          pctExtraChips.appendChild(chip);
          capturedExtras.push({ label: keyLabel, value: String(v) });
        });
      }

      const capturedSteps: string[] = [];
      pctStepsList.innerHTML = '';
      if (Array.isArray(data.steps) && data.steps.length > 0) {
        data.steps.forEach((s: string) => {
          const li = document.createElement('li');
          li.textContent = s;
          pctStepsList.appendChild(li);
          capturedSteps.push(s);
        });
        pctStepsBox.classList.remove('hidden');
      } else {
        pctStepsBox.classList.add('hidden');
      }

      lastCalculation = {
        operation: op,
        opLabel: PERCENTAGE_OP_NAMES[op] ?? op,
        inputs: capturedInputs,
        result: data.formatted ?? String(data.result),
        steps: capturedSteps,
        extras: capturedExtras,
      };

      pctResultBox.classList.remove('hidden');
      pctStatusMessage.classList.add('hidden');

    } catch (err) {
      console.error('Percentage calculation error:', err);
      pctStatusMessage.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
      pctStatusMessage.classList.replace('text-gray-500', 'text-red-600');
      lastCalculation = null;
    }
  });
}

// ----------------------------------------------------
// EMI CALCULATOR LOGIC
// ----------------------------------------------------

function formatINR(amount: number): string {
  if (!isFinite(amount)) return '₹0';
  try {
    return new Intl.NumberFormat('en-IN', {
      style: 'currency',
      currency: 'INR',
      maximumFractionDigits: 2,
      minimumFractionDigits: 2,
    }).format(amount);
  } catch {
    return '₹' + amount.toFixed(2);
  }
}

function formatINRPlain(amount: number): string {
  if (!isFinite(amount)) return '0';
  try {
    return new Intl.NumberFormat('en-IN', {
      maximumFractionDigits: 2,
      minimumFractionDigits: 2,
    }).format(amount);
  } catch {
    return amount.toFixed(2);
  }
}

const emiForm = document.getElementById('emi-form') as HTMLFormElement | null;

if (emiForm) {
  const emiPrincipal = document.getElementById('emi-principal') as HTMLInputElement;
  const emiRate = document.getElementById('emi-rate') as HTMLInputElement;
  const emiTenure = document.getElementById('emi-tenure') as HTMLInputElement;
  const emiTenureUnit = document.getElementById('emi-tenure-unit') as HTMLSelectElement;
  const emiStatus = document.getElementById('emi-status-message') as HTMLParagraphElement;

  const emiResultBox = document.getElementById('emi-result-box') as HTMLDivElement;
  const emiResultValue = document.getElementById('emi-result-value') as HTMLSpanElement;
  const emiResultPrincipal = document.getElementById('emi-result-principal') as HTMLSpanElement;
  const emiResultInterest = document.getElementById('emi-result-interest') as HTMLSpanElement;
  const emiResultTotal = document.getElementById('emi-result-total') as HTMLSpanElement;

  const emiSplitPrincipal = document.getElementById('emi-split-bar-principal') as HTMLDivElement;
  const emiSplitInterest = document.getElementById('emi-split-bar-interest') as HTMLDivElement;
  const emiPctPrincipal = document.getElementById('emi-breakdown-principal-pct') as HTMLSpanElement;
  const emiPctInterest = document.getElementById('emi-breakdown-interest-pct') as HTMLSpanElement;

  const emiToggleBtn = document.getElementById('emi-amortization-toggle') as HTMLButtonElement;
  const emiToggleLabel = document.getElementById('emi-amortization-toggle-label') as HTMLSpanElement;
  const emiToggleCount = document.getElementById('emi-amortization-count') as HTMLSpanElement;
  const emiAmortBox = document.getElementById('emi-amortization-box') as HTMLDivElement;
  const emiAmortBody = document.getElementById('emi-amortization-body') as HTMLTableSectionElement;

  const emiCopyBtn = document.getElementById('emi-copy-btn') as HTMLButtonElement;
  const emiDownloadBtn = document.getElementById('emi-download-btn') as HTMLButtonElement;

  let lastEMI: EMISnapshot | null = null;
  let lastLoanTenure: LoanTenureSnapshot | null = null;
  let amortizationOpen = false;

  // ── EMI view mode: banker (existing) vs borrower (new) ──
  let emiMode: EmiMode = 'banker';

  const modeButtons = document.querySelectorAll<HTMLButtonElement>('.emi-mode-btn');
  const tenureWrap = document.getElementById('emi-tenure-wrap') as HTMLDivElement | null;
  const monthlyPaymentWrap = document.getElementById('emi-monthly-payment-wrap') as HTMLDivElement | null;
  const monthlyPaymentInput = document.getElementById('emi-monthly-payment') as HTMLInputElement | null;
  const resultValueLabel = document.getElementById('emi-result-value-label') as HTMLSpanElement | null;

  function setEmiMode(mode: EmiMode): void {
    emiMode = mode;

    const activeCls = ['bg-white', 'text-teal-700', 'shadow-sm'];
    const inactiveCls = ['text-gray-600', 'hover:text-teal-700'];

    modeButtons.forEach(btn => {
      const isActive = btn.dataset.mode === mode;
      btn.classList.remove(...activeCls, ...inactiveCls);
      if (isActive) btn.classList.add(...activeCls);
      else btn.classList.add(...inactiveCls);
    });

    if (mode === 'banker') {
      tenureWrap?.classList.remove('hidden');
      monthlyPaymentWrap?.classList.add('hidden');
      if (monthlyPaymentInput) monthlyPaymentInput.required = false;
      if (emiTenure) emiTenure.required = true;
      if (emiTenureUnit) emiTenureUnit.required = true;
      if (resultValueLabel) resultValueLabel.textContent = 'Monthly EMI';
    } else {
      tenureWrap?.classList.add('hidden');
      monthlyPaymentWrap?.classList.remove('hidden');
      if (emiTenure) emiTenure.required = false;
      if (emiTenureUnit) emiTenureUnit.required = false;
      if (monthlyPaymentInput) monthlyPaymentInput.required = true;
      if (resultValueLabel) resultValueLabel.textContent = 'Monthly Payment';
    }

    emiResultBox?.classList.add('hidden');
    emiStatus?.classList.add('hidden');
    lastEMI = null;
    lastLoanTenure = null;
  }

  modeButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const m = (btn.dataset.mode as EmiMode) || 'banker';
      setEmiMode(m);
    });
  });

  setEmiMode('banker');

  function setAmortizationOpen(open: boolean) {
    amortizationOpen = open;
    if (open) {
      emiAmortBox.classList.remove('hidden');
      emiToggleLabel.textContent = '▾ Amortization Schedule';
    } else {
      emiAmortBox.classList.add('hidden');
      emiToggleLabel.textContent = '▸ Amortization Schedule';
    }
  }

  emiToggleBtn.addEventListener('click', () => {
    setAmortizationOpen(!amortizationOpen);
  });

  function renderAmortization(rows: EMIAmortRow[]) {
    emiAmortBody.innerHTML = '';
    const frag = document.createDocumentFragment();

    rows.forEach(row => {
      const tr = document.createElement('tr');
      tr.className = 'hover:bg-gray-50';

      const cells: [string, string][] = [
        [String(row.month), 'text-left'],
        [formatINRPlain(row.openingBalance), 'text-right'],
        [formatINRPlain(row.principalPaid), 'text-right'],
        [formatINRPlain(row.interestPaid), 'text-right'],
        [formatINRPlain(row.closingBalance), 'text-right'],
      ];

      cells.forEach(([text, align]) => {
        const td = document.createElement('td');
        td.className = `px-3 py-2 ${align} tabular-nums text-gray-700`;
        td.textContent = text;
        tr.appendChild(td);
      });

      frag.appendChild(tr);
    });

    emiAmortBody.appendChild(frag);
    emiToggleCount.textContent = `${rows.length} rows`;
  }

  emiCopyBtn.addEventListener('click', async () => {
    if (lastLoanTenure) {
      const line = '─'.repeat(52);
      const buf: string[] = [];
      buf.push(line);
      buf.push('  LOAN TENURE CALCULATION');
      buf.push(line);
      buf.push('');
      buf.push('  INPUT');
      buf.push(`    Loan Amount:      ${formatINR(lastLoanTenure.principal)}`);
      buf.push(`    Interest Rate:    ${lastLoanTenure.annualRate}% p.a.`);
      buf.push(`    Monthly Payment:  ${formatINR(lastLoanTenure.monthlyPayment)}`);
      buf.push(`    Monthly Rate:     ${lastLoanTenure.monthlyRatePercent}%`);
      buf.push('');
      buf.push('  RESULT');
      buf.push(`    Tenure:           ${lastLoanTenure.tenureMonths} months (${lastLoanTenure.tenureYears} years)`);
      buf.push(`    Total Interest:   ${formatINR(lastLoanTenure.totalInterest)}`);
      buf.push(`    Total Payment:    ${formatINR(lastLoanTenure.totalPayment)}`);
      buf.push('');
      buf.push('  BREAKDOWN');
      buf.push(`    Principal:        ${lastLoanTenure.principalPercent.toFixed(2)}%`);
      buf.push(`    Interest:         ${lastLoanTenure.interestPercent.toFixed(2)}%`);
      buf.push('');
      buf.push(line);

      const ok = await copyToClipboard(buf.join('\n'));
      if (ok) flashButtonLabel(emiCopyBtn, '✓ Full Copied');
      else flashButtonLabel(emiCopyBtn, '⚠ Copy failed');
      return;
    }

    if (!lastEMI) return;

    const line = '─'.repeat(52);
    const buf: string[] = [];
    buf.push(line);
    buf.push('  LOAN EMI CALCULATION');
    buf.push(line);
    buf.push('');
    buf.push('  INPUT');
    buf.push(`    Loan Amount:      ${formatINR(lastEMI.principal)}`);
    buf.push(`    Interest Rate:    ${lastEMI.annualRate}% p.a.`);
    buf.push(`    Tenure:           ${lastEMI.tenureInput} ${lastEMI.tenureUnit} (${lastEMI.tenureMonths} months)`);
    buf.push(`    Monthly Rate:     ${lastEMI.monthlyRatePercent}%`);
    buf.push('');
    buf.push('  RESULT');
    buf.push(`    Monthly EMI:      ${formatINR(lastEMI.emi)}`);
    buf.push(`    Total Interest:   ${formatINR(lastEMI.totalInterest)}`);
    buf.push(`    Total Payment:    ${formatINR(lastEMI.totalPayment)}`);
    buf.push('');
    buf.push('  BREAKDOWN');
    buf.push(`    Principal:        ${lastEMI.principalPercent.toFixed(2)}%`);
    buf.push(`    Interest:         ${lastEMI.interestPercent.toFixed(2)}%`);
    buf.push('');
    buf.push(line);

    const ok = await copyToClipboard(buf.join('\n'));
    if (ok) {
      flashButtonLabel(emiCopyBtn, '✓ Full Copied');
    } else {
      flashButtonLabel(emiCopyBtn, '⚠ Copy failed');
    }
  });

  emiDownloadBtn.addEventListener('click', () => {
    if (lastLoanTenure) {
      const headers = ['Month', 'Opening Balance', 'Principal Paid', 'Interest Paid', 'Total Paid', 'Closing Balance'];
      const csvRows: string[] = [headers.join(',')];

      lastLoanTenure.amortization.forEach(r => {
        csvRows.push([
          r.month,
          r.openingBalance.toFixed(2),
          r.principalPaid.toFixed(2),
          r.interestPaid.toFixed(2),
          r.totalPaid.toFixed(2),
          r.closingBalance.toFixed(2),
        ].join(','));
      });

      const csv = csvRows.join('\n');
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.style.display = 'none';
      a.href = url;
      a.download = `loan-tenure-amortization-${lastLoanTenure.tenureMonths}months.csv`;
      document.body.appendChild(a);
      a.click();
      URL.revokeObjectURL(url);
      document.body.removeChild(a);

      flashButtonLabel(emiDownloadBtn, '✓ Downloaded');
      return;
    }

    if (!lastEMI) return;

    const headers = ['Month', 'Opening Balance', 'Principal Paid', 'Interest Paid', 'Total Paid', 'Closing Balance'];
    const csvRows: string[] = [headers.join(',')];

    lastEMI.amortization.forEach(r => {
      csvRows.push([
        r.month,
        r.openingBalance.toFixed(2),
        r.principalPaid.toFixed(2),
        r.interestPaid.toFixed(2),
        r.totalPaid.toFixed(2),
        r.closingBalance.toFixed(2),
      ].join(','));
    });

    const csv = csvRows.join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.style.display = 'none';
    a.href = url;
    a.download = `emi-amortization-${lastEMI.tenureMonths}months.csv`;
    document.body.appendChild(a);
    a.click();
    URL.revokeObjectURL(url);
    document.body.removeChild(a);

    flashButtonLabel(emiDownloadBtn, '✓ Downloaded');
  });

  emiForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    emiStatus.classList.remove('hidden', 'text-red-600', 'text-green-600');
    emiStatus.classList.add('text-gray-500');
    emiStatus.textContent = 'Calculating...';
    emiResultBox.classList.add('hidden');
    setAmortizationOpen(false);

    const principal = parseFloat(emiPrincipal.value);
    const rate = parseFloat(emiRate.value);

    if (emiMode === 'borrower') {
      const monthlyPayment = monthlyPaymentInput ? parseFloat(monthlyPaymentInput.value) : NaN;

      if (isNaN(principal) || principal <= 0) {
        emiStatus.textContent = 'Please enter a valid loan amount.';
        emiStatus.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      if (isNaN(rate) || rate < 0) {
        emiStatus.textContent = 'Please enter a valid interest rate.';
        emiStatus.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      if (isNaN(monthlyPayment) || monthlyPayment <= 0) {
        emiStatus.textContent = 'Please enter a valid monthly payment.';
        emiStatus.classList.replace('text-gray-500', 'text-red-600');
        return;
      }

      try {
        const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
        const apiUrl = isLocal
          ? 'http://localhost:8080/calculate-loan-tenure'
          : 'https://utils.api.srilakshmiretail.in/calculate-loan-tenure';

        const res = await fetch(apiUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            principal,
            annualInterestRate: rate,
            monthlyPayment,
          }),
        });

        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || `Server error: ${res.status}`);
        }

        const data = await res.json();

        emiResultValue.textContent = formatINR(data.emi);
        emiResultPrincipal.textContent = formatINR(data.principal);
        emiResultInterest.textContent = formatINR(data.totalInterest);
        emiResultTotal.textContent = formatINR(data.totalPayment);

        const pPct = data.breakdown.principalPercent;
        const iPct = data.breakdown.interestPercent;
        emiSplitPrincipal.style.width = `${pPct}%`;
        emiSplitInterest.style.width = `${iPct}%`;
        emiPctPrincipal.textContent = `${pPct.toFixed(2)}% Principal`;
        emiPctInterest.textContent = `${iPct.toFixed(2)}% Interest`;

        renderAmortization(data.amortization);

        lastLoanTenure = {
          principal: data.principal,
          annualRate: rate,
          monthlyPayment: data.emi,
          tenureMonths: data.tenureMonths,
          tenureYears: data.tenureYears,
          monthlyRatePercent: data.monthlyRatePercent,
          totalInterest: data.totalInterest,
          totalPayment: data.totalPayment,
          principalPercent: pPct,
          interestPercent: iPct,
          amortization: data.amortization,
        };
        lastEMI = null;

        emiResultBox.classList.remove('hidden');
        emiStatus.classList.add('hidden');

        if (emiToggleCount) {
          emiToggleCount.textContent = `${data.tenureMonths} months (${data.tenureYears} yrs)`;
        }

      } catch (err) {
        console.error('Loan tenure calculation error:', err);
        emiStatus.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
        emiStatus.classList.replace('text-gray-500', 'text-red-600');
        lastLoanTenure = null;
      }
      return;
    }

    const tenure = parseFloat(emiTenure?.value ?? '');
    const unit = emiTenureUnit?.value ?? '';

    if (isNaN(principal) || principal <= 0) {
      emiStatus.textContent = 'Please enter a valid loan amount.';
      emiStatus.classList.replace('text-gray-500', 'text-red-600');
      return;
    }
    if (isNaN(rate) || rate < 0) {
      emiStatus.textContent = 'Please enter a valid interest rate.';
      emiStatus.classList.replace('text-gray-500', 'text-red-600');
      return;
    }
    if (isNaN(tenure) || tenure <= 0) {
      emiStatus.textContent = 'Please enter a valid tenure.';
      emiStatus.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/calculate-emi'
        : 'https://utils.api.srilakshmiretail.in/calculate-emi';

      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          principal,
          annualInterestRate: rate,
          tenure,
          tenureUnit: unit,
        }),
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || `Server error: ${res.status}`);
      }

      const data = await res.json();

      emiResultValue.textContent = formatINR(data.emi);
      emiResultPrincipal.textContent = formatINR(data.principal);
      emiResultInterest.textContent = formatINR(data.totalInterest);
      emiResultTotal.textContent = formatINR(data.totalPayment);

      const pPct = data.breakdown.principalPercent;
      const iPct = data.breakdown.interestPercent;
      emiSplitPrincipal.style.width = `${pPct}%`;
      emiSplitInterest.style.width = `${iPct}%`;
      emiPctPrincipal.textContent = `${pPct.toFixed(2)}% Principal`;
      emiPctInterest.textContent = `${iPct.toFixed(2)}% Interest`;

      renderAmortization(data.amortization);

      lastEMI = {
        principal: data.principal,
        annualRate: rate,
        tenureMonths: data.tenureMonths,
        tenureInput: tenure,
        tenureUnit: unit,
        monthlyRatePercent: data.monthlyRatePercent,
        emi: data.emi,
        totalInterest: data.totalInterest,
        totalPayment: data.totalPayment,
        principalPercent: pPct,
        interestPercent: iPct,
        amortization: data.amortization,
      };
      lastLoanTenure = null;

      emiResultBox.classList.remove('hidden');
      emiStatus.classList.add('hidden');

    } catch (err) {
      console.error('EMI calculation error:', err);
      emiStatus.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
      emiStatus.classList.replace('text-gray-500', 'text-red-600');
      lastEMI = null;
    }
  });
}

// ----------------------------------------------------
// TAX / VAT / GST CALCULATOR LOGIC
// ----------------------------------------------------

interface TaxModeConfig {
  amount?: { label: string; placeholder: string };
  rate?: { label: string; placeholder: string };
  taxType?: boolean;
  net?: boolean;
  gross?: boolean;
  taxPaid?: boolean;
  income?: boolean;
  slabs?: boolean;
  regime?: boolean;
}

const TAX_MODES: Record<string, TaxModeConfig> = {
  add_tax: {
    amount: { label: 'Base Amount (exclusive)', placeholder: 'e.g. 1000' },
    rate: { label: 'Tax Rate (%)', placeholder: 'e.g. 18' },
  },
  remove_tax: {
    amount: { label: 'Gross Amount (inclusive)', placeholder: 'e.g. 1180' },
    rate: { label: 'Tax Rate (%)', placeholder: 'e.g. 18' },
  },
  find_rate: {
    net: true,
    gross: true,
  },
  split_gst: {
    amount: { label: 'GST-inclusive Amount', placeholder: 'e.g. 1180' },
    rate: { label: 'GST Rate (%)', placeholder: 'e.g. 18' },
    taxType: true,
  },
  reverse_gst: {
    taxPaid: true,
    rate: { label: 'GST Rate (%)', placeholder: 'e.g. 18' },
  },
  income_tax: {
    income: true,
    slabs: true,
    regime: true,
  },
};

const taxForm = document.getElementById('tax-form') as HTMLFormElement | null;

if (taxForm) {
  const taxMode = document.getElementById('tax-mode') as HTMLSelectElement;
  const taxAmountWrap = document.getElementById('tax-amount-wrap') as HTMLDivElement;
  const taxAmountLabel = document.getElementById('tax-amount-label') as HTMLLabelElement;
  const taxAmount = document.getElementById('tax-amount') as HTMLInputElement;
  const taxRateWrap = document.getElementById('tax-rate-wrap') as HTMLDivElement;
  const taxRateLabel = document.getElementById('tax-rate-label') as HTMLLabelElement;
  const taxRate = document.getElementById('tax-rate') as HTMLInputElement;
  const taxTypeWrap = document.getElementById('tax-type-wrap') as HTMLDivElement;
  const taxType = document.getElementById('tax-type') as HTMLSelectElement;
  const taxNetWrap = document.getElementById('tax-net-wrap') as HTMLDivElement;
  const taxNet = document.getElementById('tax-net') as HTMLInputElement;
  const taxGrossWrap = document.getElementById('tax-gross-wrap') as HTMLDivElement;
  const taxGross = document.getElementById('tax-gross') as HTMLInputElement;
  const taxPaidWrap = document.getElementById('tax-paid-wrap') as HTMLDivElement;
  const taxPaid = document.getElementById('tax-paid') as HTMLInputElement;
  const taxIncomeWrap = document.getElementById('tax-income-wrap') as HTMLDivElement;
  const taxIncome = document.getElementById('tax-income') as HTMLInputElement;
  const taxSlabsWrap = document.getElementById('tax-slabs-wrap') as HTMLDivElement;
  const taxSlabs = document.getElementById('tax-slabs') as HTMLTextAreaElement;
  const taxRegimeWrap = document.getElementById('tax-regime-wrap') as HTMLDivElement;
  const taxRegime = document.getElementById('tax-regime') as HTMLInputElement;

  const taxStatus = document.getElementById('tax-status-message') as HTMLParagraphElement;
  const taxResultBox = document.getElementById('tax-result-box') as HTMLDivElement;
  const taxResultFmt = document.getElementById('tax-result-formatted') as HTMLSpanElement;
  const taxResultNet = document.getElementById('tax-result-net') as HTMLSpanElement;
  const taxResultTax = document.getElementById('tax-result-tax') as HTMLSpanElement;
  const taxResultGross = document.getElementById('tax-result-gross') as HTMLSpanElement;
  const taxExtraChips = document.getElementById('tax-extra-chips') as HTMLDivElement;
  const taxStepsBox = document.getElementById('tax-steps-box') as HTMLDivElement;
  const taxStepsList = document.getElementById('tax-steps-list') as HTMLOListElement;

  function applyTaxModeConfig(mode: string) {
    const cfg = TAX_MODES[mode];
    if (!cfg) return;

    if (cfg.amount) {
      taxAmountLabel.textContent = cfg.amount.label;
      taxAmount.placeholder = cfg.amount.placeholder;
      taxAmountWrap.classList.remove('hidden');
    } else {
      taxAmount.value = '';
      taxAmountWrap.classList.add('hidden');
    }

    if (cfg.rate) {
      taxRateLabel.textContent = cfg.rate.label;
      taxRate.placeholder = cfg.rate.placeholder;
      taxRateWrap.classList.remove('hidden');
    } else {
      taxRate.value = '';
      taxRateWrap.classList.add('hidden');
    }

    const toggles: [HTMLDivElement, boolean | undefined][] = [
      [taxTypeWrap, cfg.taxType],
      [taxNetWrap, cfg.net],
      [taxGrossWrap, cfg.gross],
      [taxPaidWrap, cfg.taxPaid],
      [taxIncomeWrap, cfg.income],
      [taxSlabsWrap, cfg.slabs],
      [taxRegimeWrap, cfg.regime],
    ];
    for (const [el, show] of toggles) {
      if (show) el.classList.remove('hidden');
      else el.classList.add('hidden');
    }

    if (!cfg.net) taxNet.value = '';
    if (!cfg.gross) taxGross.value = '';
    if (!cfg.taxPaid) taxPaid.value = '';
    if (!cfg.income) taxIncome.value = '';
    if (!cfg.slabs) taxSlabs.value = '';
    if (!cfg.regime) taxRegime.value = '';

    taxResultBox.classList.add('hidden');
    taxStatus.classList.add('hidden');
  }

  taxMode.addEventListener('change', () => applyTaxModeConfig(taxMode.value));
  applyTaxModeConfig(taxMode.value);

  function parseSlabs(raw: string): { from: number; to: number; rate: number }[] {
    const out: { from: number; to: number; rate: number }[] = [];
    const lines = raw.split(/\r?\n/);
    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      const parts = trimmed.split(',').map(p => p.trim());
      if (parts.length !== 3) {
        throw new Error(`Invalid slab line: "${trimmed}" (expected "from,to,rate")`);
      }
      const from = parseFloat(parts[0]);
      const to = parseFloat(parts[1]);
      const rate = parseFloat(parts[2]);
      if (isNaN(from) || isNaN(to) || isNaN(rate)) {
        throw new Error(`Non-numeric slab line: "${trimmed}"`);
      }
      out.push({ from, to, rate });
    }
    return out;
  }

  taxForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    taxStatus.classList.remove('hidden', 'text-red-600', 'text-green-600');
    taxStatus.classList.add('text-gray-500');
    taxStatus.textContent = 'Calculating...';
    taxResultBox.classList.add('hidden');

    const mode = taxMode.value;
    const cfg = TAX_MODES[mode];
    const payload: Record<string, unknown> = { mode };

    try {
      if (cfg.amount) {
        const v = parseFloat(taxAmount.value);
        if (isNaN(v)) throw new Error('Please enter a valid amount.');
        payload.amount = v;
      }
      if (cfg.rate) {
        const v = parseFloat(taxRate.value);
        if (isNaN(v)) throw new Error('Please enter a valid tax rate.');
        payload.taxRate = v;
      }
      if (cfg.taxType) payload.taxType = taxType.value;
      if (cfg.net) {
        const v = parseFloat(taxNet.value);
        if (isNaN(v)) throw new Error('Please enter a valid net amount.');
        payload.netAmount = v;
      }
      if (cfg.gross) {
        const v = parseFloat(taxGross.value);
        if (isNaN(v)) throw new Error('Please enter a valid gross amount.');
        payload.grossAmount = v;
      }
      if (cfg.taxPaid) {
        const v = parseFloat(taxPaid.value);
        if (isNaN(v)) throw new Error('Please enter a valid tax-paid amount.');
        payload.taxPaid = v;
      }
      if (cfg.income) {
        const v = parseFloat(taxIncome.value);
        if (isNaN(v)) throw new Error('Please enter a valid income.');
        payload.income = v;
      }
      if (cfg.slabs) {
        payload.slabs = parseSlabs(taxSlabs.value);
        if ((payload.slabs as unknown[]).length === 0) {
          throw new Error('Please enter at least one slab.');
        }
      }
      if (cfg.regime && taxRegime.value.trim() !== '') {
        payload.regime = taxRegime.value.trim();
      }
    } catch (err) {
      taxStatus.textContent = err instanceof Error ? err.message : 'Invalid input.';
      taxStatus.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/calculate-tax'
        : 'https://utils.api.srilakshmiretail.in/calculate-tax';

      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || `Server error: ${res.status}`);
      }

      const data = await res.json();

      taxResultFmt.textContent = data.formatted ?? String(data.grossAmount ?? '');
      taxResultNet.textContent = formatINRPlain(data.netAmount);
      taxResultTax.textContent = formatINRPlain(data.taxAmount);
      taxResultGross.textContent = formatINRPlain(data.grossAmount);

      taxExtraChips.innerHTML = '';
      if (data.extra && typeof data.extra === 'object') {
        Object.entries(data.extra).forEach(([k, v]) => {
          if (v === null || v === undefined || Array.isArray(v) || typeof v === 'object') return;
          const keyLabel = k.replace(/([A-Z])/g, ' $1').replace(/^./, s => s.toUpperCase());
          const chip = document.createElement('span');
          chip.className = 'inline-flex items-center gap-1 bg-white border border-teal-200 text-teal-800 text-xs font-medium px-3 py-1 rounded-full';
          chip.textContent = `${keyLabel}: ${v}`;
          taxExtraChips.appendChild(chip);
        });
      }

      taxStepsList.innerHTML = '';
      if (Array.isArray(data.steps) && data.steps.length > 0) {
        data.steps.forEach((s: string) => {
          const li = document.createElement('li');
          li.textContent = s;
          taxStepsList.appendChild(li);
        });
        taxStepsBox.classList.remove('hidden');
      } else {
        taxStepsBox.classList.add('hidden');
      }

      taxResultBox.classList.remove('hidden');
      taxStatus.classList.add('hidden');

    } catch (err) {
      console.error('Tax calculation error:', err);
      taxStatus.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
      taxStatus.classList.replace('text-gray-500', 'text-red-600');
    }
  });
}

// ----------------------------------------------------
// INTEREST (SI / CI) CALCULATOR LOGIC
// ----------------------------------------------------
const interestForm = document.getElementById('interest-form') as HTMLFormElement | null;

if (interestForm) {
  const modeSimpleBtn = document.getElementById('interest-mode-simple') as HTMLButtonElement;
  const modeCompoundBtn = document.getElementById('interest-mode-compound') as HTMLButtonElement;

  const principalInput = document.getElementById('interest-principal') as HTMLInputElement;
  const rateInput = document.getElementById('interest-rate') as HTMLInputElement;
  const timeInput = document.getElementById('interest-time') as HTMLInputElement;
  const timeUnitInput = document.getElementById('interest-time-unit') as HTMLSelectElement;
  const daysOption = document.getElementById('interest-time-unit-days') as HTMLOptionElement;

  const frequencyWrap = document.getElementById('interest-frequency-wrap') as HTMLDivElement;
  const frequencyInput = document.getElementById('interest-frequency') as HTMLSelectElement;

  const statusMessage = document.getElementById('interest-status-message') as HTMLParagraphElement;
  const resultBox = document.getElementById('interest-result-box') as HTMLDivElement;

  const resultTotal = document.getElementById('interest-result-total') as HTMLSpanElement;
  const resultPrincipal = document.getElementById('interest-result-principal') as HTMLSpanElement;
  const resultInterest = document.getElementById('interest-result-interest') as HTMLSpanElement;

  const splitPrincipal = document.getElementById('interest-split-bar-principal') as HTMLDivElement;
  const splitInterest = document.getElementById('interest-split-bar-interest') as HTMLDivElement;
  const pctPrincipal = document.getElementById('interest-breakdown-principal-pct') as HTMLSpanElement;
  const pctInterest = document.getElementById('interest-breakdown-interest-pct') as HTMLSpanElement;

  const stepsToggleBtn = document.getElementById('interest-steps-toggle') as HTMLButtonElement;
  const stepsToggleLbl = document.getElementById('interest-steps-toggle-label') as HTMLSpanElement;
  const stepsBox = document.getElementById('interest-steps-box') as HTMLDivElement;
  const stepsList = document.getElementById('interest-steps-list') as HTMLOListElement;

  const yearlyWrap = document.getElementById('interest-yearly-wrap') as HTMLDivElement;
  const yearlyToggleBtn = document.getElementById('interest-yearly-toggle') as HTMLButtonElement;
  const yearlyToggleLbl = document.getElementById('interest-yearly-toggle-label') as HTMLSpanElement;
  const yearlyCount = document.getElementById('interest-yearly-count') as HTMLSpanElement;
  const yearlyBox = document.getElementById('interest-yearly-box') as HTMLDivElement;
  const yearlyBody = document.getElementById('interest-yearly-body') as HTMLTableSectionElement;

  let currentMode: 'simple' | 'compound' = 'simple';
  let stepsOpen = false;
  let yearlyOpen = false;

  function setMode(mode: 'simple' | 'compound') {
    currentMode = mode;

    const activeCls = ['bg-white', 'text-teal-700', 'shadow-sm'];
    const inactiveCls = ['text-gray-600', 'hover:text-teal-700'];

    for (const btn of [modeSimpleBtn, modeCompoundBtn]) {
      btn.classList.remove(...activeCls, ...inactiveCls);
    }

    if (mode === 'simple') {
      modeSimpleBtn.classList.add(...activeCls);
      modeCompoundBtn.classList.add(...inactiveCls);
    } else {
      modeCompoundBtn.classList.add(...activeCls);
      modeSimpleBtn.classList.add(...inactiveCls);
    }

    if (mode === 'compound') {
      frequencyWrap.classList.remove('hidden');
    } else {
      frequencyWrap.classList.add('hidden');
    }

    if (mode === 'compound') {
      daysOption.disabled = true;
      if (timeUnitInput.value === 'days') {
        timeUnitInput.value = 'years';
      }
    } else {
      daysOption.disabled = false;
    }

    resultBox.classList.add('hidden');
    statusMessage.classList.add('hidden');

    setStepsOpen(false);
    setYearlyOpen(false);
  }

  modeSimpleBtn.addEventListener('click', () => setMode('simple'));
  modeCompoundBtn.addEventListener('click', () => setMode('compound'));

  function setStepsOpen(open: boolean) {
    stepsOpen = open;
    if (open) {
      stepsBox.classList.remove('hidden');
      stepsToggleLbl.textContent = '▾ Calculation Steps';
    } else {
      stepsBox.classList.add('hidden');
      stepsToggleLbl.textContent = '▸ Calculation Steps';
    }
  }

  function setYearlyOpen(open: boolean) {
    yearlyOpen = open;
    if (open) {
      yearlyBox.classList.remove('hidden');
      yearlyToggleLbl.textContent = '▾ Yearly Breakdown';
    } else {
      yearlyBox.classList.add('hidden');
      yearlyToggleLbl.textContent = '▸ Yearly Breakdown';
    }
  }

  stepsToggleBtn.addEventListener('click', () => setStepsOpen(!stepsOpen));
  yearlyToggleBtn.addEventListener('click', () => setYearlyOpen(!yearlyOpen));

  function formatINR2(amount: number): string {
    if (!isFinite(amount)) return '₹0';
    try {
      return new Intl.NumberFormat('en-IN', {
        style: 'currency',
        currency: 'INR',
        maximumFractionDigits: 2,
        minimumFractionDigits: 2,
      }).format(amount);
    } catch {
      return '₹' + amount.toFixed(2);
    }
  }

  function formatINRPlain2(amount: number): string {
    if (!isFinite(amount)) return '0';
    try {
      return new Intl.NumberFormat('en-IN', {
        maximumFractionDigits: 2,
        minimumFractionDigits: 2,
      }).format(amount);
    } catch {
      return amount.toFixed(2);
    }
  }

  interestForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    statusMessage.classList.remove('hidden', 'text-red-600', 'text-green-600');
    statusMessage.classList.add('text-gray-500');
    statusMessage.textContent = 'Calculating...';
    resultBox.classList.add('hidden');
    setStepsOpen(false);
    setYearlyOpen(false);

    const principal = parseFloat(principalInput.value);
    const rate = parseFloat(rateInput.value);
    const time = parseFloat(timeInput.value);
    const timeUnit = timeUnitInput.value;

    if (isNaN(principal) || principal <= 0) {
      statusMessage.textContent = 'Please enter a valid principal amount.';
      statusMessage.classList.replace('text-gray-500', 'text-red-600');
      return;
    }
    if (isNaN(rate) || rate < 0 || rate > 100) {
      statusMessage.textContent = 'Please enter a valid interest rate (0–100).';
      statusMessage.classList.replace('text-gray-500', 'text-red-600');
      return;
    }
    if (isNaN(time) || time <= 0) {
      statusMessage.textContent = 'Please enter a valid time.';
      statusMessage.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    let payload: Record<string, unknown>;
    let apiPath: string;

    if (currentMode === 'simple') {
      payload = {
        principal,
        annualInterestRate: rate,
        time,
        timeUnit,
      };
      apiPath = '/calculate-simple-interest';
    } else {
      payload = {
        principal,
        annualInterestRate: rate,
        time,
        timeUnit,
        compoundingFrequency: frequencyInput.value,
      };
      apiPath = '/calculate-compound-interest';
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? `http://localhost:8080${apiPath}`
        : `https://utils.api.srilakshmiretail.in${apiPath}`;

      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || `Server error: ${res.status}`);
      }

      const data = await res.json();

      resultTotal.textContent = formatINR2(data.totalAmount);

      resultPrincipal.textContent = formatINR2(data.principal);
      resultInterest.textContent = formatINR2(data.interest);

      const pPct = data.breakdown.principalPercent;
      const iPct = data.breakdown.interestPercent;
      splitPrincipal.style.width = `${pPct}%`;
      splitInterest.style.width = `${iPct}%`;
      pctPrincipal.textContent = `${pPct.toFixed(2)}% Principal`;
      pctInterest.textContent = `${iPct.toFixed(2)}% Interest`;

      stepsList.innerHTML = '';
      const steps: string[] = Array.isArray(data.steps) ? data.steps : [];
      if (steps.length > 0) {
        for (const s of steps) {
          const li = document.createElement('li');
          li.textContent = s;
          stepsList.appendChild(li);
        }
        stepsToggleBtn.classList.remove('hidden');
      } else {
        stepsToggleBtn.classList.add('hidden');
      }

      const yearlyRows: { year: number; openingBalance: number; interestEarned: number; closingBalance: number }[] =
        Array.isArray(data.yearlyBreakdown) ? data.yearlyBreakdown : [];

      if (currentMode === 'compound' && yearlyRows.length > 0) {
        yearlyBody.innerHTML = '';
        const frag = document.createDocumentFragment();
        for (const row of yearlyRows) {
          const tr = document.createElement('tr');
          tr.className = 'hover:bg-gray-50';

          const cells: [string, string][] = [
            [String(row.year), 'text-left'],
            [formatINRPlain2(row.openingBalance), 'text-right'],
            [formatINRPlain2(row.interestEarned), 'text-right'],
            [formatINRPlain2(row.closingBalance), 'text-right'],
          ];
          for (const [text, align] of cells) {
            const td = document.createElement('td');
            td.className = `px-3 py-2 ${align} tabular-nums text-gray-700`;
            td.textContent = text;
            tr.appendChild(td);
          }
          frag.appendChild(tr);
        }
        yearlyBody.appendChild(frag);
        yearlyCount.textContent = `${yearlyRows.length} rows`;
        yearlyWrap.classList.remove('hidden');
      } else {
        yearlyWrap.classList.add('hidden');
      }

      resultBox.classList.remove('hidden');
      statusMessage.classList.add('hidden');

    } catch (err) {
      console.error('Interest calculation error:', err);
      statusMessage.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
      statusMessage.classList.replace('text-gray-500', 'text-red-600');
    }
  });

  setMode('simple');
}

// ----------------------------------------------------
// SCIENTIFIC CALCULATOR LOGIC
// ----------------------------------------------------
const scientificForm = document.getElementById('scientific-form') as HTMLFormElement | null;

if (scientificForm) {
  const sciOperation = document.getElementById('scientific-operation') as HTMLSelectElement;

  const sciAngleWrap = document.getElementById('scientific-angle-wrap') as HTMLDivElement;
  const sciAngleDeg = document.getElementById('scientific-angle-degrees') as HTMLButtonElement;
  const sciAngleRad = document.getElementById('scientific-angle-radians') as HTMLButtonElement;

  const sciV1Wrap = document.getElementById('scientific-value1-wrap') as HTMLDivElement;
  const sciV1Label = document.getElementById('scientific-value1-label') as HTMLLabelElement;
  const sciV1 = document.getElementById('scientific-value1') as HTMLInputElement;

  const sciV2Wrap = document.getElementById('scientific-value2-wrap') as HTMLDivElement;
  const sciV2Label = document.getElementById('scientific-value2-label') as HTMLLabelElement;
  const sciV2 = document.getElementById('scientific-value2') as HTMLInputElement;

  const sciStatus = document.getElementById('scientific-status-message') as HTMLParagraphElement;
  const sciResultBox = document.getElementById('scientific-result-box') as HTMLDivElement;
  const sciResultFmt = document.getElementById('scientific-result-formatted') as HTMLSpanElement;
  const sciStepsBox = document.getElementById('scientific-steps-box') as HTMLDivElement;
  const sciStepsList = document.getElementById('scientific-steps-list') as HTMLOListElement;
  const sciCopyBtn = document.getElementById('scientific-copy-btn') as HTMLButtonElement;

  interface SciFieldConfig { label: string; placeholder: string; }
  interface SciOpConfig {
    value1?: SciFieldConfig;
    value2?: SciFieldConfig;
    showAngleUnit?: boolean;
  }

  const SCIENTIFIC_OPS: Record<string, SciOpConfig> = {
    pi: {},
    e: {},

    sin: { value1: { label: 'Angle', placeholder: 'e.g. 30' }, showAngleUnit: true },
    cos: { value1: { label: 'Angle', placeholder: 'e.g. 60' }, showAngleUnit: true },
    tan: { value1: { label: 'Angle', placeholder: 'e.g. 45' }, showAngleUnit: true },
    csc: { value1: { label: 'Angle', placeholder: 'e.g. 30' }, showAngleUnit: true },
    sec: { value1: { label: 'Angle', placeholder: 'e.g. 60' }, showAngleUnit: true },
    cot: { value1: { label: 'Angle', placeholder: 'e.g. 45' }, showAngleUnit: true },

    asin: { value1: { label: 'Ratio ([-1, 1])', placeholder: 'e.g. 0.5' }, showAngleUnit: true },
    acos: { value1: { label: 'Ratio ([-1, 1])', placeholder: 'e.g. 0.5' }, showAngleUnit: true },
    atan: { value1: { label: 'Ratio', placeholder: 'e.g. 1' }, showAngleUnit: true },

    atan2: {
      value1: { label: 'Y', placeholder: 'e.g. 1' },
      value2: { label: 'X', placeholder: 'e.g. 1' },
      showAngleUnit: true,
    },

    sinh: { value1: { label: 'Value', placeholder: 'e.g. 1' } },
    cosh: { value1: { label: 'Value', placeholder: 'e.g. 0' } },
    tanh: { value1: { label: 'Value', placeholder: 'e.g. 0' } },
    asinh: { value1: { label: 'Value', placeholder: 'e.g. 0' } },
    acosh: { value1: { label: 'Value (≥ 1)', placeholder: 'e.g. 1' } },
    atanh: { value1: { label: 'Value ((-1, 1))', placeholder: 'e.g. 0' } },

    log: { value1: { label: 'Number (> 0)', placeholder: 'e.g. 1000' } },
    ln: { value1: { label: 'Number (> 0)', placeholder: 'e.g. 2.718' } },
    log_base: {
      value1: { label: 'Argument (> 0)', placeholder: 'e.g. 1000' },
      value2: { label: 'Base (> 0, ≠ 1)', placeholder: 'e.g. 10' },
    },
    exp: { value1: { label: 'Exponent', placeholder: 'e.g. 1' } },

    pow: {
      value1: { label: 'Base', placeholder: 'e.g. 2' },
      value2: { label: 'Exponent', placeholder: 'e.g. 10' },
    },
    sqrt: { value1: { label: 'Number (≥ 0)', placeholder: 'e.g. 144' } },
    cbrt: { value1: { label: 'Number', placeholder: 'e.g. 27' } },
    nth_root: {
      value1: { label: 'Number', placeholder: 'e.g. 16' },
      value2: { label: 'N (root degree)', placeholder: 'e.g. 4' },
    },

    abs: { value1: { label: 'Number', placeholder: 'e.g. -7.5' } },
    floor: { value1: { label: 'Number', placeholder: 'e.g. 3.7' } },
    ceil: { value1: { label: 'Number', placeholder: 'e.g. 3.2' } },
    round: { value1: { label: 'Number', placeholder: 'e.g. 3.5' } },
    trunc: { value1: { label: 'Number', placeholder: 'e.g. -3.9' } },
    sign: { value1: { label: 'Number', placeholder: 'e.g. -5' } },

    factorial: { value1: { label: 'N (non-negative integer, ≤ 170)', placeholder: 'e.g. 5' } },
    ncr: {
      value1: { label: 'N (total items)', placeholder: 'e.g. 5' },
      value2: { label: 'R (chosen)', placeholder: 'e.g. 2' },
    },
    npr: {
      value1: { label: 'N (total items)', placeholder: 'e.g. 5' },
      value2: { label: 'R (chosen)', placeholder: 'e.g. 2' },
    },

    gcd: {
      value1: { label: 'First integer', placeholder: 'e.g. 48' },
      value2: { label: 'Second integer', placeholder: 'e.g. 18' },
    },
    lcm: {
      value1: { label: 'First integer', placeholder: 'e.g. 4' },
      value2: { label: 'Second integer', placeholder: 'e.g. 6' },
    },
    mod: {
      value1: { label: 'Dividend', placeholder: 'e.g. 10' },
      value2: { label: 'Divisor (≠ 0)', placeholder: 'e.g. 3' },
    },
    hypot: {
      value1: { label: 'Side A', placeholder: 'e.g. 3' },
      value2: { label: 'Side B', placeholder: 'e.g. 4' },
    },
  };

  let angleUnit: 'degrees' | 'radians' = 'degrees';

  function renderAngleUnitButtons(): void {
    const activeCls = ['bg-white', 'text-violet-700', 'shadow-sm'];
    const inactiveCls = ['text-gray-600', 'hover:text-violet-700'];

    for (const btn of [sciAngleDeg, sciAngleRad]) {
      btn.classList.remove(...activeCls, ...inactiveCls);
    }
    if (angleUnit === 'degrees') {
      sciAngleDeg.classList.add(...activeCls);
      sciAngleRad.classList.add(...inactiveCls);
    } else {
      sciAngleRad.classList.add(...activeCls);
      sciAngleDeg.classList.add(...inactiveCls);
    }
  }

  sciAngleDeg.addEventListener('click', () => {
    angleUnit = 'degrees';
    renderAngleUnitButtons();
  });
  sciAngleRad.addEventListener('click', () => {
    angleUnit = 'radians';
    renderAngleUnitButtons();
  });

  function applyScientificOpConfig(op: string): void {
    const cfg = SCIENTIFIC_OPS[op];
    if (!cfg) return;

    if (cfg.value1) {
      sciV1Label.textContent = cfg.value1.label;
      sciV1.placeholder = cfg.value1.placeholder;
      sciV1Wrap.classList.remove('hidden');
      sciV1.required = true;
    } else {
      sciV1.value = '';
      sciV1.required = false;
      sciV1Wrap.classList.add('hidden');
    }

    if (cfg.value2) {
      sciV2Label.textContent = cfg.value2.label;
      sciV2.placeholder = cfg.value2.placeholder;
      sciV2Wrap.classList.remove('hidden');
      sciV2.required = true;
    } else {
      sciV2.value = '';
      sciV2.required = false;
      sciV2Wrap.classList.add('hidden');
    }

    if (cfg.showAngleUnit) {
      sciAngleWrap.classList.remove('hidden');
    } else {
      sciAngleWrap.classList.add('hidden');
    }

    sciResultBox.classList.add('hidden');
    sciStatus.classList.add('hidden');
    sciStepsBox.classList.add('hidden');
  }

  sciOperation.addEventListener('change', () => {
    applyScientificOpConfig(sciOperation.value);
  });

  sciCopyBtn.addEventListener('click', async () => {
    const text = sciResultFmt.textContent || '';
    if (!text) return;
    const ok = await copyToClipboard(text);
    if (ok) {
      flashButtonLabel(sciCopyBtn, '✓ Copied');
    } else {
      flashButtonLabel(sciCopyBtn, '⚠ Failed');
    }
  });

  scientificForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    sciStatus.classList.remove('hidden', 'text-red-600', 'text-green-600');
    sciStatus.classList.add('text-gray-500');
    sciStatus.textContent = 'Calculating...';
    sciResultBox.classList.add('hidden');
    sciStepsBox.classList.add('hidden');

    const op = sciOperation.value;
    const cfg = SCIENTIFIC_OPS[op];

    const payload: Record<string, unknown> = { operation: op };

    if (cfg.value1) {
      const v = parseFloat(sciV1.value);
      if (isNaN(v)) {
        sciStatus.textContent = `Please enter a valid number for ${cfg.value1.label}.`;
        sciStatus.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      payload.value1 = v;
    }

    if (cfg.value2) {
      const v = parseFloat(sciV2.value);
      if (isNaN(v)) {
        sciStatus.textContent = `Please enter a valid number for ${cfg.value2.label}.`;
        sciStatus.classList.replace('text-gray-500', 'text-red-600');
        return;
      }
      payload.value2 = v;
    }

    if (cfg.showAngleUnit) {
      payload.angleUnit = angleUnit;
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/calculate-scientific'
        : 'https://utils.api.srilakshmiretail.in/calculate-scientific';

      const res = await fetch(apiUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || `Server error: ${res.status}`);
      }

      const data = await res.json();

      sciResultFmt.textContent = data.formatted ?? String(data.result);

      sciStepsList.innerHTML = '';
      const steps: string[] = Array.isArray(data.steps) ? data.steps : [];
      if (steps.length > 0) {
        for (const s of steps) {
          const li = document.createElement('li');
          li.textContent = s;
          sciStepsList.appendChild(li);
        }
        sciStepsBox.classList.remove('hidden');
      } else {
        sciStepsBox.classList.add('hidden');
      }

      sciResultBox.classList.remove('hidden');
      sciStatus.classList.add('hidden');
    } catch (err) {
      console.error('Scientific calculation error:', err);
      sciStatus.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error occurred'}`;
      sciStatus.classList.replace('text-gray-500', 'text-red-600');
    }
  });

  renderAngleUnitButtons();
  applyScientificOpConfig(sciOperation.value);
}

// ----------------------------------------------------
// FILE REPAIR / FORMATTER LOGIC
// ----------------------------------------------------
const repairForm = document.getElementById('repair-form') as HTMLFormElement | null;

if (repairForm) {
  const repairFileInput = document.getElementById('repair-file-input') as HTMLInputElement;
  const repairLevel = document.getElementById('repair-level') as HTMLSelectElement;
  const repairDescribe = document.getElementById('repair-describe') as HTMLInputElement;

  const repairDetectionBox = document.getElementById('repair-detection-box') as HTMLDivElement;
  const repairDetectedFormat = document.getElementById('repair-detected-format-text') as HTMLSpanElement;

  const repairStatus = document.getElementById('repair-status-message') as HTMLParagraphElement;
  const repairReportBox = document.getElementById('repair-report-box') as HTMLDivElement;
  const repairResultFormat = document.getElementById('repair-result-format') as HTMLSpanElement;
  const repairResultLevel = document.getElementById('repair-result-level') as HTMLSpanElement;
  const repairResultChanged = document.getElementById('repair-result-changed') as HTMLSpanElement;
  const repairFixesChips = document.getElementById('repair-fixes-chips') as HTMLDivElement;
  const repairWarningsWrap = document.getElementById('repair-warnings-wrap') as HTMLDivElement;
  const repairWarningsList = document.getElementById('repair-warnings-list') as HTMLUListElement;

  let detectedRepairFormat = '';

  repairFileInput.addEventListener('change', () => {
    if (repairFileInput.files && repairFileInput.files.length > 0) {
      const file = repairFileInput.files[0];
      const ext = file.name.split('.').pop()?.toLowerCase();

      if (ext) {
        detectedRepairFormat = ext;
        repairDetectedFormat.textContent = ext.toUpperCase() + ' (Detected)';
        repairDetectionBox.classList.replace('bg-gray-100', 'bg-amber-50');
        repairDetectionBox.classList.replace('border-gray-200', 'border-amber-200');
        repairDetectedFormat.classList.replace('text-gray-500', 'text-amber-700');
      } else {
        resetRepairDetectionBox();
      }
    } else {
      resetRepairDetectionBox();
    }
  });

  function resetRepairDetectionBox() {
    detectedRepairFormat = '';
    repairDetectedFormat.textContent = 'Auto-detected on upload';
    repairDetectionBox.classList.replace('bg-amber-50', 'bg-gray-100');
    repairDetectionBox.classList.replace('border-amber-200', 'border-gray-200');
    repairDetectedFormat.classList.replace('text-amber-700', 'text-gray-500');
  }

  repairForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    repairStatus.classList.remove('hidden', 'text-red-600', 'text-green-600');
    repairStatus.classList.add('text-gray-500');
    repairReportBox.classList.add('hidden');

    if (!repairFileInput.files || repairFileInput.files.length === 0) {
      repairStatus.textContent = 'Please select a file to repair.';
      repairStatus.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    const file = repairFileInput.files[0];
    const formData = new FormData();
    formData.append('file', file);
    formData.append('repairLevel', repairLevel.value);
    if (repairDescribe.checked) {
      formData.append('describe', 'true');
    }

    try {
      repairStatus.textContent = 'Repairing file...';

      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/repair'
        : 'https://utils.api.srilakshmiretail.in/repair';

      const response = await fetch(apiUrl, {
        method: 'POST',
        body: formData,
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(errText || `Server error: ${response.status}`);
      }

      const contentType = response.headers.get('content-type') || '';

      if (contentType.includes('application/json')) {
        const json = await response.json();

        repairResultFormat.textContent = json.format ?? '—';
        repairResultLevel.textContent = json.level ?? '—';
        repairResultChanged.textContent = json.changed ? 'Yes' : 'No';

        repairFixesChips.innerHTML = '';
        if (Array.isArray(json.applied) && json.applied.length > 0) {
          json.applied.forEach((fix: string) => {
            const chip = document.createElement('span');
            chip.className = 'inline-flex items-center gap-1 bg-white border border-amber-200 text-amber-800 text-xs font-medium px-3 py-1 rounded-full';
            chip.textContent = fix;
            repairFixesChips.appendChild(chip);
          });
        } else {
          const chip = document.createElement('span');
          chip.className = 'inline-flex items-center gap-1 bg-white border border-gray-200 text-gray-500 text-xs font-medium px-3 py-1 rounded-full';
          chip.textContent = 'No fixes applied';
          repairFixesChips.appendChild(chip);
        }

        repairWarningsList.innerHTML = '';
        if (Array.isArray(json.warnings) && json.warnings.length > 0) {
          json.warnings.forEach((w: string) => {
            const li = document.createElement('li');
            li.textContent = w;
            repairWarningsList.appendChild(li);
          });
          repairWarningsWrap.classList.remove('hidden');
        } else {
          repairWarningsWrap.classList.add('hidden');
        }

        repairReportBox.classList.remove('hidden');

        const repairedContent = json.output ?? '';
        const outExt = detectedRepairFormat || json.format || 'txt';
        const blob = new Blob([repairedContent], { type: 'text/plain;charset=utf-8;' });
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.style.display = 'none';
        a.href = url;
        a.download = `repaired.${outExt}`;
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        document.body.removeChild(a);

        repairStatus.textContent = json.changed
          ? 'Repair complete! File downloaded.'
          : 'File was already valid — downloaded unchanged.';
        repairStatus.classList.replace('text-gray-500', 'text-green-600');

      } else {
        const blob = await response.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.style.display = 'none';
        a.href = url;
        a.download = `repaired.${detectedRepairFormat || 'txt'}`;
        document.body.appendChild(a);
        a.click();
        window.URL.revokeObjectURL(url);
        document.body.removeChild(a);

        repairResultFormat.textContent = response.headers.get('X-Repair-Format') ?? '—';
        repairResultLevel.textContent = response.headers.get('X-Repair-Level') ?? '—';
        repairResultChanged.textContent = response.headers.get('X-Repair-Changed') === 'true' ? 'Yes' : 'No';

        const applied = response.headers.get('X-Repair-Applied') || '';
        repairFixesChips.innerHTML = '';
        if (applied && applied !== 'none') {
          applied.split(';').forEach(part => {
            part.split(',').forEach(fix => {
              const trimmed = fix.trim();
              if (!trimmed) return;
              if (trimmed.startsWith('level=') ||
                trimmed.startsWith('changed=') ||
                trimmed.startsWith('fixed=')) return;
              const chip = document.createElement('span');
              chip.className = 'inline-flex items-center gap-1 bg-white border border-amber-200 text-amber-800 text-xs font-medium px-3 py-1 rounded-full';
              chip.textContent = trimmed;
              repairFixesChips.appendChild(chip);
            });
          });
        }
        if (repairFixesChips.children.length === 0) {
          const chip = document.createElement('span');
          chip.className = 'inline-flex items-center gap-1 bg-white border border-gray-200 text-gray-500 text-xs font-medium px-3 py-1 rounded-full';
          chip.textContent = 'No fixes applied';
          repairFixesChips.appendChild(chip);
        }

        repairWarningsWrap.classList.add('hidden');
        repairReportBox.classList.remove('hidden');
        repairStatus.textContent = 'Repair complete! File downloaded.';
        repairStatus.classList.replace('text-gray-500', 'text-green-600');
      }

    } catch (error) {
      console.error('Repair error:', error);
      repairStatus.textContent = `Error: ${error instanceof Error ? error.message : 'Unknown error occurred'}`;
      repairStatus.classList.replace('text-gray-500', 'text-red-600');
    }
  });
}

// IMAGE COMPRESS LOGIC (single authoritative handler)
// ----------------------------------------------------
const imageForm = document.getElementById('image-compress-form') as HTMLFormElement | null;

if (imageForm) {
  const imageInput = document.getElementById('image-input') as HTMLInputElement;
  const imageDetected = document.getElementById('image-detected') as HTMLParagraphElement;
  const imageConversionType = document.getElementById('image-conversion-type') as HTMLSelectElement;
  const imageDataType = document.getElementById('image-data-type') as HTMLSelectElement;
  const imageTargetValue = document.getElementById('image-target-value') as HTMLInputElement;
  const imageSizeUnitWrap = document.getElementById('image-sizeunit-wrap') as HTMLDivElement;
  const imageSizeUnit = document.getElementById('image-size-unit') as HTMLSelectElement;
  const imageTargetFormat = document.getElementById('image-target-format') as HTMLSelectElement;
  const imageMaxDimension = document.getElementById('image-max-dimension') as HTMLInputElement;
  const imageTargetHint = document.getElementById('image-target-hint') as HTMLSpanElement;

  const imageStatus = document.getElementById('image-status-message') as HTMLParagraphElement;
  const imageResultBox = document.getElementById('image-result-box') as HTMLDivElement;
  const imageReportTarget = document.getElementById('image-report-target') as HTMLSpanElement;
  const imageReportActual = document.getElementById('image-report-actual') as HTMLSpanElement;
  const imageReportFormat = document.getElementById('image-report-format') as HTMLSpanElement;
  const imageReportDims = document.getElementById('image-report-dims') as HTMLSpanElement;

  // -- Auto-detect format on file selection --
  imageInput.addEventListener('change', () => {
    if (imageInput.files && imageInput.files.length > 0) {
      const f = imageInput.files[0];
      const ext = f.name.split('.').pop()?.toLowerCase() || '';
      const norm = ext === 'jpeg' ? 'jpg' : ext === 'tif' ? 'tiff' : ext;
      imageDetected.textContent = `${norm.toUpperCase()} • ${(f.size / 1024).toFixed(1)} KB`;
    } else {
      imageDetected.textContent = 'Auto-detected on upload';
    }
  });

  // -- Show/hide sizeUnit based on dataType --
  function refreshImageFormVisibility() {
    const isSize = imageDataType.value === 'size';
    imageSizeUnitWrap.classList.toggle('hidden', !isSize);

    const ct = imageConversionType.value;
    if (isSize) {
      imageTargetHint.textContent = '(1 KB – 20 MB)';
      imageTargetValue.placeholder = 'e.g. 200';
    } else if (ct === 'expand') {
      imageTargetHint.textContent = '(101–1000)';
      imageTargetValue.placeholder = 'e.g. 150';
    } else {
      imageTargetHint.textContent = '(1–99 for compress)';
      imageTargetValue.placeholder = 'e.g. 50';
    }
  }
  imageDataType.addEventListener('change', refreshImageFormVisibility);
  imageConversionType.addEventListener('change', refreshImageFormVisibility);
  refreshImageFormVisibility();

  // -- Submit handler --
  imageForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    imageStatus.classList.remove('hidden', 'text-red-600', 'text-green-600', 'text-amber-600');
    imageStatus.classList.add('text-gray-500');
    imageStatus.textContent = 'Compressing...';
    imageResultBox.classList.add('hidden');

    if (!imageInput.files || imageInput.files.length === 0) {
      imageStatus.textContent = 'Please select an image file first.';
      imageStatus.classList.replace('text-gray-500', 'text-red-600');
      return;
    }

    const formData = new FormData();
    formData.append('file', imageInput.files[0]);
    formData.append('conversionType', imageConversionType.value);
    formData.append('dataType', imageDataType.value);
    formData.append('targetValue', imageTargetValue.value);
    if (imageDataType.value === 'size') {
      formData.append('sizeUnit', imageSizeUnit.value);
    }
    formData.append('targetFormat', imageTargetFormat.value);
    if (imageMaxDimension.value.trim() !== '') {
      formData.append('maxDimension', imageMaxDimension.value);
    }

    try {
      const isLocal = window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';
      const apiUrl = isLocal
        ? 'http://localhost:8080/compress-image'
        : 'https://utils.api.srilakshmiretail.in/compress-image';

      const res = await fetch(apiUrl, { method: 'POST', body: formData });

      if (!res.ok) {
        const errText = await res.text();
        let msg = errText || `Server error: ${res.status}`;
        try {
          const parsed = JSON.parse(errText);
          if (parsed && parsed.error) msg = parsed.error;
        } catch { /* not JSON, use raw text */ }
        throw new Error(msg);
      }

      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.style.display = 'none';
      a.href = url;
      a.download = `compressed.${res.headers.get('X-Compress-Format') || 'jpg'}`;
      document.body.appendChild(a);
      a.click();
      URL.revokeObjectURL(url);
      document.body.removeChild(a);

      const targetBytes = parseInt(res.headers.get('X-Compress-Target-Size') || '0', 10);
      const actualBytes = parseInt(res.headers.get('X-Compress-Actual-Size') || '0', 10);
      const targetMet = res.headers.get('X-Compress-Target-Met') === 'true';

      imageReportTarget.textContent = `${(targetBytes / 1024).toFixed(1)} KB`;
      imageReportActual.textContent = `${(actualBytes / 1024).toFixed(1)} KB`;
      imageReportFormat.textContent = (res.headers.get('X-Compress-Format') || '—').toUpperCase();
      imageReportDims.textContent = res.headers.get('X-Compress-Dimensions') || '—';
      imageResultBox.classList.remove('hidden');

      if (targetMet) {
        imageStatus.textContent = 'Success! Target reached — file downloaded.';
        imageStatus.classList.replace('text-gray-500', 'text-green-600');
      } else {
        imageStatus.textContent = 'Best-effort result — target unreachable, smallest achievable size returned.';
        imageStatus.classList.replace('text-gray-500', 'text-amber-600');
      }
    } catch (err) {
      imageStatus.textContent = `Error: ${err instanceof Error ? err.message : 'Unknown error'}`;
      imageStatus.classList.replace('text-gray-500', 'text-red-600');
    }
  });
}