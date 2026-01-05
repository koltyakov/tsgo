#!/usr/bin/env bun
/**
 * TypeScript Type Inference Worker
 *
 * Runs as a persistent process, receiving inference requests over stdin
 * and sending results to stdout. Uses TypeScript's type checker API
 * for accurate type information with full type expansion.
 */

// @ts-ignore - TypeScript module available at runtime via Bun
import ts from 'typescript';

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
}

interface InferenceResult {
  type: string;
  kind: 'primitive' | 'object' | 'array' | 'union' | 'function' | 'literal' | 'any';
  properties?: Array<{
    name: string;
    type: string;
    optional: boolean;
  }>;
  elementType?: string;
  returnType?: string;
  error?: string;
}

interface RpcResponse {
  id: string;
  result?: InferenceResult;
  error?: {
    message: string;
    stack?: string;
  };
}

/**
 * Check if code has an export default statement
 */
function hasDefaultExport(code: string): boolean {
  return /\bexport\s+default\b/.test(code) || /\bexport\s*\{[^}]*\bas\s+default\b/.test(code);
}

/**
 * Get the last expression from code that could be a REPL result.
 * Returns the modified code with export default added, or null if not applicable.
 */
function addDefaultExportForLastExpression(code: string): string | null {
  if (hasDefaultExport(code)) {
    return null;
  }

  const lines = code.split('\n');
  let lastExprIndex = -1;
  let lastExpr = '';

  for (let i = lines.length - 1; i >= 0; i--) {
    const trimmed = lines[i].trim();
    if (!trimmed || trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) {
      continue;
    }
    if (
      trimmed.startsWith('import ') ||
      trimmed.startsWith('export ') ||
      trimmed.startsWith('type ') ||
      trimmed.startsWith('interface ') ||
      trimmed.startsWith('declare ') ||
      trimmed.startsWith('const ') ||
      trimmed.startsWith('let ') ||
      trimmed.startsWith('var ') ||
      trimmed.startsWith('function ') ||
      trimmed.startsWith('class ') ||
      trimmed.startsWith('enum ') ||
      trimmed.startsWith('namespace ') ||
      trimmed.startsWith('if ') ||
      trimmed.startsWith('for ') ||
      trimmed.startsWith('while ') ||
      trimmed.startsWith('switch ') ||
      trimmed.startsWith('try ') ||
      trimmed.startsWith('return ') ||
      trimmed.startsWith('throw ') ||
      trimmed === '}' ||
      trimmed === '{' ||
      trimmed === ');'
    ) {
      return null;
    }

    lastExprIndex = i;
    lastExpr = trimmed;
    break;
  }

  if (lastExprIndex === -1 || !lastExpr) {
    return null;
  }

  const expr = lastExpr.replace(/;$/, '');
  if (!expr) {
    return null;
  }

  const newLines = [...lines];
  newLines[lastExprIndex] = `export default (${expr});`;

  return newLines.join('\n');
}

/**
 * Infer the type of the default export from TypeScript code using the type checker.
 * This properly expands type aliases and handles complex nested types.
 */
