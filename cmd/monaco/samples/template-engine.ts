// Context: Template helpers and formatters

export const defaultHelpers = {
  upper: (s: string) => s.toUpperCase(),
  lower: (s: string) => s.toLowerCase(),
  capitalize: (s: string) => s.charAt(0).toUpperCase() + s.slice(1),
  trim: (s: string) => s.trim(),
  currency: (n: number) => `$${n.toFixed(2)}`,
  date: (d: string) => new Date(d).toLocaleDateString(),
  pluralize: (count: number, singular: string, plural?: string) => 
    count === 1 ? singular : (plural || singular + 's'),
};

export function escapeHtml(str: string): string {
  const escapes: Record<string, string> = {
    '&': '&amp;', '<': '&lt;', '>': '&gt;',
    '"': '&quot;', "'": '&#39;',
  };
  return str.replace(/[&<>"']/g, c => escapes[c]);
}

// --- Code ---

// Template Engine - String interpolation with helpers and conditionals
// Demonstrates regex parsing, recursive evaluation, and DSL design

interface TemplateContext {
  [key: string]: unknown;
}

interface RenderResult {
  success: boolean;
  output?: string;
  error?: string;
  variables: string[];
  helpers: string[];
}

type HelperFunction = (...args: unknown[]) => unknown;

class TemplateEngine {
  private helpers: Record<string, HelperFunction> = {};
  private cache: Map<string, ParsedTemplate> = new Map();

  constructor() {
    // Register default helpers
    for (const [name, fn] of Object.entries(defaultHelpers)) {
      this.helpers[name] = fn as HelperFunction;
    }
  }

  // Register a custom helper
  registerHelper(name: string, fn: HelperFunction): void {
    this.helpers[name] = fn;
  }

  // Render a template with context
  render(template: string, context: TemplateContext): RenderResult {
    const variables: string[] = [];
    const helpers: string[] = [];

    try {
      // Get or parse template
      let parsed = this.cache.get(template);
      if (!parsed) {
        parsed = this.parse(template);
        this.cache.set(template, parsed);
      }

      // Render each segment
      const output = parsed.segments.map(segment => {
        switch (segment.type) {
          case 'text':
            return segment.content;

          case 'variable': {
            variables.push(segment.content!);
            const value = this.resolvePath(segment.content!, context);
            return String(value ?? '');
          }

          case 'escaped': {
            variables.push(segment.content!);
            const value = this.resolvePath(segment.content!, context);
            return escapeHtml(String(value ?? ''));
          }

          case 'helper': {
            helpers.push(segment.helper!);
            const fn = this.helpers[segment.helper!];
            if (!fn) throw new Error(`Unknown helper: ${segment.helper}`);
            
            const args = segment.args!.map(arg => {
              if (arg.startsWith('"') || arg.startsWith("'")) {
                return arg.slice(1, -1);
              }
              if (!isNaN(Number(arg))) {
                return Number(arg);
              }
              variables.push(arg);
              return this.resolvePath(arg, context);
            });
            
            return String(fn(...args));
          }

          case 'conditional': {
            variables.push(segment.condition!);
            const value = this.resolvePath(segment.condition!, context);
            const truthy = this.isTruthy(value);
            
            if (truthy) {
              return segment.trueBranch 
                ? this.render(segment.trueBranch, context).output || ''
                : '';
            } else {
              return segment.falseBranch
                ? this.render(segment.falseBranch, context).output || ''
                : '';
            }
          }

          case 'loop': {
            variables.push(segment.collection!);
            const collection = this.resolvePath(segment.collection!, context);
            if (!Array.isArray(collection)) return '';

            return collection.map((item, index) => {
              const loopContext: TemplateContext = {
                ...context,
                [segment.itemVar!]: item,
                [`${segment.itemVar!}Index`]: index,
                [`${segment.itemVar!}First`]: index === 0,
                [`${segment.itemVar!}Last`]: index === collection.length - 1,
              };
              return this.render(segment.body!, loopContext).output || '';
            }).join('');
          }
        }
        return '';
      }).join('');

      return {
        success: true,
        output,
        variables: [...new Set(variables)],
        helpers: [...new Set(helpers)],
      };
    } catch (err) {
      return {
        success: false,
        error: (err as Error).message,
        variables,
        helpers,
      };
    }
  }

  private parse(template: string): ParsedTemplate {
    const segments: TemplateSegment[] = [];
    let pos = 0;

    while (pos < template.length) {
      // Look for template syntax
      const doubleOpen = template.indexOf('{{', pos);
      
      if (doubleOpen === -1) {
        // No more templates, rest is text
        segments.push({ type: 'text', content: template.slice(pos) });
        break;
      }

      // Add text before template
      if (doubleOpen > pos) {
        segments.push({ type: 'text', content: template.slice(pos, doubleOpen) });
      }

      // Parse template expression
      const closePos = template.indexOf('}}', doubleOpen);
      if (closePos === -1) {
        throw new Error('Unclosed template expression');
      }

      const expr = template.slice(doubleOpen + 2, closePos).trim();
      pos = closePos + 2;

      // Escaped output {{{var}}}
      if (expr.startsWith('{') && template[closePos + 2] === '}') {
        const inner = expr.slice(1);
        segments.push({ type: 'escaped', content: inner });
        pos++;
        continue;
      }

      // Conditional {{#if condition}}...{{else}}...{{/if}}
      if (expr.startsWith('#if ')) {
        const condition = expr.slice(4).trim();
        const { trueBranch, falseBranch, endPos } = this.parseConditional(
          template, pos
        );
        segments.push({
          type: 'conditional',
          condition,
          trueBranch,
          falseBranch,
        });
        pos = endPos;
        continue;
      }

      // Loop {{#each collection as item}}...{{/each}}
      if (expr.startsWith('#each ')) {
        const match = expr.match(/^#each\s+(\S+)\s+as\s+(\S+)$/);
        if (!match) throw new Error(`Invalid each syntax: ${expr}`);
        const [, collection, itemVar] = match;
        const { body, endPos } = this.parseLoop(template, pos);
        segments.push({
          type: 'loop',
          collection,
          itemVar,
          body,
        });
        pos = endPos;
        continue;
      }

      // Helper {{helper arg1 arg2}}
      const helperMatch = expr.match(/^(\w+)\s+(.+)$/);
      if (helperMatch && this.helpers[helperMatch[1]]) {
        const [, helper, argsStr] = helperMatch;
        const args = this.parseArgs(argsStr);
        segments.push({ type: 'helper', helper, args });
        continue;
      }

      // Simple variable {{var}}
      segments.push({ type: 'variable', content: expr });
    }

    return { segments };
  }

  private parseConditional(
    template: string, 
    startPos: number
  ): { trueBranch: string; falseBranch?: string; endPos: number } {
    let depth = 1;
    let pos = startPos;
    let elsePos = -1;

    while (depth > 0 && pos < template.length) {
      const nextIf = template.indexOf('{{#if', pos);
      const nextElse = template.indexOf('{{else}}', pos);
      const nextEnd = template.indexOf('{{/if}}', pos);

      // Find next relevant marker
      const positions = [
        nextIf >= 0 ? nextIf : Infinity,
        nextElse >= 0 ? nextElse : Infinity,
        nextEnd >= 0 ? nextEnd : Infinity,
      ];
      const minPos = Math.min(...positions);

      if (minPos === Infinity) throw new Error('Unclosed #if');

      if (minPos === nextIf) {
        depth++;
        pos = nextIf + 5;
      } else if (minPos === nextElse && depth === 1) {
        elsePos = nextElse;
        pos = nextElse + 8;
      } else if (minPos === nextEnd) {
        depth--;
        if (depth === 0) {
          const trueBranch = elsePos >= 0
            ? template.slice(startPos, elsePos)
            : template.slice(startPos, nextEnd);
          const falseBranch = elsePos >= 0
            ? template.slice(elsePos + 8, nextEnd)
            : undefined;
          return { trueBranch, falseBranch, endPos: nextEnd + 7 };
        }
        pos = nextEnd + 7;
      }
    }

    throw new Error('Unclosed #if');
  }

  private parseLoop(
    template: string, 
    startPos: number
  ): { body: string; endPos: number } {
    const endTag = '{{/each}}';
    const endPos = template.indexOf(endTag, startPos);
    if (endPos === -1) throw new Error('Unclosed #each');

    return {
      body: template.slice(startPos, endPos),
      endPos: endPos + endTag.length,
    };
  }

  private parseArgs(argsStr: string): string[] {
    const args: string[] = [];
    let current = '';
    let inQuote = false;
    let quoteChar = '';

    for (const char of argsStr) {
      if ((char === '"' || char === "'") && !inQuote) {
        inQuote = true;
        quoteChar = char;
        current += char;
      } else if (char === quoteChar && inQuote) {
        inQuote = false;
        current += char;
        args.push(current);
        current = '';
      } else if (char === ' ' && !inQuote) {
        if (current) args.push(current);
        current = '';
      } else {
        current += char;
      }
    }
    if (current) args.push(current);

    return args;
  }

  private resolvePath(path: string, context: TemplateContext): unknown {
    const parts = path.split('.');
    let current: unknown = context;

    for (const part of parts) {
      if (current == null) return undefined;
      current = (current as Record<string, unknown>)[part];
    }

    return current;
  }

  private isTruthy(value: unknown): boolean {
    if (Array.isArray(value)) return value.length > 0;
    return Boolean(value);
  }
}

interface TemplateSegment {
  type: 'text' | 'variable' | 'escaped' | 'helper' | 'conditional' | 'loop';
  content?: string;
  helper?: string;
  args?: string[];
  condition?: string;
  trueBranch?: string;
  falseBranch?: string;
  collection?: string;
  itemVar?: string;
  body?: string;
}

interface ParsedTemplate {
  segments: TemplateSegment[];
}

// Create engine and add custom helper
const engine = new TemplateEngine();

engine.registerHelper('repeat', (...args: unknown[]) => {
  const [str, times] = args as [string, number];
  return String(str).repeat(times);
});

engine.registerHelper('join', (...args: unknown[]) => {
  const [arr, sep] = args as [unknown[], string];
  return Array.isArray(arr) ? arr.join(sep) : String(arr);
});

// Test data
const context: TemplateContext = {
  user: {
    name: 'Alice',
    email: 'alice@example.com',
    role: 'admin',
    active: true,
  },
  order: {
    id: 'ORD-12345',
    total: 149.99,
    date: '2025-01-04',
    items: [
      { name: 'Widget', quantity: 2, price: 49.99 },
      { name: 'Gadget', quantity: 1, price: 50.01 },
    ],
  },
  notifications: ['Welcome!', 'New feature available', 'Special offer'],
  htmlContent: '<script>alert("XSS")</script>',
};

// Test templates
const templates = [
  {
    name: 'Simple interpolation',
    template: 'Hello, {{user.name}}! Your email is {{user.email}}.',
  },
  {
    name: 'With helpers',
    template: 'Hi, {{upper user.name}}! Order {{order.id}} total: {{currency order.total}}',
  },
  {
    name: 'Conditional',
    template: '{{#if user.active}}Account active{{else}}Account inactive{{/if}}',
  },
  {
    name: 'Loop',
    template: 'Items: {{#each order.items as item}}{{item.name}} x{{item.quantity}}; {{/each}}',
  },
  {
    name: 'Escaped HTML',
    template: 'Safe: {{{htmlContent}}}',
  },
  {
    name: 'Complex template',
    template: `Order {{order.id}}
{{#if user.active}}
Welcome back, {{capitalize user.name}}!
{{else}}
Please activate your account.
{{/if}}
Items ({{order.items.length}}):
{{#each order.items as item}}
  - {{item.name}}: {{currency item.price}} x {{item.quantity}}
{{/each}}
Total: {{currency order.total}}`,
  },
];

// Render all templates
const results = templates.map(({ name, template }) => ({
  name,
  template,
  result: engine.render(template, context),
}));

// Define result type for clear contract
interface TemplateTestResult {
  name: string;
  output: string | undefined;
  success: boolean;
  usedVariables: string[];
  usedHelpers: string[];
}

interface UserInfo {
  name: string;
  email: string;
  role: string;
  active: boolean;
}

interface PlaygroundResult {
  availableHelpers: string[];
  context: {
    userName: UserInfo;
    orderItemCount: number;
  };
  results: TemplateTestResult[];
}

const result: PlaygroundResult = {
  availableHelpers: Object.keys(defaultHelpers).concat(['repeat', 'join']),
  context: {
    userName: context.user as UserInfo,
    orderItemCount: (context.order as Record<string, unknown[]>).items.length,
  },
  results: results.map(r => ({
    name: r.name,
    output: r.result.output,
    success: r.result.success,
    usedVariables: r.result.variables,
    usedHelpers: r.result.helpers,
  })),
};

export default result;
