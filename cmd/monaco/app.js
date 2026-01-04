const defaultCode = `// tsgo TypeScript Playground
// Loading sample...

export default {};
`;

const STORAGE_KEY = 'tsgo-playground-code';
const CONTEXT_KEY = 'tsgo-playground-context';
const SAMPLE_KEY = 'tsgo-playground-sample';
const DEFAULT_SAMPLE = 'hello-world';

// Current context code (loaded from .ctx.ts file)
let currentContextCode = '';
let currentSampleId = '';

// Strip @ts-nocheck and triple-slash references from code for Monaco display
function stripTripleSlashRefs(code) {
  return code
    .replace(/^\/\/ @ts-nocheck.*\n?/gm, '')
    .replace(/^\/\/\/\s*<reference.*\/>\s*\n?/gm, '')
    .trim() + '\n';
}

// Sample metadata with recommended engines
const SAMPLE_ENGINES = {
  'async-fetch': 'bun',
  'parallel-tasks': 'bun',
  'task-scheduler': 'bun',
  'bun-native': 'bun',
};

// GOJA unsupported features patterns
const GOJA_UNSUPPORTED_PATTERNS = [
  { pattern: /\basync\s+function\b/g, message: 'Async functions are not supported by GOJA engine' },
  { pattern: /\basync\s*\(/g, message: 'Async arrow functions are not supported by GOJA engine' },
  { pattern: /\basync\s*\w+\s*=>/g, message: 'Async arrow functions are not supported by GOJA engine' },
  { pattern: /\bawait\s+/g, message: 'Await is not supported by GOJA engine' },
  { pattern: /\bfetch\s*\(/g, message: 'fetch() is not available in GOJA engine' },
  { pattern: /\bnew\s+WebSocket\s*\(/g, message: 'WebSocket is not available in GOJA engine' },
  { pattern: /\bsetTimeout\s*\(/g, message: 'setTimeout() is not available in GOJA engine' },
  { pattern: /\bsetInterval\s*\(/g, message: 'setInterval() is not available in GOJA engine' },
];

let monaco = null;
let editor = null;
let extraLib = null;

function loadSavedCode() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved || defaultCode;
  } catch (e) {
    return defaultCode;
  }
}

function saveCode(code) {
  try {
    localStorage.setItem(STORAGE_KEY, code);
  } catch (e) {
    // Ignore storage errors
  }
}

async function runCode() {
  const btn = document.getElementById('run-btn');
  const output = document.getElementById('output');
  const meta = document.getElementById('output-meta');
  const engine = document.getElementById('engine-select').value;

  btn.disabled = true;
  btn.textContent = '⏳ Running...';
  output.className = '';
  output.textContent = 'Executing...';
  meta.textContent = '';
  const status = document.getElementById('output-status');
  status.textContent = '';
  status.className = 'output-status';

  try {
    const code = editor.getValue();
    const response = await fetch('/execute', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code, contextCode: currentContextCode, engine })
    });

    const result = await response.json();

    if (result.error) {
      output.className = 'error';
      status.textContent = '✗ Error';
      status.className = 'output-status error';
      output.textContent = result.error;
    } else {
      output.className = 'success';
      status.textContent = '✓ Success';
      status.className = 'output-status success';
      let valueStr;
      if (typeof result.value === 'object') {
        valueStr = JSON.stringify(result.value, null, 2);
      } else {
        valueStr = String(result.value);
      }
      output.textContent = valueStr;
      meta.textContent = `${result.engine} • ${result.duration} • ${result.type}`;
    }
  } catch (err) {
    output.className = 'error';
    status.textContent = '✗ Error';
    status.className = 'output-status error';
    output.textContent = 'Network Error: ' + err.message;
  } finally {
    btn.disabled = false;
    btn.innerHTML = '▶ Run <span class="kbd">⌘↵</span>';
  }
}