function inferDefaultExportType(code: string): InferenceResult {
  const modifiedCode = addDefaultExportForLastExpression(code);
  const codeToAnalyze = modifiedCode ?? code;

  const fileName = 'input.ts';

  const files: Map<string, string> = new Map();
  files.set(fileName, codeToAnalyze);

  const options: ts.CompilerOptions = {
    target: ts.ScriptTarget.ESNext,
    module: ts.ModuleKind.ESNext,
    strict: true,
    noEmit: true,
  };

  const host = ts.createCompilerHost(options);
  const originalGetSourceFile = host.getSourceFile;
  host.getSourceFile = (name: string, languageVersion: ts.ScriptTarget) => {
    if (files.has(name)) {
      return ts.createSourceFile(name, files.get(name)!, ts.ScriptTarget.ESNext, true);
    }
    return originalGetSourceFile(name, languageVersion);
  };
  host.fileExists = (name: string) => files.has(name) || ts.sys.fileExists(name);
  host.readFile = (name: string) => files.get(name) ?? ts.sys.readFile(name);

  const program = ts.createProgram([fileName], options, host);
  const checker = program.getTypeChecker();
  const sourceFile = program.getSourceFile(fileName);

  if (!sourceFile) {
    return { type: 'any', kind: 'any', error: 'Could not parse source file' };
  }

  // Find the default export
  let exportType: ts.Type | undefined;
  let exportNode: ts.Node | undefined;
  let literalFallback: InferenceResult | undefined;

  ts.forEachChild(sourceFile, (node: ts.Node) => {
    // Handle: export default expr;
    if (ts.isExportAssignment(node) && !node.isExportEquals) {
      exportNode = node.expression;
      exportType = checker.getTypeAtLocation(node.expression);
      
      // TypeScript's type checker returns 'any' for direct literal exports like
      // `export default 42` or `export default "hello"`. Detect these via AST.
      const expr = node.expression;
      if (ts.isNumericLiteral(expr)) {
        literalFallback = { type: 'number', kind: 'primitive' };
      } else if (ts.isStringLiteral(expr)) {
        literalFallback = { type: 'string', kind: 'primitive' };
      } else if (ts.isBigIntLiteral(expr)) {
        literalFallback = { type: 'bigint', kind: 'primitive' };
      } else if (expr.kind === ts.SyntaxKind.NullKeyword) {
        literalFallback = { type: 'null', kind: 'literal' };
      } else if (expr.kind === ts.SyntaxKind.TrueKeyword || expr.kind === ts.SyntaxKind.FalseKeyword) {
        // Direct boolean literal exports - type checker returns literal type, we want primitive
        literalFallback = { type: 'boolean', kind: 'primitive' };
      } else if (ts.isArrowFunction(expr) || ts.isFunctionExpression(expr)) {
        // Direct function exports - extract the return type
        // For `export default async () => { return 42; }` we want number (awaited)
        const sig = checker.getSignatureFromDeclaration(expr);
        if (sig) {
          let returnType = checker.getReturnTypeOfSignature(sig);
          // Unwrap Promise<T> to get the awaited type
          returnType = unwrapPromiseType(returnType, checker);
          const returnTypeStr = checker.typeToString(returnType, undefined, 
            ts.TypeFormatFlags.NoTruncation | ts.TypeFormatFlags.InTypeAlias);
          // Determine kind based on return type
          let kind: InferenceResult['kind'] = 'any';
          if (returnType.flags & ts.TypeFlags.Number) kind = 'primitive';
          else if (returnType.flags & ts.TypeFlags.String) kind = 'primitive';
          else if (returnType.flags & ts.TypeFlags.Boolean) kind = 'primitive';
          else if (returnType.flags & ts.TypeFlags.Void) kind = 'primitive';
          else if (checker.isArrayType(returnType)) kind = 'array';
          else if (returnType.isUnion()) kind = 'union';
          else if (returnType.flags & ts.TypeFlags.Object) kind = 'object';
          literalFallback = { type: returnTypeStr, kind, returnType: returnTypeStr };
        }
      } else if (ts.isArrayLiteralExpression(expr)) {
        // Array literals with direct export - infer element type
        if (expr.elements.length === 0) {
          literalFallback = { type: 'never[]', kind: 'array', elementType: 'never' };
        }
        // Non-empty arrays are typically handled correctly
      } else if (ts.isObjectLiteralExpression(expr)) {
        // Object literals with direct export - checker usually handles this
      }
    }
    // Handle: export { x as default };
    else if (ts.isExportDeclaration(node) && node.exportClause && ts.isNamedExports(node.exportClause)) {
      for (const element of node.exportClause.elements) {
        if (element.name.text === 'default') {
          exportNode = element;
          exportType = checker.getTypeAtLocation(element);
        }
      }
    }
    // Handle: export default function() {} or export default async function() {}
    else if (ts.isFunctionDeclaration(node)) {
      const isDefault = node.modifiers?.some((m: ts.ModifierLike) => m.kind === ts.SyntaxKind.DefaultKeyword);
      const isExport = node.modifiers?.some((m: ts.ModifierLike) => m.kind === ts.SyntaxKind.ExportKeyword);
      if (isDefault && isExport) {
        exportNode = node;
        exportType = checker.getTypeAtLocation(node);
        // Extract the return type
        const sig = checker.getSignatureFromDeclaration(node);
        if (sig) {
          let returnType = checker.getReturnTypeOfSignature(sig);
          // Unwrap Promise<T> to get the awaited type
          returnType = unwrapPromiseType(returnType, checker);
          const returnTypeStr = checker.typeToString(returnType, undefined,
            ts.TypeFormatFlags.NoTruncation | ts.TypeFormatFlags.InTypeAlias);
          let kind: InferenceResult['kind'] = 'any';
          if (returnType.flags & ts.TypeFlags.Number) kind = 'primitive';
          else if (returnType.flags & ts.TypeFlags.String) kind = 'primitive';
          else if (returnType.flags & ts.TypeFlags.Boolean) kind = 'primitive';
          else if (returnType.flags & ts.TypeFlags.Void) kind = 'primitive';
          else if (checker.isArrayType(returnType)) kind = 'array';
          else if (returnType.isUnion()) kind = 'union';
          else if (returnType.flags & ts.TypeFlags.Object) kind = 'object';
          literalFallback = { type: returnTypeStr, kind, returnType: returnTypeStr };
        }
      }
    }
  });

  if (!exportType || !exportNode) {
    return { type: 'void', kind: 'primitive' };
  }

  // Get the fully expanded type string
  // Use InTypeAlias to expand all type aliases recursively
  const typeString = checker.typeToString(
    exportType,
    exportNode,
    ts.TypeFormatFlags.NoTruncation |
    ts.TypeFormatFlags.InTypeAlias
  );

  // If we detected a direct literal export, use the widened type
  // This handles: export default 42, export default "hi", export default true
  if (literalFallback) {
    return literalFallback;
  }

  // Format the type string for readability with newlines
  const formattedType = formatTypeString(typeString, exportType, checker);

  return analyzeTypeString(formattedType, exportType, checker);
}

