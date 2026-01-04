// Context: Loyalty program and pricing configuration

interface LoyaltyTier {
  name: string;
  minPoints: number;
  multiplier: number;
  perks: string[];
}

export const loyaltyTiers: Record<string, LoyaltyTier> = {
  bronze: { name: 'Bronze', minPoints: 0, multiplier: 1.0, perks: ['Newsletter'] },
  silver: { name: 'Silver', minPoints: 1000, multiplier: 1.25, perks: ['Newsletter', 'Early Access'] },
  gold: { name: 'Gold', minPoints: 5000, multiplier: 1.5, perks: ['Newsletter', 'Early Access', 'Free Shipping'] },
  platinum: { name: 'Platinum', minPoints: 15000, multiplier: 2.0, perks: ['Newsletter', 'Early Access', 'Free Shipping', 'Personal Concierge'] },
};

export function calculateLoyaltyPoints(purchaseAmount: number, tierKey: string): number {
  const tier = loyaltyTiers[tierKey] || loyaltyTiers.bronze;
  return Math.floor(purchaseAmount * tier.multiplier);
}

export function getTierByPoints(points: number): string {
  const tiers = Object.entries(loyaltyTiers).sort((a, b) => b[1].minPoints - a[1].minPoints);
  for (const [key, tier] of tiers) {
    if (points >= tier.minPoints) return key;
  }
  return 'bronze';
}

// --- Code ---

// Business Rules Engine - Dynamic pricing calculation
// Demonstrates rule-based systems and strategy patterns

interface PricingRule {
  id: string;
  name: string;
  priority: number;
  condition: (context: PricingContext) => boolean;
  discount: number; // percentage discount (0-100)
  type: "percentage" | "fixed";
}

interface PricingContext {
  customerId: string;
  customerTier: "bronze" | "silver" | "gold" | "platinum";
  loyaltyPoints: number;
  cartTotal: number;
  itemCount: number;
  isFirstOrder: boolean;
  dayOfWeek: number; // 0-6 (Sunday-Saturday)
  couponCode?: string;
}

interface PricingResult {
  originalTotal: number;
  finalTotal: number;
  appliedRules: { name: string; discount: number }[];
  totalDiscount: number;
  savings: number;
  pointsEarned: number;
}

// Define pricing rules (highest priority first)
const pricingRules: PricingRule[] = [
  {
    id: "platinum-vip",
    name: "Platinum VIP Discount",
    priority: 100,
    condition: (ctx) => ctx.customerTier === "platinum",
    discount: 20,
    type: "percentage",
  },
  {
    id: "gold-member",
    name: "Gold Member Discount",
    priority: 90,
    condition: (ctx) => ctx.customerTier === "gold",
    discount: 15,
    type: "percentage",
  },
  {
    id: "silver-member",
    name: "Silver Member Discount",
    priority: 80,
    condition: (ctx) => ctx.customerTier === "silver",
    discount: 10,
    type: "percentage",
  },
  {
    id: "first-order",
    name: "First Order Bonus",
    priority: 70,
    condition: (ctx) => ctx.isFirstOrder,
    discount: 15,
    type: "percentage",
  },
  {
    id: "bulk-order",
    name: "Bulk Order Discount",
    priority: 60,
    condition: (ctx) => ctx.itemCount >= 10,
    discount: 5,
    type: "percentage",
  },
  {
    id: "weekend-sale",
    name: "Weekend Sale",
    priority: 50,
    condition: (ctx) => ctx.dayOfWeek === 0 || ctx.dayOfWeek === 6,
    discount: 10,
    type: "percentage",
  },
  {
    id: "large-cart",
    name: "Large Cart Bonus",
    priority: 40,
    condition: (ctx) => ctx.cartTotal >= 500,
    discount: 25,
    type: "fixed",
  },
  {
    id: "coupon-save10",
    name: "SAVE10 Coupon",
    priority: 30,
    condition: (ctx) => ctx.couponCode === "SAVE10",
    discount: 10,
    type: "percentage",
  },
  {
    id: "coupon-flat20",
    name: "FLAT20 Coupon",
    priority: 30,
    condition: (ctx) => ctx.couponCode === "FLAT20",
    discount: 20,
    type: "fixed",
  },
];

// Rule engine configuration
const config = {
  maxRules: 3, // Maximum number of rules that can be applied
  maxDiscountPercentage: 40, // Cap total percentage discount
  stackableTypes: true, // Allow stacking percentage and fixed discounts
};

