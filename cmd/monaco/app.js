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
  userRole: user.role
};
`;

let ws = null;
let monaco = null;
let editor = null;
let extraLib = null;

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
      // Update preview
      document.getElementById('types-preview').textContent = data.types;
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

    // Create editor
    editor = monaco.editor.create(document.getElementById('editor-container'), {
      value: defaultCode,
      language: 'typescript',
      theme: 'vs-dark',
      fontSize: 14,
      lineNumbers: 'on',
      minimap: { enabled: false },
      automaticLayout: true,
      padding: { top: 15 },
      scrollBeyondLastLine: false,
      tabSize: 2,
    });

    // Update cursor position in footer
    editor.onDidChangeCursorPosition((e) => {
      document.getElementById('cursor-pos').textContent = 
        `Ln ${e.position.lineNumber}, Col ${e.position.column}`;
    });

    // Cmd+Enter to run
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, runCode);

    // Connect to WebSocket for live type updates
    connectWebSocket();
  });
}

function initResizablePanels() {
  // Horizontal resize for types panel
  const typesPanel = document.getElementById('types-panel');
  const typesResize = document.getElementById('types-resize');
  let isResizingH = false;

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
    }
    if (isResizingV) {
      isResizingV = false;
      outputResize.classList.remove('dragging');
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    }
    if (isResizingContract) {
      isResizingContract = false;
      contractResize.classList.remove('dragging');
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
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
    } else {
      contractContent.className = 'json';
      contractContent.textContent = currentContract.jsonSchema || '// No schema generated';
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