/**
 * Unwrap Promise<T> to get T (the awaited type)
 */
function unwrapPromiseType(type: ts.Type, checker: ts.TypeChecker): ts.Type {
  const typeRef = type as ts.TypeReference;
  if (typeRef.target) {
    const symbol = typeRef.target.getSymbol();
    if (symbol && symbol.getName() === 'Promise') {
      const typeArgs = checker.getTypeArguments(typeRef);
      if (typeArgs.length > 0) {
        return typeArgs[0];
      }
    }
  }
  return type;
}

/**
 * Set of built-in types that should NOT be expanded
 */
const PRESERVE_TYPES = new Set([
  'Promise', 'Map', 'Set', 'WeakMap', 'WeakSet',
  'Date', 'RegExp', 'Error', 'URL', 'URLSearchParams',
  'ArrayBuffer', 'SharedArrayBuffer', 'DataView',
  'Int8Array', 'Uint8Array', 'Uint8ClampedArray',
  'Int16Array', 'Uint16Array', 'Int32Array', 'Uint32Array',
  'Float32Array', 'Float64Array', 'BigInt64Array', 'BigUint64Array',
  'ReadableStream', 'WritableStream', 'TransformStream',
  'Blob', 'File', 'FormData', 'Headers', 'Request', 'Response',
]);

/**
 * Check if a type is a built-in that should be preserved
 */
function isPreservedType(type: ts.Type, checker: ts.TypeChecker): { preserved: boolean; name?: string; typeArgs?: readonly ts.Type[] } {
  // Check if it's a TypeReference with a target
  const typeRef = type as ts.TypeReference;
  if (typeRef.target && typeRef.target !== type) {
    const symbol = typeRef.target.getSymbol();
    if (symbol) {
      const name = symbol.getName();
      if (PRESERVE_TYPES.has(name)) {
        const typeArgs = checker.getTypeArguments(typeRef);
        return { preserved: true, name, typeArgs };
      }
    }
  }
  return { preserved: false };
}

/**
 * Check if a union type represents an enum and return the enum name
 */
function getEnumName(type: ts.UnionType, checker: ts.TypeChecker): string | null {
  if (!type.types || type.types.length === 0) return null;
  
  let enumName: string | null = null;
  
  for (const member of type.types) {
    // Get the symbol for this type
    const symbol = member.getSymbol();
    if (!symbol) return null;
    
    // Check if this symbol's parent is an enum
    const parent = symbol.parent;
    if (!parent) return null;
    
    const parentDecl = parent.declarations?.[0];
    if (!parentDecl || !ts.isEnumDeclaration(parentDecl)) return null;
    
    const thisEnumName = parent.getName();
    if (enumName === null) {
      enumName = thisEnumName;
    } else if (enumName !== thisEnumName) {
      // Different enums in the union
      return null;
    }
  }
  
  return enumName;
}