function initMonaco() {
  require.config({ paths: { vs: 'https://cdn.jsdelivr.net/npm/monaco-editor@0.45.0/min/vs' } });

  require(['vs/editor/editor.main'], function (_monaco) {
    monaco = _monaco;

    // Configure TypeScript
    monaco.languages.typescript.typescriptDefaults.setCompilerOptions({
      target: monaco.languages.typescript.ScriptTarget.ESNext,
      module: monaco.languages.typescript.ModuleKind.ESNext,
      moduleResolution: monaco.languages.typescript.ModuleResolutionKind.NodeJs,
      allowNonTsExtensions: true,
      strict: true,
      noEmit: true,
    });

    // Disable default lib to avoid conflicts
    monaco.languages.typescript.typescriptDefaults.setDiagnosticsOptions({
      noSemanticValidation: false,
      noSyntaxValidation: false,
    });

    // Create editor with saved or default code
    editor = monaco.editor.create(document.getElementById('editor-container'), {
      value: loadSavedCode(),
      language: 'typescript',
      theme: 'vs-dark',
      fontSize: 14,
      lineNumbers: 'on',
      minimap: { enabled: false },
      automaticLayout: true,
      padding: { top: 15 },
      scrollBeyondLastLine: false,
      tabSize: 2,
      fixedOverflowWidgets: true,
      hover: {
        above: false,  // Force hover widgets to appear below the line
      },
    });

    // Save code to localStorage on change (debounced)
    let saveTimeout = null;
    editor.onDidChangeModelContent(() => {
      clearTimeout(saveTimeout);
      saveTimeout = setTimeout(() => saveCode(editor.getValue()), 500);
    });

    // Update cursor position in footer
    editor.onDidChangeCursorPosition((e) => {
      document.getElementById('cursor-pos').textContent = 
        `Ln ${e.position.lineNumber}, Col ${e.position.column}`;
    });

    // Cmd+Enter to run
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, runCode);

    // Validate code for engine compatibility
    editor.onDidChangeModelContent(() => {
      validateForEngine();
    });

    // Also validate when engine changes
    document.getElementById('engine-select').addEventListener('change', validateForEngine);

    // Initial validation
    validateForEngine();
  });
}

// Validate code for GOJA engine limitations
function validateForEngine() {
  if (!editor || !monaco) return;
  
  const engine = document.getElementById('engine-select').value;
  const model = editor.getModel();
  const markers = [];

  // Only add warnings for GOJA engine
  if (engine === 'goja') {
    const code = editor.getValue();
    const lines = code.split('\n');

    for (const { pattern, message } of GOJA_UNSUPPORTED_PATTERNS) {
      pattern.lastIndex = 0; // Reset regex state
      let match;
      while ((match = pattern.exec(code)) !== null) {
        // Find line and column from match index
        let charCount = 0;
        let lineNumber = 1;
        let column = 1;
        
        for (let i = 0; i < lines.length; i++) {
          if (charCount + lines[i].length >= match.index) {
            lineNumber = i + 1;
            column = match.index - charCount + 1;
            break;
          }
          charCount += lines[i].length + 1; // +1 for newline
        }

        markers.push({
          severity: monaco.MarkerSeverity.Error,
          message: message,
          startLineNumber: lineNumber,
          startColumn: column,
          endLineNumber: lineNumber,
          endColumn: column + match[0].length,
        });
      }
    }
  }

  monaco.editor.setModelMarkers(model, 'goja-compat', markers);
}

