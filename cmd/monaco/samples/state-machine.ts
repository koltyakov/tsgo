// Context: Order fulfillment system utilities

interface ShippingConfig {
  carriers: Record<string, { name: string; baseRate: number; perKg: number }>;
  defaultCarrier: string;
  freeShippingThreshold: number;
}

export const shippingConfig: ShippingConfig = {
  carriers: {
    'standard': { name: 'Standard Ground', baseRate: 5.99, perKg: 0.50 },
    'express': { name: 'Express 2-Day', baseRate: 12.99, perKg: 1.00 },
    'overnight': { name: 'Overnight', baseRate: 24.99, perKg: 2.50 },
  },
  defaultCarrier: 'standard',
  freeShippingThreshold: 100
};

export function calculateShipping(subtotal: number, weightKg: number, carrier?: string): number {
  if (subtotal >= shippingConfig.freeShippingThreshold) return 0;
  const c = shippingConfig.carriers[carrier || shippingConfig.defaultCarrier];
  return Math.round((c.baseRate + weightKg * c.perKg) * 100) / 100;
}

export function generateTrackingNumber(): string {
  return 'TRK' + Date.now().toString(36).toUpperCase() + Math.random().toString(36).substring(2, 6).toUpperCase();
}

// --- Code ---

// State Machine - Order workflow state transitions
// Demonstrates finite state machine pattern for business workflows

type OrderState = 
  | "pending"
  | "confirmed"
  | "processing"
  | "shipped"
  | "delivered"
  | "cancelled"
  | "refunded";

interface OrderTransition {
  from: OrderState;
  to: OrderState;
  action: string;
  guard?: (order: Order) => boolean;
}

interface Order {
  id: string;
  state: OrderState;
  items: { name: string; quantity: number; price: number; weightKg: number }[];
  trackingNumber?: string;
  createdAt: string;
  history: { state: OrderState; timestamp: string; action: string }[];
}

// Define valid state transitions
const transitions: OrderTransition[] = [
  { from: "pending", to: "confirmed", action: "confirm" },
  { from: "pending", to: "cancelled", action: "cancel" },
  { from: "confirmed", to: "processing", action: "startProcessing" },
  { from: "confirmed", to: "cancelled", action: "cancel" },
  { from: "processing", to: "shipped", action: "ship" },
  { from: "processing", to: "cancelled", action: "cancel" },
  { from: "shipped", to: "delivered", action: "deliver" },
  { from: "delivered", to: "refunded", action: "refund", guard: (order) => {
    // Can only refund within 30 days (simplified check)
    return true;
  }},
  { from: "cancelled", to: "pending", action: "reopen" },
];

// Get available actions for current state
function getAvailableActions(state: OrderState): string[] {
  return transitions
    .filter(t => t.from === state)
    .map(t => t.action);
}

// Execute a state transition
function transition(order: Order, action: string): { success: boolean; order: Order; error?: string } {
  const validTransition = transitions.find(
    t => t.from === order.state && t.action === action
  );

  if (!validTransition) {
    return {
      success: false,
      order,
      error: `Invalid action '${action}' for state '${order.state}'`,
    };
  }

  // Check guard condition if present
  if (validTransition.guard && !validTransition.guard(order)) {
    return {
      success: false,
      order,
      error: `Guard condition failed for action '${action}'`,
    };
  }

  // Create new order with updated state
  const newOrder: Order = {
    ...order,
    state: validTransition.to,
    history: [
      ...order.history,
      {
        state: validTransition.to,
        timestamp: new Date().toISOString(),
        action,
      },
    ],
  };

  // Generate tracking number when shipped
  if (action === 'ship' && !newOrder.trackingNumber) {
    newOrder.trackingNumber = generateTrackingNumber();
  }

  return { success: true, order: newOrder };
}

// Calculate order totals
function calculateOrderTotals(order: Order): { subtotal: number; shipping: number; total: number } {
  const subtotal = order.items.reduce((sum, item) => sum + item.price * item.quantity, 0);
  const totalWeight = order.items.reduce((sum, item) => sum + item.weightKg * item.quantity, 0);
  const shipping = calculateShipping(subtotal, totalWeight);
  
  return {
    subtotal: Math.round(subtotal * 100) / 100,
    shipping,
    total: Math.round((subtotal + shipping) * 100) / 100,
  };
}

// Create a sample order
function createOrder(): Order {
  return {
    id: `ORD-${Date.now()}`,
    state: "pending",
    items: [
      { name: "Widget Pro", quantity: 2, price: 29.99, weightKg: 0.5 },
      { name: "Gadget Plus", quantity: 1, price: 49.99, weightKg: 1.2 },
    ],
    createdAt: new Date().toISOString(),
    history: [
      { state: "pending", timestamp: new Date().toISOString(), action: "create" },
    ],
  };
}

// Simulate an order workflow
let order = createOrder();
const workflow: string[] = [];
const totals = calculateOrderTotals(order);

// Try to process the order through various states
const actions = ["confirm", "startProcessing", "ship", "deliver"];

for (const action of actions) {
  const result = transition(order, action);
  if (result.success) {
    order = result.order;
    workflow.push(`✓ ${action}: ${order.state}`);
  } else {
    workflow.push(`✗ ${action}: ${result.error}`);
    break;
  }
}

// Try an invalid transition
const invalidResult = transition(order, "confirm");

export default {
  orderId: order.id,
  finalState: order.state,
  trackingNumber: order.trackingNumber,
  workflow,
  stateHistory: order.history.map(h => `${h.action} → ${h.state}`),
  availableActions: getAvailableActions(order.state),
  invalidTransitionTest: {
    action: "confirm",
    result: invalidResult.error,
  },
  financials: {
    subtotal: totals.subtotal.toFixed(2),
    shipping: totals.shipping.toFixed(2),
    total: totals.total.toFixed(2),
    freeShippingAt: shippingConfig.freeShippingThreshold,
  },
};
