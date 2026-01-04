// Context: Math and utility functions for expressions

export const constants: Record<string, number> = {
  PI: Math.PI,
  E: Math.E,
  TAU: Math.PI * 2,
  PHI: 1.618033988749895, // Golden ratio
};

export const functions: Record<string, (...args: number[]) => number> = {
  sin: Math.sin,
  cos: Math.cos,
  tan: Math.tan,
  sqrt: Math.sqrt,
  abs: Math.abs,
  floor: Math.floor,
  ceil: Math.ceil,
  round: Math.round,
  log: Math.log,
  log10: Math.log10,
  pow: Math.pow,
  min: Math.min,
  max: Math.max,
};

// --- Code ---

// Expression Parser - Mini expression evaluator with variables and functions
// Demonstrates lexical analysis, parsing, and AST evaluation

type TokenType = 
  | 'NUMBER' | 'IDENTIFIER' | 'OPERATOR' | 'LPAREN' | 'RPAREN' 
  | 'COMMA' | 'EOF';

interface Token {
  type: TokenType;
  value: string;
  position: number;
}

type ASTNode = 
  | { kind: 'number'; value: number }
  | { kind: 'variable'; name: string }
  | { kind: 'binary'; operator: string; left: ASTNode; right: ASTNode }
  | { kind: 'unary'; operator: string; operand: ASTNode }
  | { kind: 'call'; name: string; args: ASTNode[] };

interface ParseResult {
  success: boolean;
  ast?: ASTNode;
  error?: string;
  tokens?: Token[];
}

interface EvalResult {
  success: boolean;
  value?: number;
  error?: string;
  steps?: string[];
}

// Tokenizer
function tokenize(input: string): Token[] {
  const tokens: Token[] = [];
  let pos = 0;

  while (pos < input.length) {
    const char = input[pos];

    // Skip whitespace
    if (/\s/.test(char)) {
      pos++;
      continue;
    }

    // Numbers (including decimals)
    if (/\d/.test(char)) {
      let num = '';
      const start = pos;
      while (pos < input.length && /[\d.]/.test(input[pos])) {
        num += input[pos++];
      }
      tokens.push({ type: 'NUMBER', value: num, position: start });
      continue;
    }

    // Identifiers (variables and functions)
    if (/[a-zA-Z_]/.test(char)) {
      let id = '';
      const start = pos;
      while (pos < input.length && /[a-zA-Z0-9_]/.test(input[pos])) {
        id += input[pos++];
      }
      tokens.push({ type: 'IDENTIFIER', value: id, position: start });
      continue;
    }

    // Operators
    if ('+-*/^%'.includes(char)) {
      tokens.push({ type: 'OPERATOR', value: char, position: pos++ });
      continue;
    }

    // Parentheses
    if (char === '(') {
      tokens.push({ type: 'LPAREN', value: char, position: pos++ });
      continue;
    }
    if (char === ')') {
      tokens.push({ type: 'RPAREN', value: char, position: pos++ });
      continue;
    }

    // Comma (for function arguments)
    if (char === ',') {
      tokens.push({ type: 'COMMA', value: char, position: pos++ });
      continue;
    }

    throw new Error(`Unknown character '${char}' at position ${pos}`);
  }

  tokens.push({ type: 'EOF', value: '', position: pos });
  return tokens;
}

// Parser (recursive descent)
class Parser {
  private tokens: Token[];
  private pos = 0;

  constructor(tokens: Token[]) {
    this.tokens = tokens;
  }

  private current(): Token {
    return this.tokens[this.pos];
  }

  private consume(type?: TokenType): Token {
    const token = this.current();
    if (type && token.type !== type) {
      throw new Error(`Expected ${type}, got ${token.type}`);
    }
    this.pos++;
    return token;
  }

  parse(): ASTNode {
    const result = this.parseExpression();
    if (this.current().type !== 'EOF') {
      throw new Error(`Unexpected token: ${this.current().value}`);
    }
    return result;
  }

  // expression = term (('+' | '-') term)*
  private parseExpression(): ASTNode {
    let left = this.parseTerm();

    while (this.current().type === 'OPERATOR' && 
           '+-'.includes(this.current().value)) {
      const op = this.consume().value;
      const right = this.parseTerm();
      left = { kind: 'binary', operator: op, left, right };
    }

    return left;
  }

  // term = factor (('*' | '/' | '%') factor)*
  private parseTerm(): ASTNode {
    let left = this.parsePower();

    while (this.current().type === 'OPERATOR' && 
           '*/%'.includes(this.current().value)) {
      const op = this.consume().value;
      const right = this.parsePower();
      left = { kind: 'binary', operator: op, left, right };
    }

    return left;
  }

  // power = unary ('^' power)?  (right associative)
  private parsePower(): ASTNode {
    let base = this.parseUnary();

    if (this.current().type === 'OPERATOR' && this.current().value === '^') {
      this.consume();
      const exp = this.parsePower(); // Right associative
      base = { kind: 'binary', operator: '^', left: base, right: exp };
    }

    return base;
  }

  // unary = ('-' | '+') unary | primary
  private parseUnary(): ASTNode {
    if (this.current().type === 'OPERATOR' && 
        '+-'.includes(this.current().value)) {
      const op = this.consume().value;
      const operand = this.parseUnary();
      if (op === '+') return operand;
      return { kind: 'unary', operator: '-', operand };
    }
    return this.parsePrimary();
  }