function initResizablePanels() {
  const PANEL_SIZES_KEY = 'tsgo-playground-panels';

  // Load saved panel sizes
  function loadPanelSizes() {
    try {
      const saved = localStorage.getItem(PANEL_SIZES_KEY);
      return saved ? JSON.parse(saved) : {};
    } catch (e) {
      return {};
    }
  }

  // Save panel sizes
  function savePanelSizes(sizes) {
    try {
      const current = loadPanelSizes();
      localStorage.setItem(PANEL_SIZES_KEY, JSON.stringify({ ...current, ...sizes }));
    } catch (e) {
      // Ignore storage errors
    }
  }

  // Horizontal resize for types panel
  const typesPanel = document.getElementById('types-panel');
  const typesResize = document.getElementById('types-resize');
  let isResizingH = false;

  // Restore saved sizes
  const savedSizes = loadPanelSizes();
  if (savedSizes.typesWidth) typesPanel.style.width = savedSizes.typesWidth + 'px';
  if (savedSizes.outputHeight) document.getElementById('output-panel').style.height = savedSizes.outputHeight + 'px';
  if (savedSizes.contractHeight) document.getElementById('contract-panel').style.height = savedSizes.contractHeight + 'px';

  typesResize.addEventListener('mousedown', (e) => {
    isResizingH = true;
    typesResize.classList.add('dragging');
    document.body.style.cursor = 'ew-resize';
    document.body.style.userSelect = 'none';
    e.preventDefault();
  });

  // Vertical resize for output panel
  const outputPanel = document.getElementById('output-panel');
  const outputResize = document.getElementById('output-resize');
  let isResizingV = false;

  outputResize.addEventListener('mousedown', (e) => {
    isResizingV = true;
    outputResize.classList.add('dragging');
    document.body.style.cursor = 'ns-resize';
    document.body.style.userSelect = 'none';
    e.preventDefault();
  });

  // Vertical resize for contract panel
  const contractPanel = document.getElementById('contract-panel');
  const contractResize = document.getElementById('contract-resize');
  let isResizingContract = false;

  contractResize.addEventListener('mousedown', (e) => {
    isResizingContract = true;
    contractResize.classList.add('dragging');
    document.body.style.cursor = 'ns-resize';
    document.body.style.userSelect = 'none';
    e.preventDefault();
  });

  document.addEventListener('mousemove', (e) => {
    if (isResizingH) {
      const containerRight = document.body.clientWidth;
      const newWidth = containerRight - e.clientX;
      if (newWidth >= 200 && newWidth <= 600) {
        typesPanel.style.width = newWidth + 'px';
      }
    }
    if (isResizingV) {
      const containerBottom = document.querySelector('main').getBoundingClientRect().bottom;
      const newHeight = containerBottom - e.clientY;
      if (newHeight >= 80 && newHeight <= 500) {
        outputPanel.style.height = newHeight + 'px';
      }
    }
    if (isResizingContract) {
      const panelRect = typesPanel.getBoundingClientRect();
      const newHeight = panelRect.bottom - e.clientY;
      if (newHeight >= 100 && newHeight <= 500) {
        contractPanel.style.height = newHeight + 'px';
      }
    }
  });

  document.addEventListener('mouseup', () => {
    if (isResizingH) {
      isResizingH = false;
      typesResize.classList.remove('dragging');
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      savePanelSizes({ typesWidth: parseInt(typesPanel.style.width) });
    }
    if (isResizingV) {
      isResizingV = false;
      outputResize.classList.remove('dragging');
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      savePanelSizes({ outputHeight: parseInt(outputPanel.style.height) });
    }
    if (isResizingContract) {
      isResizingContract = false;
      contractResize.classList.remove('dragging');
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      savePanelSizes({ contractHeight: parseInt(contractPanel.style.height) });
    }
  });
}

// Global reference for clearing contract from outside
let clearContractDisplay = null;

