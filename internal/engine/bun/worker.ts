// Bun worker script for tsgo executor
// This script runs as a persistent process and handles JSON-RPC requests over stdin/stdout

// Bun runtime globals
declare const Bun: {
  stdin: {
    stream(): AsyncIterable<Uint8Array>;
  };
};

declare const process: {
  exit(code?: number): never;
  stdout: {
    write(data: string): void;
  };
};

interface RpcRequest {
  id: string;
  method: string;
  code?: string;
  context?: Record<string, unknown>;
  policy?: SecurityPolicy;
}

interface RpcResponse {
  id: string;
  result?: unknown;
  error?: {
    message: string;
    stack?: string;
  };
  logs?: string[];
  metrics?: {
    executionTimeMs: number;
  };
}

interface SecurityPolicy {
  networkAccess?: boolean;
  diskAccess?: boolean;
}

let currentLogs: string[] = [];

function safeStringify(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (value === null) {
    return 'null';
  }
  if (value === undefined) {
    return 'undefined';
  }
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function logWithLevel(level: string, args: unknown[]): void {
  const line = args.map(safeStringify).join(' ');
  currentLogs.push(`${level}: ${line}`);
}

// Override console to prevent stdout corruption of the RPC channel
// and capture logs for the response payload.
(globalThis as Record<string, unknown>).console = {
  log: (...args: unknown[]) => logWithLevel('log', args),
  info: (...args: unknown[]) => logWithLevel('info', args),
  warn: (...args: unknown[]) => logWithLevel('warn', args),
  error: (...args: unknown[]) => logWithLevel('error', args),
};

// Security: Remove dangerous globals if restricted
// eslint-disable-next-line @typescript-eslint/no-unused-vars
function applySecurityPolicy(policy?: SecurityPolicy): void {
  if (!policy) {
    return;
  }

  if (!policy.networkAccess) {
    // Intentionally removing network globals for sandboxing
    (globalThis as Record<string, unknown>).fetch = undefined;
    (globalThis as Record<string, unknown>).WebSocket = undefined;
  } else {
    // Restore network globals if they were disabled in a previous run
    (globalThis as Record<string, unknown>).fetch = baseFetch;
    (globalThis as Record<string, unknown>).WebSocket = baseWebSocket;
  }

  if (!policy.diskAccess) {
    // Note: Bun's file APIs are module-based, harder to fully restrict
    // Process-level sandboxing (seccomp/landlock) should be used for strong isolation
  }
}

// Capture base globalThis keys at startup for isolation cleanup
const baseGlobalKeys = new Set(Object.keys(globalThis));
const baseFetch = (globalThis as Record<string, unknown>).fetch;
const baseWebSocket = (globalThis as Record<string, unknown>).WebSocket;

// Clean up any globals added during execution (context isolation)
function cleanupGlobals(): void {
  const currentKeys = Object.keys(globalThis);
  for (const key of currentKeys) {
    if (!baseGlobalKeys.has(key)) {
      delete (globalThis as Record<string, unknown>)[key];
    }
  }
}

function isValidIdentifier(name: string): boolean {
  return /^[A-Za-z_$][A-Za-z0-9_$]*$/.test(name);
}

function buildContextSetup(context: Record<string, unknown>, declarationKeyword: 'var' | 'const'): string {
  const keys = Object.keys(context);
  if (keys.length === 0) {
    return '';
  }

  const lines: string[] = [`const __context__ = ${JSON.stringify(context)};`];
  for (const key of keys) {
    const keyLiteral = JSON.stringify(key);
    if (isValidIdentifier(key)) {
      lines.push(`${declarationKeyword} ${key} = __context__[${keyLiteral}];`);
    } else {
      lines.push(`(globalThis as Record<string, unknown>)[${keyLiteral}] = __context__[${keyLiteral}];`);
    }
  }

  return lines.join('\n');
}

// Execute user code and extract default export
async function executeCode(code: string, context: Record<string, unknown>): Promise<unknown> {
  // Check if code is already transpiled
  // IIFE format: contains __tsgo_exports__
  // ESM format: starts with "// " comment (esbuild adds sourcemap comment) and has "export"
  const isIIFE = code.includes('__tsgo_exports__');
  const isESM = code.includes('export ') || code.includes('export{');
  const isTranspiled = isIIFE || isESM;
  
  if (isIIFE) {
    // The code comes pre-transpiled from esbuild as an IIFE with GlobalName="__tsgo_exports__"
    // Format: var __tsgo_exports__ = (() => { ... return exports; })();
    
    // Build context injection for the Function constructor
    const contextSetup = buildContextSetup(context, 'var');
    
    const wrappedCode = `
${contextSetup}
${code}
return typeof __tsgo_exports__ !== 'undefined' ? __tsgo_exports__ : undefined;
`;
    
    // Use Function constructor to execute the code
    const fn = new Function(wrappedCode);
    const exports = fn();

    // Handle the exports object
    if (exports && typeof exports === 'object') {
      // Check for default export
      if ('default' in exports) {
        const defaultExport = exports.default;
        // If default export is a function, invoke it
        if (typeof defaultExport === 'function') {
          const result = defaultExport();
          if (result instanceof Promise) {
            return await result;
          }
          return result;
        }
        return defaultExport;
      }
      // Return the exports object if it has properties
      if (Object.keys(exports).length > 0) {
        return exports;
      }
    }
    
    return exports;
  } else if (isESM) {
    // ESM format from esbuild - use dynamic import via Blob URL
    // ESM already handles exports properly
    const contextSetup = buildContextSetup(context, 'const');

    const wrappedCode = `
${contextSetup}

${code}
`;

    const blob = new Blob([wrappedCode], { type: 'application/javascript' });
    const url = URL.createObjectURL(blob);

    try {
      const module = await import(url);
      
      // Get the default export
      const handler = module.default;
      
      // If default export is a function, invoke it
      if (typeof handler === 'function') {
        const result = handler();
        
        // Handle async functions
        if (result instanceof Promise) {
          return await result;
        }
        return result;
      }
      
      // Return the value directly if not a function
      return handler;
    } finally {
      URL.revokeObjectURL(url);
    }
  } else {
    // Raw TypeScript/JavaScript code - use dynamic import via Blob URL
    // Inject context via JSON serialization for full isolation
    const contextSetup = buildContextSetup(context, 'const');

    const wrappedCode = `
${contextSetup}

${code}
`;

    const blob = new Blob([wrappedCode], { type: 'application/typescript' });
    const url = URL.createObjectURL(blob);

    try {
      const module = await import(url);
      
      // Get the default export
      const handler = module.default;
      
      // If default export is a function, invoke it
      if (typeof handler === 'function') {
        const result = handler();
        
        // Handle async functions
        if (result instanceof Promise) {
          return await result;
        }
        return result;
      }
      
      // Return the value directly if not a function
      return handler;
    } finally {
      URL.revokeObjectURL(url);
    }
  }
}

// Handle incoming request
async function handleRequest(request: RpcRequest): Promise<RpcResponse> {
  const startTime = performance.now();

  try {
    switch (request.method) {
      case 'ping':
        return { id: request.id, result: 'pong' };
        
      case 'execute':
        if (!request.code) {
          return {
            id: request.id,
            error: { message: 'missing code in execute request' }
          };
        }
        
        try {
          currentLogs = [];
          applySecurityPolicy(request.policy);
          const result = await executeCode(request.code, request.context || {});
          const executionTimeMs = performance.now() - startTime;
          
          return {
            id: request.id,
            result,
            logs: currentLogs,
            metrics: { executionTimeMs }
          };
        } finally {
          // Clean up any globals added during execution (context isolation)
          cleanupGlobals();
        }
        
      case 'shutdown':
        setTimeout(() => process.exit(0), 100);
        return { id: request.id, result: 'shutting down' };
        
      default:
        return {
          id: request.id,
          error: { message: `unknown method: ${request.method}` }
        };
    }
  } catch (err) {
    const error = err instanceof Error ? err : new Error(String(err));
    return {
      id: request.id,
      error: {
        message: error.message,
        stack: error.stack
      }
    };
  }
}

// Send response to stdout
function sendResponse(response: RpcResponse): void {
  const json = JSON.stringify(response);
  process.stdout.write(`${json}\n`);
}

// Read and process requests from stdin
async function main(): Promise<void> {
  const decoder = new TextDecoder();
  let buffer = '';
  
  // Signal ready
  sendResponse({ id: '0', result: 'ready' });
  
  for await (const chunk of Bun.stdin.stream()) {
    buffer += decoder.decode(chunk);
    
    // Process complete lines
    let newlineIndex;
    while ((newlineIndex = buffer.indexOf('\n')) !== -1) {
      const line = buffer.slice(0, newlineIndex);
      buffer = buffer.slice(newlineIndex + 1);
      
      if (line.trim()) {
        try {
          const request = JSON.parse(line) as RpcRequest;
          // Handle request sequentially to ensure isolation
          // Concurrency is achieved at the process pool level
          try {
            const response = await handleRequest(request);
            sendResponse(response);
          } catch (err) {
            sendResponse({
              id: request.id,
              error: { message: `execution error: ${err}` }
            });
          }
        } catch (err) {
          sendResponse({
            id: 'unknown',
            error: { message: `invalid JSON: ${err}` }
          });
        }
      }
    }
  }
}

main().catch(console.error);