/**
 * Check if a union consists entirely of same-type literals that can be widened
 * e.g., 1 | 0 -> "number", "a" | "b" -> "string"
 * Returns null if not all same type, or if types include non-literals
 */
function getWidenedLiteralUnion(type: ts.UnionType): string | null {
  if (!type.types || type.types.length === 0) return null;
  
  let allNumericLiterals = true;
  let allStringLiterals = true;
  let allBooleanLiterals = true;
  
  for (const member of type.types) {
    if (!member.isNumberLiteral()) allNumericLiterals = false;
    if (!member.isStringLiteral()) allStringLiterals = false;
    if (!(member.flags & ts.TypeFlags.BooleanLiteral)) allBooleanLiterals = false;
  }
  
  if (allNumericLiterals) return 'number';
  if (allStringLiterals) return 'string';
  if (allBooleanLiterals) return 'boolean';
  
  return null;
}

/**
 * Recursively expand a type to its full structural representation
 */
function expandType(type: ts.Type, checker: ts.TypeChecker, visited: Set<ts.Type> = new Set(), depth: number = 0): string {
  // Prevent infinite recursion
  if (visited.has(type) || depth > 10) {
    return checker.typeToString(type);
  }
  visited.add(type);

  // Check for preserved built-in types first
  const preserved = isPreservedType(type, checker);
  if (preserved.preserved && preserved.name) {
    if (preserved.typeArgs && preserved.typeArgs.length > 0) {
      const expandedArgs = preserved.typeArgs.map(t => expandType(t, checker, new Set(visited), depth + 1));
      return `${preserved.name}<${expandedArgs.join(', ')}>`;
    }
    return preserved.name;
  }

  // Handle literal types
  if (type.isStringLiteral()) {
    return `"${type.value}"`;
  }
  if (type.isNumberLiteral()) {
    return String(type.value);
  }
  if (type.flags & ts.TypeFlags.BooleanLiteral) {
    return (type as ts.IntrinsicType).intrinsicName;
  }

  // Handle primitive types
  if (type.flags & ts.TypeFlags.String) return 'string';
  if (type.flags & ts.TypeFlags.Number) return 'number';
  if (type.flags & ts.TypeFlags.Boolean) return 'boolean';
  if (type.flags & ts.TypeFlags.BigInt) return 'bigint';
  if (type.flags & ts.TypeFlags.Null) return 'null';
  if (type.flags & ts.TypeFlags.Undefined) return 'undefined';
  if (type.flags & ts.TypeFlags.Void) return 'void';
  if (type.flags & ts.TypeFlags.Never) return 'never';
  if (type.flags & ts.TypeFlags.Any) return 'any';
  if (type.flags & ts.TypeFlags.Unknown) return 'unknown';

  // Handle enum types - check if all union members come from same enum
  if (type.isUnion()) {
    const enumName = getEnumName(type, checker);
    if (enumName) {
      return enumName;
    }
    
    // Widen unions of same-type literals to their base type
    // e.g., 1 | 0 -> number, "a" | "b" -> string (but only when inferred, not explicit)
    const widenedType = getWidenedLiteralUnion(type);
    if (widenedType) {
      return widenedType;
    }
    
    const parts = type.types.map((t: ts.Type) => expandType(t, checker, new Set(visited), depth + 1));
    return parts.join(' | ');
  }

  // Handle intersection types
  if (type.isIntersection()) {
    const parts = type.types.map((t: ts.Type) => expandType(t, checker, new Set(visited), depth + 1));
    return parts.join(' & ');
  }

  // Handle array types
  if (checker.isArrayType(type)) {
    const typeArgs = checker.getTypeArguments(type as ts.TypeReference);
    if (typeArgs.length > 0) {
      const elementType = expandType(typeArgs[0], checker, new Set(visited), depth + 1);
      // Wrap complex element types in parentheses
      if (elementType.includes(' | ') || elementType.includes(' & ') || elementType.includes('=>')) {
        return `(${elementType})[]`;
      }
      return `${elementType}[]`;
    }
    return 'unknown[]';
  }

  // Handle tuple types
  if (checker.isTupleType(type)) {
    const typeRef = type as ts.TypeReference;
    const target = typeRef.target as ts.TupleType;
    const typeArgs = checker.getTypeArguments(typeRef);
    
    // Check for readonly modifier
    const isReadonly = target.readonly ?? false;
    
    // Get element flags for optional detection
    const elementFlags = target.elementFlags ?? [];
    
    const elements: string[] = [];
    for (let i = 0; i < typeArgs.length; i++) {
      const elemType = expandType(typeArgs[i], checker, new Set(visited), depth + 1);
      // ts.ElementFlags.Optional = 2
      const isOptional = (elementFlags[i] ?? 0) & 2;
      if (isOptional) {
        // For optional elements, show as T? instead of T | undefined
        const baseType = elemType.replace(/ \| undefined$/, '').replace(/^undefined \| /, '');
        elements.push(`${baseType}?`);
      } else {
        elements.push(elemType);
      }
    }
    
    const tupleStr = `[${elements.join(', ')}]`;
    return isReadonly ? `readonly ${tupleStr}` : tupleStr;
  }

  // Handle function types
  const callSignatures = type.getCallSignatures();
  if (callSignatures.length > 0) {
    const sig = callSignatures[0];
    const params = sig.getParameters().map((p: ts.Symbol) => {
      const paramType = checker.getTypeOfSymbol(p);
      const expandedType = expandType(paramType, checker, new Set(visited), depth + 1);
      return `${p.name}: ${expandedType}`;
    });
    const returnType = expandType(checker.getReturnTypeOfSignature(sig), checker, new Set(visited), depth + 1);
    return `(${params.join(', ')}) => ${returnType}`;
  }

  // Handle object types - expand all properties
  if (type.flags & ts.TypeFlags.Object) {
    const properties = type.getProperties();
    if (properties.length === 0) {
      // Check for index signatures
      const stringIndexType = type.getStringIndexType();
      const numberIndexType = type.getNumberIndexType();
      if (stringIndexType) {
        const indexType = expandType(stringIndexType, checker, new Set(visited), depth + 1);
        return `{ [key: string]: ${indexType}; }`;
      }
      if (numberIndexType) {
        const indexType = expandType(numberIndexType, checker, new Set(visited), depth + 1);
        return `{ [index: number]: ${indexType}; }`;
      }
      return '{}';
    }

    const propStrs: string[] = [];
    for (const prop of properties) {
      const propType = checker.getTypeOfSymbol(prop);
      const expandedPropType = expandType(propType, checker, new Set(visited), depth + 1);
      const isOptional = !!(prop.flags & ts.SymbolFlags.Optional);
      const optionalMark = isOptional ? '?' : '';
      propStrs.push(`${prop.name}${optionalMark}: ${expandedPropType};`);
    }

    return `{ ${propStrs.join(' ')} }`;
  }

  // Fallback
  return checker.typeToString(type);
}