function initContractGeneration() {
  const contractContent = document.getElementById('contract-content');
  const contractStatus = document.getElementById('contract-status');
  const tabs = document.querySelectorAll('.contract-tab');
  
  let currentContract = { typescript: '', jsonSchema: '' };
  let currentTab = 'typescript';
  let debounceTimer = null;

  // Tab switching
  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      currentTab = tab.dataset.tab;
      updateContractDisplay();
    });
  });

  function updateContractDisplay() {
    if (currentTab === 'typescript') {
      contractContent.className = 'typescript';
      contractContent.textContent = currentContract.typescript || '// No contract generated';
      contractContent.setAttribute('data-lang', 'typescript');
    } else {
      contractContent.className = 'json';
      contractContent.textContent = currentContract.jsonSchema || '// No schema generated';
      contractContent.setAttribute('data-lang', 'json');
    }
    // Apply syntax highlighting
    if (monaco) {
      monaco.editor.colorizeElement(contractContent, { theme: 'vs-dark' });
    }
  }

  // Expose clear function globally
  clearContractDisplay = function() {
    clearTimeout(debounceTimer);
    currentContract = { typescript: '// Loading...', jsonSchema: '// Loading...' };
    contractStatus.textContent = 'Loading sample...';
    updateContractDisplay();
  };

  async function generateContract() {
    if (!editor) return;
    
    const code = editor.getValue();
    contractStatus.textContent = 'Generating...';
    
    try {
      const response = await fetch('/contract', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code, contextCode: currentContextCode })
      });
      
      const result = await response.json();
      
      if (result.error) {
        contractStatus.textContent = 'Error: ' + result.error;
        currentContract = { typescript: '// Error generating contract', jsonSchema: '// Error generating schema' };
      } else {
        currentContract = {
          typescript: result.typescript || '// No types exported',
          jsonSchema: result.jsonSchema || '{}'
        };
        // Show which inferrer was used
        const inferrer = result.inferrer === 'typescript' ? 'TS Compiler' : 'Go Analyzer';
        contractStatus.textContent = `Generated (${inferrer})`;
      }
      
      updateContractDisplay();
    } catch (err) {
      contractStatus.textContent = 'Network error';
      currentContract = { typescript: '// Network error', jsonSchema: '// Network error' };
      updateContractDisplay();
    }
  }

  function debouncedGenerate() {
    clearTimeout(debounceTimer);
    contractStatus.textContent = 'Waiting...';
    debounceTimer = setTimeout(generateContract, 400);
  }

  // Hook into Monaco editor when ready
  const checkEditor = setInterval(() => {
    if (editor) {
      clearInterval(checkEditor);
      editor.onDidChangeModelContent(debouncedGenerate);
      // Don't generate initial contract here - let sample loading trigger it
      // This avoids showing "any" type before the context is loaded
    }
  }, 100);
}

// Initialize everything when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  initMonaco();
  initResizablePanels();
  initContractGeneration();
  initSampleSelector();
  initWebSocket();
});

// WebSocket for connection status
function initWebSocket() {
  const wsStatus = document.getElementById('ws-status');
  const wsText = document.getElementById('ws-text');
  
  function connect() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${protocol}//${location.host}/monaco/ws`);
    
    ws.onopen = () => {
      wsStatus.classList.add('connected');
      wsStatus.title = 'Connected';
      wsText.textContent = 'Connected';
    };
    
    ws.onclose = () => {
      wsStatus.classList.remove('connected');
      wsStatus.title = 'Disconnected';
      wsText.textContent = 'Disconnected';
      // Reconnect after 3 seconds
      setTimeout(connect, 3000);
    };
    
    ws.onerror = () => {
      // Will trigger onclose
    };
    
    ws.onmessage = (event) => {
      // Handle potential future messages
      try {
        const data = JSON.parse(event.data);
        console.log('WS message:', data);
      } catch (e) {
        // Ignore non-JSON messages
      }
    };
  }
  
  connect();
}

// Update Monaco with context types
function updateMonacoTypes(contextCode) {
  if (!monaco) return;
  
  // Dispose old extra lib
  if (extraLib) {
    extraLib.dispose();
    extraLib = null;
  }
  
  if (!contextCode) {
    // No context - clear types preview
    const typesPreview = document.getElementById('types-preview');
    typesPreview.textContent = '// No context loaded\n// Select a sample to load types';
    return;
  }
  
  // Extract type declarations from context for Monaco IntelliSense
  // The context file is valid TypeScript, so we use it directly
  // But we need to convert exports to ambient declarations for the editor
  const ambientTypes = convertToAmbientDeclarations(contextCode);
  
  extraLib = monaco.languages.typescript.typescriptDefaults.addExtraLib(
    ambientTypes,
    'file:///node_modules/@types/tsgo/index.d.ts'
  );
  
  // Update types preview
  const typesPreview = document.getElementById('types-preview');
  typesPreview.textContent = ambientTypes;
  typesPreview.setAttribute('data-lang', 'typescript');
  monaco.editor.colorizeElement(typesPreview, { theme: 'vs-dark' });
}

