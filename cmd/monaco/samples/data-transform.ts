// Context: E-commerce inventory and pricing utilities

interface TaxConfig {
  defaultRate: number;
  rates: Record<string, number>;
}

export const taxConfig: TaxConfig = {
  defaultRate: 0.08,
  rates: {
    'electronics': 0.10,
    'food': 0.02,
    'clothing': 0.07,
    'furniture': 0.08,
    'office': 0.08,
    'kitchen': 0.06,
  }
};

export function calculateTax(amount: number, category: string): number {
  const rate = taxConfig.rates[category] ?? taxConfig.defaultRate;
  return Math.round(amount * rate * 100) / 100;
}

export function applyDiscount(price: number, discountPercent: number): number {
  return Math.round(price * (1 - discountPercent / 100) * 100) / 100;
}

// --- Code ---

// Data Transformation - Transform and filter collections
// Demonstrates functional programming patterns

interface Product {
  id: number;
  name: string;
  price: number;
  category: string;
  inStock: boolean;
}

interface CartItem {
  productId: number;
  quantity: number;
}

// Sample product catalog
const products: Product[] = [
  { id: 1, name: "Laptop", price: 999.99, category: "electronics", inStock: true },
  { id: 2, name: "Headphones", price: 149.99, category: "electronics", inStock: true },
  { id: 3, name: "Coffee Mug", price: 12.99, category: "kitchen", inStock: false },
  { id: 4, name: "Desk Chair", price: 299.99, category: "furniture", inStock: true },
  { id: 5, name: "Notebook", price: 4.99, category: "office", inStock: true },
  { id: 6, name: "Monitor", price: 399.99, category: "electronics", inStock: true },
  { id: 7, name: "Keyboard", price: 79.99, category: "electronics", inStock: false },
];

// Shopping cart
const cart: CartItem[] = [
  { productId: 1, quantity: 1 },
  { productId: 2, quantity: 2 },
  { productId: 5, quantity: 5 },
];

// Filter products by category
function filterByCategory(items: Product[], category: string): Product[] {
  return items.filter(item => item.category === category);
}

// Get only in-stock products
function getInStockProducts(items: Product[]): Product[] {
  return items.filter(item => item.inStock);
}

// Calculate cart total with tax
function calculateCartTotal(items: CartItem[], catalog: Product[]): { subtotal: number; tax: number; total: number } {
  let subtotal = 0;
  let totalTax = 0;
  
  for (const cartItem of items) {
    const product = catalog.find(p => p.id === cartItem.productId);
    if (product && product.inStock) {
      const lineTotal = product.price * cartItem.quantity;
      subtotal += lineTotal;
      totalTax += calculateTax(lineTotal, product.category);
    }
  }
  
  return {
    subtotal: Math.round(subtotal * 100) / 100,
    tax: Math.round(totalTax * 100) / 100,
    total: Math.round((subtotal + totalTax) * 100) / 100,
  };
}

// Group products by category
function groupByCategory(items: Product[]): Record<string, Product[]> {
  return items.reduce((groups, item) => {
    const category = item.category;
    if (!groups[category]) {
      groups[category] = [];
    }
    groups[category].push(item);
    return groups;
  }, {} as Record<string, Product[]>);
}

interface EnrichedCartItem extends CartItem {
  product: Product | undefined;
  subtotal: number;
}

// Get cart items with full product details
function enrichCartItems(items: CartItem[], catalog: Product[]): EnrichedCartItem[] {
  return items.map(cartItem => {
    const product = catalog.find(p => p.id === cartItem.productId);
    return {
      ...cartItem,
      product,
      subtotal: product ? product.price * cartItem.quantity : 0,
    };
  });
}

// Perform transformations
const electronics = filterByCategory(products, "electronics");
const available = getInStockProducts(products);
const cartTotals = calculateCartTotal(cart, products);
const byCategory = groupByCategory(products);
const enrichedCart = enrichCartItems(cart, products);

// Apply member discount using injected function
const memberDiscount = 10;
const discountedSubtotal = applyDiscount(cartTotals.subtotal, memberDiscount);

export default {
  electronicsCount: electronics.length,
  availableProducts: available.map(p => p.name),
  cartBreakdown: {
    subtotal: cartTotals.subtotal.toFixed(2),
    tax: cartTotals.tax.toFixed(2),
    total: cartTotals.total.toFixed(2),
    afterMemberDiscount: discountedSubtotal.toFixed(2),
  },
  categorySummary: Object.keys(byCategory).map(cat => ({
    category: cat,
    count: byCategory[cat].length,
    taxRate: (taxConfig.rates[cat] * 100) + '%',
  })),
  cartDetails: enrichedCart,
};
