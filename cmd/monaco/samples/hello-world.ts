// Context: Customer support system globals

interface Agent {
  id: number;
  name: string;
  email: string;
  department: 'sales' | 'support' | 'billing';
  activeTickets: number;
}

interface Tenant {
  id: string;
  name: string;
  plan: 'starter' | 'professional' | 'enterprise';
  maxAgents: number;
}

export const currentAgent: Agent = {
  id: 42,
  name: "Sarah Chen",
  email: "sarah.chen@company.com",
  department: "support",
  activeTickets: 7
};

export const tenant: Tenant = {
  id: "tenant-acme-corp",
  name: "Acme Corporation",
  plan: "enterprise",
  maxAgents: 50
};

export function formatCurrency(amount: number, currency?: string): string {
  const symbol = currency === 'EUR' ? '€' : currency === 'GBP' ? '£' : '$';
  return symbol + amount.toFixed(2);
}

export function calculatePriority(ticketAge: number, customerTier: string): number {
  const tierMultiplier = customerTier === 'enterprise' ? 2 : customerTier === 'professional' ? 1.5 : 1;
  return Math.min(Math.floor(ticketAge * tierMultiplier / 24), 5);
}

// --- Code ---

// Hello World - Basic greeting with type-safe globals
// This example shows how to use injected global variables

// Access the injected agent object with full type safety
const agent: Agent = currentAgent;

// Create a personalized greeting
const greeting = `Welcome back, ${agent.name}!`;

// Access tenant configuration
const tenantInfo = `Logged into: ${tenant.name} (${tenant.plan} plan)`;

// Use injected helper functions
const ticketValue = formatCurrency(299.99);
const urgency = calculatePriority(48, tenant.plan);

// Export a simple result
export default {
  greeting,
  agentId: agent.id,
  department: agent.department,
  tenantInfo,
  sampleTicketValue: ticketValue,
  calculatedPriority: urgency,
};
