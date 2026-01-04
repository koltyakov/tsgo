const defaultCode = `// tsgo TypeScript Playground
// Press ⌘+Enter to run, or click the Run button

// Access injected global variables with full type support
const user: User = currentUser;
const greeting = \`Hello, \${user.name}!\`;

// Use the config object (console.log works in both GOJA and Bun)
if (config.debug) {
  console.log("Debug mode enabled");
  console.log(\`API URL: \${config.apiUrl}\`);
}

// Use injected helper functions (work in both GOJA and Bun!)
const total = sum(10, 20);
const product = multiply(5, 6);

// Create a new user object
const newUser: User = {
  id: 2,
  name: "Alice",
  email: "alice@example.com",
  role: "user"
};

// Do some computation
function fibonacci(n: number): number {
  if (n <= 1) return n;
  return fibonacci(n - 1) + fibonacci(n - 2);
}

const fib10 = fibonacci(10);

// Return a result (last expression or export default)
export default {
  greeting,
  newUser,
  fib10,
  userRole: user.role,
  // Show the injected function results
  mathResults: { sum: total, product }
};
`;

const STORAGE_KEY = 'tsgo-playground-code';

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

let ws = null;
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

function updateStatus(connected) {
  const dot = document.getElementById('status-dot');
  const text = document.getElementById('status-text');
  if (connected) {
    dot.classList.add('connected');
    text.textContent = 'Connected';
  } else {
    dot.classList.remove('connected');
    text.textContent = 'Disconnected';
  }
}

function connectWebSocket() {
  ws = new WebSocket('ws://localhost:8080/monaco/ws');

  ws.onopen = () => updateStatus(true);
  ws.onclose = () => {
    updateStatus(false);
    setTimeout(connectWebSocket, 2000);
  };

  ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'types' && monaco) {
      // Update Monaco types
      if (extraLib) extraLib.dispose();
      extraLib = monaco.languages.typescript.typescriptDefaults.addExtraLib(
        data.types,
        'file:///node_modules/@types/tsgo/index.d.ts'
      );
      // Update preview with syntax highlighting
      const typesPreview = document.getElementById('types-preview');
      typesPreview.textContent = data.types;
      typesPreview.setAttribute('data-lang', 'typescript');
      monaco.editor.colorizeElement(typesPreview, { theme: 'vs-dark' });
    }
  };
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

  try {
    const code = editor.getValue();
    const response = await fetch('/execute', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code, engine })
    });

    const result = await response.json();

    if (result.error) {
      output.className = 'error';
      output.textContent = '❌ Error:\n' + result.error;
    } else {
      output.className = 'success';
      let valueStr;
      if (typeof result.value === 'object') {
        valueStr = JSON.stringify(result.value, null, 2);
      } else {
        valueStr = String(result.value);
      }
      output.textContent = '✅ Result:\n' + valueStr;
      meta.textContent = `${result.engine} • ${result.duration} • ${result.type}`;
    }
  } catch (err) {
    output.className = 'error';
    output.textContent = '❌ Network Error:\n' + err.message;
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

    // Connect to WebSocket for live type updates
    connectWebSocket();
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

  async function generateContract() {
    if (!editor) return;
    
    const code = editor.getValue();
    contractStatus.textContent = 'Generating...';
    
    try {
      const response = await fetch('/contract', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code })
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
        const count = result.contract?.types?.length || 0;
        contractStatus.textContent = `Generated ${count} type${count !== 1 ? 's' : ''}`;
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
      // Generate initial contract
      generateContract();
    }
  }, 100);
}

// Initialize everything when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  initMonaco();
  initResizablePanels();
  initContractGeneration();
});