  // primary = NUMBER | IDENTIFIER | IDENTIFIER '(' args ')' | '(' expression ')'
  private parsePrimary(): ASTNode {
    const token = this.current();

    if (token.type === 'NUMBER') {
      this.consume();
      return { kind: 'number', value: parseFloat(token.value) };
    }

    if (token.type === 'IDENTIFIER') {
      this.consume();
      
      // Check if it's a function call
      if (this.current().type === 'LPAREN') {
        this.consume('LPAREN');
        const args: ASTNode[] = [];
        
        if (this.current().type !== 'RPAREN') {
          args.push(this.parseExpression());
          while (this.current().type === 'COMMA') {
            this.consume();
            args.push(this.parseExpression());
          }
        }
        
        this.consume('RPAREN');
        return { kind: 'call', name: token.value, args };
      }

      // It's a variable
      return { kind: 'variable', name: token.value };
    }

    if (token.type === 'LPAREN') {
      this.consume('LPAREN');
      const expr = this.parseExpression();
      this.consume('RPAREN');
      return expr;
    }

    throw new Error(`Unexpected token: ${token.value || token.type}`);
  }
}

// Evaluator
function evaluate(
  node: ASTNode, 
  variables: Record<string, number>,
  steps: string[]
): number {
  switch (node.kind) {
    case 'number':
      return node.value;

    case 'variable': {
      const value = variables[node.name] ?? constants[node.name];
      if (value === undefined) {
        throw new Error(`Unknown variable: ${node.name}`);
      }
      steps.push(`${node.name} = ${value}`);
      return value;
    }

    case 'unary': {
      const operand = evaluate(node.operand, variables, steps);
      const result = -operand;
      steps.push(`-${operand} = ${result}`);
      return result;
    }

    case 'binary': {
      const left = evaluate(node.left, variables, steps);
      const right = evaluate(node.right, variables, steps);
      let result: number;

      switch (node.operator) {
        case '+': result = left + right; break;
        case '-': result = left - right; break;
        case '*': result = left * right; break;
        case '/': 
          if (right === 0) throw new Error('Division by zero');
          result = left / right; 
          break;
        case '%': result = left % right; break;
        case '^': result = Math.pow(left, right); break;
        default: throw new Error(`Unknown operator: ${node.operator}`);
      }

      steps.push(`${left} ${node.operator} ${right} = ${result}`);
      return result;
    }

    case 'call': {
      const fn = functions[node.name];
      if (!fn) {
        throw new Error(`Unknown function: ${node.name}`);
      }
      const args = node.args.map(arg => evaluate(arg, variables, steps));
      const result = fn(...args);
      steps.push(`${node.name}(${args.join(', ')}) = ${result}`);
      return result;
    }
  }
}

// Main API
function parseAndEvaluate(
  expression: string, 
  variables: Record<string, number> = {}
): { parse: ParseResult; eval: EvalResult } {
  let parseResult: ParseResult;
  let evalResult: EvalResult;

  try {
    const tokens = tokenize(expression);
    const parser = new Parser(tokens);
    const ast = parser.parse();
    parseResult = { success: true, ast, tokens };

    try {
      const steps: string[] = [];
      const value = evaluate(ast, variables, steps);
      evalResult = { success: true, value, steps };
    } catch (e) {
      evalResult = { success: false, error: (e as Error).message };
    }
  } catch (e) {
    parseResult = { success: false, error: (e as Error).message };
    evalResult = { success: false, error: 'Parse failed' };
  }

  return { parse: parseResult, eval: evalResult };
}

// Test cases
const testCases: Array<{ expr: string; vars: Record<string, number>; expected: number }> = [
  { expr: '2 + 3 * 4', vars: {}, expected: 14 },
  { expr: '(2 + 3) * 4', vars: {}, expected: 20 },
  { expr: 'x^2 + y^2', vars: { x: 3, y: 4 }, expected: 25 },
  { expr: 'sqrt(x^2 + y^2)', vars: { x: 3, y: 4 }, expected: 5 },
  { expr: '2 * PI * r', vars: { r: 10 }, expected: 2 * Math.PI * 10 },
  { expr: 'sin(PI / 2)', vars: {}, expected: 1 },
  { expr: 'max(a, b, c)', vars: { a: 5, b: 12, c: 8 }, expected: 12 },
  { expr: '-(-5)', vars: {}, expected: 5 },
  { expr: '2^3^2', vars: {}, expected: 512 }, // Right associative: 2^(3^2) = 2^9
];

const results = testCases.map(({ expr, vars, expected }) => {
  const result = parseAndEvaluate(expr, vars);
  return {
    expression: expr,
    variables: Object.keys(vars).length > 0 ? vars : undefined,
    expected,
    actual: result.eval.value,
    passed: result.eval.success && 
            Math.abs((result.eval.value || 0) - expected) < 0.0001,
    steps: result.eval.steps,
  };
});

export default {
  availableConstants: Object.keys(constants),
  availableFunctions: Object.keys(functions),
  testResults: results,
  summary: {
    total: results.length,
    passed: results.filter(r => r.passed).length,
    failed: results.filter(r => !r.passed).length,
  },
};