// Extract declare const statements from context code, handling nested braces
function extractDeclareConsts(code) {
  const declares = [];
  const declareStartRegex = /declare\s+const\s+(\w+)\s*:\s*\{/g;
  let match;
  
  while ((match = declareStartRegex.exec(code)) !== null) {
    const startIndex = match.index;
    const braceStart = code.indexOf('{', startIndex);
    
    // Count braces to find matching closing brace
    let depth = 0;
    let endIndex = braceStart;
    for (let i = braceStart; i < code.length; i++) {
      if (code[i] === '{') depth++;
      else if (code[i] === '}') {
        depth--;
        if (depth === 0) {
          endIndex = i + 1;
          break;
        }
      }
    }
    
    // Include the semicolon if present
    if (code[endIndex] === ';') endIndex++;
    
    declares.push(code.slice(startIndex, endIndex));
  }
  
  return declares;
}

// Extract interfaces from context code, handling nested braces
function extractInterfaces(code) {
  const interfaces = [];
  const interfaceStartRegex = /interface\s+(\w+)\s*\{/g;
  let match;
  
  while ((match = interfaceStartRegex.exec(code)) !== null) {
    const startIndex = match.index;
    const braceStart = code.indexOf('{', startIndex);
    
    // Count braces to find matching closing brace
    let depth = 0;
    let endIndex = braceStart;
    for (let i = braceStart; i < code.length; i++) {
      if (code[i] === '{') depth++;
      else if (code[i] === '}') {
        depth--;
        if (depth === 0) {
          endIndex = i + 1;
          break;
        }
      }
    }
    
    interfaces.push(code.slice(startIndex, endIndex));
  }
  
  return interfaces;
}

// Convert context code to ambient declarations for Monaco
function convertToAmbientDeclarations(contextCode) {
  const parts = [];
  parts.push('// Auto-generated from context file');
  
  // Extract existing declare const statements (including multi-line with nested braces)
  const declareConsts = extractDeclareConsts(contextCode);
  if (declareConsts.length > 0) {
    parts.push(''); // blank line before declare consts
    parts.push(...declareConsts);
  }
  
  // Extract interface declarations with proper nested brace handling
  const interfaces = extractInterfaces(contextCode);
  if (interfaces.length > 0) {
    parts.push(''); // blank line before interfaces
    parts.push(...interfaces);
  }
  
  // Convert exported const to declare const
  const constRegex = /export\s+const\s+(\w+)\s*:\s*([^=]+)\s*=/g;
  let match;
  const consts = [];
  const foundConstNames = new Set();
  
  // First pass: consts with explicit type annotations
  while ((match = constRegex.exec(contextCode)) !== null) {
    const [, name, type] = match;
    consts.push(`declare const ${name}: ${type.trim()};`);
    foundConstNames.add(name);
  }
  
  // Second pass: consts without type annotations (infer from value)
  const constNoTypeRegex = /export\s+const\s+(\w+)\s*=\s*([^;]+);/g;
  while ((match = constNoTypeRegex.exec(contextCode)) !== null) {
    const [, name, value] = match;
    if (foundConstNames.has(name)) continue; // Already found with explicit type
    
    // Infer type from value
    const trimmedValue = value.trim();
    let inferredType = 'any';
    if (/^-?\d+\.\d+$/.test(trimmedValue) || /^-?\d+$/.test(trimmedValue)) {
      inferredType = 'number';
    } else if (/^["'`]/.test(trimmedValue)) {
      inferredType = 'string';
    } else if (trimmedValue === 'true' || trimmedValue === 'false') {
      inferredType = 'boolean';
    } else if (trimmedValue.startsWith('[')) {
      inferredType = 'any[]';
    } else if (trimmedValue.startsWith('{')) {
      inferredType = 'object';
    }
    
    consts.push(`declare const ${name}: ${inferredType};`);
    foundConstNames.add(name);
  }
  
  if (consts.length > 0) {
    parts.push(''); // blank line before consts
    parts.push(...consts);
  }
  
  // Convert exported functions to declare function
  const funcRegex = /export\s+function\s+(\w+)\s*\(([^)]*)\)\s*:\s*(\w+)/g;
  const funcs = [];
  while ((match = funcRegex.exec(contextCode)) !== null) {
    const [, name, params, returnType] = match;
    funcs.push(`declare function ${name}(${params}): ${returnType};`);
  }
  if (funcs.length > 0) {
    parts.push(''); // blank line before functions
    parts.push(...funcs);
  }
  
  return parts.join('\n');
}

// Load a sample and its context from the server
async function loadSample(sampleId) {
  if (!sampleId || !editor) return;
  
  const sampleSelect = document.getElementById('sample-select');
  const engineSelect = document.getElementById('engine-select');
  const output = document.getElementById('output');
  const typesPreview = document.getElementById('types-preview');
  
  // Clear all panels immediately when switching samples
  output.textContent = '// Loading sample...';
  output.className = '';
  document.querySelector('.output-meta').textContent = '';
  const outputStatus = document.getElementById('output-status');
  outputStatus.textContent = '';
  outputStatus.className = 'output-status';
  
  // Clear types preview
  typesPreview.textContent = '// Loading context...';
  
  // Clear contract panel
  if (clearContractDisplay) {
    clearContractDisplay();
  }
  
  try {
    // Fetch sample from the /sample/ endpoint which splits context and code
    const response = await fetch(`/sample/${sampleId}`);
    
    if (!response.ok) {
      throw new Error(`Sample not found: ${sampleId}`);
    }
    
    const { context, code } = await response.json();
    
    // Update context types in Monaco
    currentContextCode = context || '';
    updateMonacoTypes(currentContextCode);
    
    // Strip triple-slash references for Monaco display
    const displayCode = stripTripleSlashRefs(code);
    
    // Update editor
    editor.setValue(displayCode);
    
    // Switch engine if sample requires Bun
    if (SAMPLE_ENGINES[sampleId]) {
      engineSelect.value = SAMPLE_ENGINES[sampleId];
      validateForEngine();
    } else {
      engineSelect.value = 'auto';
      validateForEngine();
    }
    
    // Keep track of current sample
    currentSampleId = sampleId;
    sampleSelect.value = sampleId;
    
    // Save to localStorage
    saveCode(displayCode);
    localStorage.setItem(CONTEXT_KEY, currentContextCode);
    localStorage.setItem(SAMPLE_KEY, sampleId);
    
  } catch (err) {
    console.error('Failed to load sample:', err);
    alert(`Failed to load sample: ${err.message}`);
  }
}

function initSampleSelector() {
  const sampleSelect = document.getElementById('sample-select');
  
  // Restore sample from localStorage or load default
  let sampleToLoad = DEFAULT_SAMPLE;
  try {
    const savedSample = localStorage.getItem(SAMPLE_KEY);
    if (savedSample) {
      sampleToLoad = savedSample;
    }
  } catch (e) {
    // Ignore storage errors
  }
  
  // Wait for Monaco to be ready, then load sample
  const checkMonaco = setInterval(() => {
    if (monaco && editor) {
      clearInterval(checkMonaco);
      loadSample(sampleToLoad);
    }
  }, 100);
  
  sampleSelect.addEventListener('change', (e) => {
    const sampleId = e.target.value;
    if (sampleId) {
      loadSample(sampleId);
    }
  });
}
