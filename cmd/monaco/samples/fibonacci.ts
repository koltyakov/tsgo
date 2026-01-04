// Context: Financial calculation utilities

interface CompoundingSchedule {
  periods: number;
  ratePerPeriod: number;
}

export function calculateCompoundGrowth(principal: number, schedule: CompoundingSchedule): number {
  return principal * Math.pow(1 + schedule.ratePerPeriod, schedule.periods);
}

export function formatPercentage(value: number, decimals?: number): string {
  return (value * 100).toFixed(decimals ?? 2) + '%';
}

export const annualRate = 0.07; // 7% annual return

// --- Code ---

// Fibonacci Sequence - Classic recursive algorithm
// Demonstrates recursion and memoization patterns

// Simple recursive Fibonacci (for small n)
function fibRecursive(n: number): number {
  if (n <= 1) return n;
  return fibRecursive(n - 1) + fibRecursive(n - 2);
}

// Optimized iterative Fibonacci (for large n)
function fibIterative(n: number): number {
  if (n <= 1) return n;
  
  let prev = 0;
  let curr = 1;
  
  for (let i = 2; i <= n; i++) {
    const next = prev + curr;
    prev = curr;
    curr = next;
  }
  
  return curr;
}

// Generate a sequence of Fibonacci numbers
function fibSequence(count: number): number[] {
  const sequence: number[] = [];
  for (let i = 0; i < count; i++) {
    sequence.push(fibIterative(i));
  }
  return sequence;
}

// Calculate some Fibonacci values
const fib10 = fibRecursive(10);
const fib30 = fibIterative(30);
const first15 = fibSequence(15);

// Use injected financial functions to show growth patterns
const investmentGrowth = calculateCompoundGrowth(10000, { 
  periods: 12, 
  ratePerPeriod: annualRate / 12 
});
const growthRate = formatPercentage(annualRate);

export default {
  fib10,
  fib30,
  first15Numbers: first15,
  goldenRatio: fibIterative(20) / fibIterative(19),
  financialExample: {
    principal: 10000,
    annualRate: growthRate,
    afterOneYear: Math.round(investmentGrowth * 100) / 100,
  },
};
