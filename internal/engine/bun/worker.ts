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
};

interface RpcRequest {
  id: string;
  method: string;
  code?: string;
  context?: Record<string, unknown>;
}

interface RpcResponse {
  id: string;
  result?: unknown;
  error?: {
    message: string;
    stack?: string;
  };
  metrics?: {
    executionTimeMs: number;
  };
}

interface SecurityPolicy {
  networkAccess?: boolean;
  diskAccess?: boolean;
}

// Security: Remove dangerous globals if restricted
// eslint-disable-next-line @typescript-eslint/no-unused-vars
function applySecurityPolicy(policy: SecurityPolicy): void {
  if (!policy.networkAccess) {
    // Intentionally removing network globals for sandboxing
    (globalThis as Record<string, unknown>).fetch = undefined;
    (globalThis as Record<string, unknown>).WebSocket = undefined;
  }

  if (!policy.diskAccess) {
    // Note: Bun's file APIs are module-based, harder to fully restrict
    // Process-level sandboxing (seccomp/landlock) should be used for strong isolation
  }
}

// Execute user code and extract default export
async function executeCode(code: string, context: Record<string, unknown>): Promise<unknown> {
  // Inject context as globals
  for (const [key, value] of Object.entries(context)) {
    (globalThis as Record<string, unknown>)[key] = value;
  }

  // Create a blob URL for dynamic import
  const wrappedCode = `
${code}

// Export handler extraction
const __mod__ = { default: typeof exports !== 'undefined' ? exports.default : undefined };
export { __mod__ };
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

    // Clean up injected globals
    for (const key of Object.keys(context)) {
      delete (globalThis as Record<string, unknown>)[key];
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
        
        const result = await executeCode(request.code, request.context || {});
        const executionTimeMs = performance.now() - startTime;
        
        return {
          id: request.id,
          result,
          metrics: { executionTimeMs }
        };
        
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
  console.log(json);
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
          const response = await handleRequest(request);
          sendResponse(response);
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
