// Context: Schema registry for validation

interface SchemaDefinition {
  name: string;
  version: string;
  strict: boolean;
}

export const schemas: Record<string, SchemaDefinition> = {
  'user': { name: 'User Schema', version: '1.0', strict: true },
  'product': { name: 'Product Schema', version: '2.1', strict: false },
  'order': { name: 'Order Schema', version: '1.5', strict: true },
};

export function getSchemaInfo(name: string): SchemaDefinition | undefined {
  return schemas[name];
}

export function formatValidationPath(path: string[]): string {
  return path.length > 0 ? path.join('.') : 'root';
}

// --- Code ---

// JSON Schema Validator - Complex type validation with custom rules
// Demonstrates recursive validation and type guards

type SchemaType = 
  | { type: 'string'; minLength?: number; maxLength?: number; pattern?: string }
  | { type: 'number'; min?: number; max?: number; integer?: boolean }
  | { type: 'boolean' }
  | { type: 'array'; items: SchemaType; minItems?: number; maxItems?: number }
  | { type: 'object'; properties: Record<string, SchemaType>; required?: string[] }
  | { type: 'union'; variants: SchemaType[] }
  | { type: 'literal'; value: string | number | boolean }
  | { type: 'null' };

interface ValidationError {
  path: string;
  message: string;
  expected: string;
  received: string;
}

interface ValidationResult {
  valid: boolean;
  errors: ValidationError[];
  checkedPaths: number;
}

// Validate a value against a schema
function validate(
  value: unknown, 
  schema: SchemaType, 
  path: string[] = []
): ValidationResult {
  const errors: ValidationError[] = [];
  let checkedPaths = 1;

  const currentPath = formatValidationPath(path);

  switch (schema.type) {
    case 'string': {
      if (typeof value !== 'string') {
        errors.push({
          path: currentPath,
          message: 'Expected string',
          expected: 'string',
          received: typeof value,
        });
      } else {
        if (schema.minLength !== undefined && value.length < schema.minLength) {
          errors.push({
            path: currentPath,
            message: `String too short (min: ${schema.minLength})`,
            expected: `length >= ${schema.minLength}`,
            received: `length ${value.length}`,
          });
        }
        if (schema.maxLength !== undefined && value.length > schema.maxLength) {
          errors.push({
            path: currentPath,
            message: `String too long (max: ${schema.maxLength})`,
            expected: `length <= ${schema.maxLength}`,
            received: `length ${value.length}`,
          });
        }
        if (schema.pattern !== undefined && !new RegExp(schema.pattern).test(value)) {
          errors.push({
            path: currentPath,
            message: `String doesn't match pattern`,
            expected: `/${schema.pattern}/`,
            received: value,
          });
        }
      }
      break;
    }

    case 'number': {
      if (typeof value !== 'number' || isNaN(value)) {
        errors.push({
          path: currentPath,
          message: 'Expected number',
          expected: 'number',
          received: typeof value,
        });
      } else {
        if (schema.integer && !Number.isInteger(value)) {
          errors.push({
            path: currentPath,
            message: 'Expected integer',
            expected: 'integer',
            received: 'float',
          });
        }
        if (schema.min !== undefined && value < schema.min) {
          errors.push({
            path: currentPath,
            message: `Number too small (min: ${schema.min})`,
            expected: `>= ${schema.min}`,
            received: String(value),
          });
        }
        if (schema.max !== undefined && value > schema.max) {
          errors.push({
            path: currentPath,
            message: `Number too large (max: ${schema.max})`,
            expected: `<= ${schema.max}`,
            received: String(value),
          });
        }
      }
      break;
    }

    case 'boolean': {
      if (typeof value !== 'boolean') {
        errors.push({
          path: currentPath,
          message: 'Expected boolean',
          expected: 'boolean',
          received: typeof value,
        });
      }
      break;
    }

    case 'null': {
      if (value !== null) {
        errors.push({
          path: currentPath,
          message: 'Expected null',
          expected: 'null',
          received: typeof value,
        });
      }
      break;
    }

    case 'literal': {
      if (value !== schema.value) {
        errors.push({
          path: currentPath,
          message: `Expected literal ${JSON.stringify(schema.value)}`,
          expected: JSON.stringify(schema.value),
          received: JSON.stringify(value),
        });
      }
      break;
    }

    case 'array': {
      if (!Array.isArray(value)) {
        errors.push({
          path: currentPath,
          message: 'Expected array',
          expected: 'array',
          received: typeof value,
        });
      } else {
        if (schema.minItems !== undefined && value.length < schema.minItems) {
          errors.push({
            path: currentPath,
            message: `Array too short (min: ${schema.minItems})`,
            expected: `length >= ${schema.minItems}`,
            received: `length ${value.length}`,
          });
        }
        if (schema.maxItems !== undefined && value.length > schema.maxItems) {
          errors.push({
            path: currentPath,
            message: `Array too long (max: ${schema.maxItems})`,
            expected: `length <= ${schema.maxItems}`,
            received: `length ${value.length}`,
          });
        }
        // Validate each item
        for (let i = 0; i < value.length; i++) {
          const itemResult = validate(value[i], schema.items, [...path, String(i)]);
          errors.push(...itemResult.errors);
          checkedPaths += itemResult.checkedPaths;
        }
      }
      break;
    }

    case 'object': {
      if (typeof value !== 'object' || value === null || Array.isArray(value)) {
        errors.push({
          path: currentPath,
          message: 'Expected object',
          expected: 'object',
          received: Array.isArray(value) ? 'array' : typeof value,
        });
      } else {
        const obj = value as Record<string, unknown>;
        
        // Check required fields
        if (schema.required) {
          for (const key of schema.required) {
            if (!(key in obj)) {
              errors.push({
                path: formatValidationPath([...path, key]),
                message: `Missing required field '${key}'`,
                expected: 'present',
                received: 'missing',
              });
            }
          }
        }

        // Validate each property
        for (const [key, propSchema] of Object.entries(schema.properties)) {
          if (key in obj) {
            const propResult = validate(obj[key], propSchema, [...path, key]);
            errors.push(...propResult.errors);
            checkedPaths += propResult.checkedPaths;
          }
        }
      }
      break;
    }

    case 'union': {
      // Try each variant until one succeeds
      let anyValid = false;
      const variantErrors: ValidationError[][] = [];

      for (const variant of schema.variants) {
        const variantResult = validate(value, variant, path);
        if (variantResult.valid) {
          anyValid = true;
          checkedPaths += variantResult.checkedPaths;
          break;
        }
        variantErrors.push(variantResult.errors);
      }

      if (!anyValid) {
        errors.push({
          path: currentPath,
          message: 'Value doesn\'t match any variant in union',
          expected: schema.variants.map(v => v.type).join(' | '),
          received: typeof value,
        });
      }
      break;
    }
  }

  return {
    valid: errors.length === 0,
    errors,
    checkedPaths,
  };
}