/**
 * Format a type string for better readability with multiline support
 */
function formatTypeString(typeStr: string, type: ts.Type, checker: ts.TypeChecker): string {
  // Use our custom expansion for full type expansion
  const expanded = expandType(type, checker);

  // Normalize Array<T> to T[]
  let result = expanded.replace(/Array<([^<>]+)>/g, '$1[]');
  while (result.includes('Array<')) {
    result = result.replace(/Array<([^<>]+)>/g, '$1[]');
  }

  return result;
}

/**
 * Analyze the type and return structured result
 */
function analyzeTypeString(typeStr: string, type: ts.Type, checker: ts.TypeChecker): InferenceResult {
  // Check for literal types
  if (type.isStringLiteral()) {
    return { type: `"${type.value}"`, kind: 'literal' };
  }
  if (type.isNumberLiteral()) {
    return { type: String(type.value), kind: 'literal' };
  }
  if (type.flags & ts.TypeFlags.BooleanLiteral) {
    // BooleanLiteral is either true or false
    const intrinsicName = (type as ts.IntrinsicType).intrinsicName;
    return { type: intrinsicName, kind: 'literal' };
  }
  if (type.flags & ts.TypeFlags.BigIntLiteral) {
    return { type: typeStr, kind: 'literal' };
  }

  // Check for primitive types
  if (type.flags & ts.TypeFlags.String) {
    return { type: 'string', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Number) {
    return { type: 'number', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Boolean) {
    return { type: 'boolean', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.BigInt) {
    return { type: 'bigint', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Null) {
    return { type: 'null', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Undefined) {
    return { type: 'undefined', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Void) {
    return { type: 'void', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Never) {
    return { type: 'never', kind: 'primitive' };
  }
  if (type.flags & ts.TypeFlags.Any) {
    return { type: 'any', kind: 'any' };
  }
  if (type.flags & ts.TypeFlags.Unknown) {
    return { type: 'unknown', kind: 'any' };
  }

  // Check for union types
  if (type.isUnion()) {
    return { type: typeStr, kind: 'union' };
  }

  // Check for array types
  if (checker.isArrayType(type)) {
    const typeArgs = checker.getTypeArguments(type as ts.TypeReference);
    const elementType = typeArgs.length > 0
      ? checker.typeToString(typeArgs[0], undefined, ts.TypeFormatFlags.NoTruncation | ts.TypeFormatFlags.InTypeAlias)
      : 'unknown';
    return { type: typeStr, kind: 'array', elementType };
  }

  // Check for function/callable types
  const callSignatures = type.getCallSignatures();
  if (callSignatures.length > 0) {
    const sig = callSignatures[0];
    const returnType = checker.typeToString(
      checker.getReturnTypeOfSignature(sig),
      undefined,
      ts.TypeFormatFlags.NoTruncation | ts.TypeFormatFlags.InTypeAlias
    );
    return { type: typeStr, kind: 'function', returnType };
  }

  // Check for object types
  if (type.flags & ts.TypeFlags.Object) {
    const properties = extractProperties(type, checker);
    return { type: typeStr, kind: 'object', properties };
  }

  // Default to object for complex types
  return { type: typeStr, kind: 'object' };
}

/**
 * Extract properties from an object type with full expansion
 */
function extractProperties(type: ts.Type, checker: ts.TypeChecker): Array<{ name: string; type: string; optional: boolean }> {
  const properties: Array<{ name: string; type: string; optional: boolean }> = [];

  for (const prop of type.getProperties()) {
    const propType = checker.getTypeOfSymbol(prop);
    // Use expandType for full expansion
    const propTypeStr = expandType(propType, checker);
    const isOptional = !!(prop.flags & ts.SymbolFlags.Optional);

    properties.push({
      name: prop.name,
      type: propTypeStr,
      optional: isOptional,
    });
  }

  return properties;
}

function sendResponse(response: RpcResponse): void {
  console.log(JSON.stringify(response));
}

async function handleRequest(request: RpcRequest): Promise<void> {
  const response: RpcResponse = { id: request.id };

  try {
    switch (request.method) {
      case 'ping':
        response.result = { type: 'pong', kind: 'primitive' };
        break;

      case 'infer':
        if (!request.code) {
          response.error = { message: 'No code provided' };
        } else {
          response.result = inferDefaultExportType(request.code);
        }
        break;

      default:
        response.error = { message: `Unknown method: ${request.method}` };
    }
  } catch (err) {
    response.error = {
      message: err instanceof Error ? err.message : String(err),
      stack: err instanceof Error ? err.stack : undefined,
    };
  }

  sendResponse(response);
}

async function main(): Promise<void> {
  const decoder = new TextDecoder();
  let buffer = '';

  for await (const chunk of Bun.stdin.stream()) {
    buffer += decoder.decode(chunk, { stream: true });

    let newlineIdx: number;
    while ((newlineIdx = buffer.indexOf('\n')) !== -1) {
      const line = buffer.slice(0, newlineIdx);
      buffer = buffer.slice(newlineIdx + 1);

      if (line.trim()) {
        try {
          const request = JSON.parse(line) as RpcRequest;
          await handleRequest(request);
        } catch {
          sendResponse({
            id: 'error',
            error: { message: 'Failed to parse request' },
          });
        }
      }
    }
  }
}

main().catch((err) => {
  console.error('Worker error:', err);
  process.exit(1);
});
