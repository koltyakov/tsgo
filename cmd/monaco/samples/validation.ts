// Context: User registration and compliance utilities

interface ComplianceConfig {
  minPasswordLength: number;
  minAge: number;
  maxAge: number;
  bannedUsernames: string[];
  requireMFA: boolean;
}

export const complianceConfig: ComplianceConfig = {
  minPasswordLength: 8,
  minAge: 13,
  maxAge: 120,
  bannedUsernames: ['admin', 'root', 'system', 'moderator'],
  requireMFA: true
};

export function checkEmailDomain(email: string, allowedDomains?: string[]): boolean {
  const domain = email.split('@')[1]?.toLowerCase();
  if (!domain) return false;
  if (!allowedDomains || allowedDomains.length === 0) return true;
  return allowedDomains.some(d => domain.endsWith(d.toLowerCase()));
}

export function sanitizeUsername(username: string): string {
  return username.toLowerCase().replace(/[^a-z0-9_]/g, '');
}

// --- Code ---

// Input Validation - Schema validation with error handling
// Demonstrates type guards and validation patterns

interface ValidationResult {
  valid: boolean;
  errors: string[];
}

interface RegistrationData {
  username: string;
  email: string;
  password: string;
  age: number;
  acceptedTerms: boolean;
}

// Validation rules
const rules = {
  username: {
    minLength: 3,
    maxLength: 20,
    pattern: /^[a-zA-Z0-9_]+$/,
  },
  email: {
    pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
  },
  password: {
    minLength: complianceConfig.minPasswordLength,
    requireUppercase: true,
    requireLowercase: true,
    requireNumber: true,
  },
  age: {
    min: complianceConfig.minAge,
    max: complianceConfig.maxAge,
  },
};

// Validate a single field
function validateField(field: string, value: any): string[] {
  const errors: string[] = [];

  switch (field) {
    case "username":
      if (typeof value !== "string") {
        errors.push("Username must be a string");
      } else {
        const sanitized = sanitizeUsername(value);
        if (sanitized.length < rules.username.minLength) {
          errors.push(`Username must be at least ${rules.username.minLength} characters`);
        }
        if (sanitized.length > rules.username.maxLength) {
          errors.push(`Username must be at most ${rules.username.maxLength} characters`);
        }
        if (!rules.username.pattern.test(value)) {
          errors.push("Username can only contain letters, numbers, and underscores");
        }
        if (complianceConfig.bannedUsernames.includes(sanitized)) {
          errors.push("This username is reserved and cannot be used");
        }
      }
      break;

    case "email":
      if (typeof value !== "string") {
        errors.push("Email must be a string");
      } else if (!rules.email.pattern.test(value)) {
        errors.push("Invalid email format");
      } else if (!checkEmailDomain(value)) {
        errors.push("Email domain is not allowed");
      }
      break;

    case "password":
      if (typeof value !== "string") {
        errors.push("Password must be a string");
      } else {
        if (value.length < rules.password.minLength) {
          errors.push(`Password must be at least ${rules.password.minLength} characters`);
        }
        if (!/[A-Z]/.test(value)) {
          errors.push("Password must contain at least one uppercase letter");
        }
        if (!/[a-z]/.test(value)) {
          errors.push("Password must contain at least one lowercase letter");
        }
        if (!/[0-9]/.test(value)) {
          errors.push("Password must contain at least one number");
        }
      }
      break;

    case "age":
      if (typeof value !== "number") {
        errors.push("Age must be a number");
      } else {
        if (value < rules.age.min) {
          errors.push(`Age must be at least ${rules.age.min}`);
        }
        if (value > rules.age.max) {
          errors.push(`Age must be at most ${rules.age.max}`);
        }
      }
      break;

    case "acceptedTerms":
      if (value !== true) {
        errors.push("You must accept the terms and conditions");
      }
      break;
  }

  return errors;
}

// Validate entire registration form
function validateRegistration(data: Partial<RegistrationData>): ValidationResult {
  const allErrors: string[] = [];
  const requiredFields = ["username", "email", "password", "age", "acceptedTerms"];

  // Check for missing fields
  for (const field of requiredFields) {
    if (!(field in data)) {
      allErrors.push(`${field} is required`);
    }
  }

  // Validate each provided field
  for (const [field, value] of Object.entries(data)) {
    const fieldErrors = validateField(field, value);
    allErrors.push(...fieldErrors);
  }

  return {
    valid: allErrors.length === 0,
    errors: allErrors,
  };
}

// Test cases
const validUser: RegistrationData = {
  username: "john_doe",
  email: "john@example.com",
  password: "SecurePass123",
  age: 25,
  acceptedTerms: true,
};

const invalidUser: Partial<RegistrationData> = {
  username: "admin", // Banned username
  email: "not-an-email",
  password: "weak",
  age: 10, // Below minimum
  acceptedTerms: false,
};

const partialUser: Partial<RegistrationData> = {
  username: "jane_smith",
  email: "jane@company.org",
};

// Run validations
const validResult = validateRegistration(validUser);
const invalidResult = validateRegistration(invalidUser);
const partialResult = validateRegistration(partialUser);

export default {
  complianceSettings: {
    minPasswordLength: complianceConfig.minPasswordLength,
    minAge: complianceConfig.minAge,
    requireMFA: complianceConfig.requireMFA,
  },
  testResults: {
    validUser: {
      input: validUser.username,
      result: validResult.valid ? "✓ Valid" : "✗ Invalid",
      errors: validResult.errors,
    },
    invalidUser: {
      input: invalidUser.username,
      result: invalidResult.valid ? "✓ Valid" : "✗ Invalid",
      errors: invalidResult.errors,
    },
    partialUser: {
      input: partialUser.username,
      result: partialResult.valid ? "✓ Valid" : "✗ Invalid",
      errors: partialResult.errors,
    },
  },
};