// Calculate pricing with rule engine
function calculatePricing(context: PricingContext): PricingResult {
  // Sort rules by priority (descending)
  const sortedRules = [...pricingRules].sort((a, b) => b.priority - a.priority);
  
  const appliedRules: { name: string; discount: number }[] = [];
  let totalPercentageDiscount = 0;
  let totalFixedDiscount = 0;
  
  // Evaluate rules
  for (const rule of sortedRules) {
    // Check if we've hit the max rules limit
    if (appliedRules.length >= config.maxRules) break;
    
    // Evaluate condition
    if (!rule.condition(context)) continue;
    
    // Apply rule based on type
    if (rule.type === "percentage") {
      // Check if adding this would exceed max discount
      if (totalPercentageDiscount + rule.discount > config.maxDiscountPercentage) {
        const remaining = config.maxDiscountPercentage - totalPercentageDiscount;
        if (remaining > 0) {
          appliedRules.push({ name: rule.name, discount: remaining });
          totalPercentageDiscount = config.maxDiscountPercentage;
        }
        break; // Stop processing percentage rules
      }
      
      appliedRules.push({ name: rule.name, discount: rule.discount });
      totalPercentageDiscount += rule.discount;
    } else {
      // Fixed discount
      appliedRules.push({ name: rule.name, discount: rule.discount });
      totalFixedDiscount += rule.discount;
    }
  }
  
  // Calculate final total
  const percentageSavings = context.cartTotal * (totalPercentageDiscount / 100);
  const afterPercentage = context.cartTotal - percentageSavings;
  const finalTotal = Math.max(0, afterPercentage - totalFixedDiscount);
  const totalSavings = context.cartTotal - finalTotal;
  
  // Calculate loyalty points earned using injected function
  const pointsEarned = calculateLoyaltyPoints(finalTotal, context.customerTier);
  
  return {
    originalTotal: context.cartTotal,
    finalTotal: Math.round(finalTotal * 100) / 100,
    appliedRules,
    totalDiscount: totalPercentageDiscount + (totalFixedDiscount / context.cartTotal * 100),
    savings: Math.round(totalSavings * 100) / 100,
    pointsEarned,
  };
}

// Test scenarios
const scenarios: { name: string; context: PricingContext }[] = [
  {
    name: "New Gold Customer",
    context: {
      customerId: "C001",
      customerTier: "gold",
      loyaltyPoints: 5500,
      cartTotal: 350,
      itemCount: 5,
      isFirstOrder: true,
      dayOfWeek: 3, // Wednesday
    },
  },
  {
    name: "Platinum Bulk Weekend",
    context: {
      customerId: "C002",
      customerTier: "platinum",
      loyaltyPoints: 18000,
      cartTotal: 750,
      itemCount: 15,
      isFirstOrder: false,
      dayOfWeek: 6, // Saturday
    },
  },
  {
    name: "Bronze with Coupon",
    context: {
      customerId: "C003",
      customerTier: "bronze",
      loyaltyPoints: 250,
      cartTotal: 200,
      itemCount: 3,
      isFirstOrder: false,
      dayOfWeek: 2, // Tuesday
      couponCode: "SAVE10",
    },
  },
];

// Calculate all scenarios
const results = scenarios.map(scenario => ({
  scenario: scenario.name,
  tier: loyaltyTiers[scenario.context.customerTier].name,
  ...calculatePricing(scenario.context),
}));

// Summary statistics
const totalOriginal = results.reduce((sum, r) => sum + r.originalTotal, 0);
const totalFinal = results.reduce((sum, r) => sum + r.finalTotal, 0);
const totalSaved = results.reduce((sum, r) => sum + r.savings, 0);
const totalPoints = results.reduce((sum, r) => sum + r.pointsEarned, 0);

export default {
  scenarios: results,
  summary: {
    totalOriginalValue: totalOriginal.toFixed(2),
    totalFinalValue: totalFinal.toFixed(2),
    totalSavings: totalSaved.toFixed(2),
    averageDiscountPercent: ((totalSaved / totalOriginal) * 100).toFixed(1) + "%",
    totalPointsEarned: totalPoints,
  },
  loyaltyProgram: Object.values(loyaltyTiers).map(t => ({
    tier: t.name,
    minPoints: t.minPoints,
    multiplier: t.multiplier + 'x',
    perks: t.perks.length,
  })),
  ruleEngineConfig: config,
};