// Define a user registration schema
const userSchema: SchemaType = {
  type: 'object',
  properties: {
    username: { type: 'string', minLength: 3, maxLength: 20, pattern: '^[a-z0-9_]+$' },
    email: { type: 'string', pattern: '^[^@]+@[^@]+\\.[^@]+$' },
    age: { type: 'number', min: 13, max: 120, integer: true },
    role: { 
      type: 'union', 
      variants: [
        { type: 'literal', value: 'admin' },
        { type: 'literal', value: 'user' },
        { type: 'literal', value: 'guest' },
      ]
    },
    preferences: {
      type: 'object',
      properties: {
        theme: { type: 'string' },
        notifications: { type: 'boolean' },
        tags: { type: 'array', items: { type: 'string' }, maxItems: 5 },
      },
      required: ['notifications'],
    },
  },
  required: ['username', 'email', 'age', 'role'],
};

// Test with valid data
const validUser = {
  username: 'john_doe',
  email: 'john@example.com',
  age: 28,
  role: 'user',
  preferences: {
    theme: 'dark',
    notifications: true,
    tags: ['tech', 'sports'],
  },
};

// Test with invalid data
const invalidUser = {
  username: 'JD',  // Too short, has uppercase
  email: 'invalid-email',  // Invalid format
  age: 10.5,  // Too young, not integer
  role: 'superuser',  // Invalid role
  preferences: {
    theme: 123,  // Wrong type
    // notifications missing (required)
    tags: ['a', 'b', 'c', 'd', 'e', 'f'],  // Too many
  },
};

// Run validations
const validResult = validate(validUser, userSchema);
const invalidResult = validate(invalidUser, userSchema);

export default {
  schema: getSchemaInfo('user'),
  validUser: {
    data: validUser,
    result: validResult,
  },
  invalidUser: {
    data: invalidUser,
    result: invalidResult,
  },
  summary: {
    totalChecked: validResult.checkedPaths + invalidResult.checkedPaths,
    validPassed: validResult.valid,
    invalidErrorCount: invalidResult.errors.length,
  },
};
