// Context: Bun runtime utilities (Bun only)

// Bun type declarations (these APIs exist at runtime)
declare const Bun: {
  version: string;
  revision: string;
  main: string;
  nanoseconds(): bigint;
  hash: {
    md5(data: string): string;
    sha1(data: string): string;
    sha256(data: string): string;
    sha512(data: string): string;
    crc32(data: string): number;
    adler32(data: string): number;
    [key: string]: ((data: string) => string | number | bigint) | undefined;
  };
  password: {
    hash(password: string, options?: { algorithm?: string; cost?: number }): Promise<string>;
    verify(password: string, hash: string): Promise<boolean>;
  };
};

declare const process: {
  platform: string;
  arch: string;
  memoryUsage(): { heapUsed: number };
  exit(code?: number): never;
};

export const bunInfo = {
  version: typeof Bun !== 'undefined' ? Bun.version : 'N/A',
  revision: typeof Bun !== 'undefined' ? Bun.revision : 'N/A',
};

export function formatBytes(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${size.toFixed(2)} ${units[unitIndex]}`;
}

export function formatNanoseconds(ns: number): string {
  if (ns < 1000) return `${ns}ns`;
  if (ns < 1000000) return `${(ns / 1000).toFixed(2)}μs`;
  if (ns < 1000000000) return `${(ns / 1000000).toFixed(2)}ms`;
  return `${(ns / 1000000000).toFixed(2)}s`;
}

// --- Code ---

// Bun Native APIs - Demonstrating Bun-specific features
// Showcases hashing, high-precision timing, and runtime info

// ============================================
// 1. High-Precision Timing with Bun.nanoseconds()
// ============================================

interface TimingResult {
  operation: string;
  iterations: number;
  totalNs: number;
  avgNs: number;
  formatted: string;
}

function benchmark(name: string, fn: () => void, iterations = 1000): TimingResult {
  const start = Bun.nanoseconds();
  for (let i = 0; i < iterations; i++) {
    fn();
  }
  const totalNs = Number(Bun.nanoseconds() - start);
  const avgNs = totalNs / iterations;
  
  return {
    operation: name,
    iterations,
    totalNs,
    avgNs,
    formatted: formatNanoseconds(avgNs),
  };
}

// Benchmark various operations
const timingResults: TimingResult[] = [];

timingResults.push(benchmark('Object creation', () => {
  const obj = { a: 1, b: 2, c: 3 };
}));

timingResults.push(benchmark('Array push', () => {
  const arr: number[] = [];
  arr.push(1, 2, 3, 4, 5);
}));

timingResults.push(benchmark('String concatenation', () => {
  const s = 'hello' + ' ' + 'world';
}));

timingResults.push(benchmark('JSON parse', () => {
  JSON.parse('{"name":"test","value":42}');
}));

timingResults.push(benchmark('Math operations', () => {
  Math.sqrt(Math.pow(3, 2) + Math.pow(4, 2));
}));

// ============================================
// 2. Cryptographic Hashing with Bun.hash()
// ============================================

interface HashResult {
  algorithm: string;
  input: string;
  hash: string;
  hashLength: number;
}

const testData = 'The quick brown fox jumps over the lazy dog';

const hashResults: HashResult[] = [];

// Bun.hash provides multiple algorithms
const algorithms = ['md5', 'sha1', 'sha256', 'sha512'] as const;

for (const algo of algorithms) {
  const hashFn = Bun.hash[algo];
  if (hashFn) {
    const hash = hashFn(testData) as string | number | bigint;
    // Convert to hex string
    const hexHash = typeof hash === 'bigint' 
      ? hash.toString(16).padStart(32, '0')
      : typeof hash === 'number'
        ? hash.toString(16)
        : String(hash);
    
    hashResults.push({
      algorithm: algo.toUpperCase(),
      input: testData.substring(0, 30) + '...',
      hash: hexHash.substring(0, 32) + (hexHash.length > 32 ? '...' : ''),
      hashLength: hexHash.length,
    });
  }
}

// CRC32 hash (fast, non-cryptographic)
const crc32Hash = Bun.hash.crc32(testData);
hashResults.push({
  algorithm: 'CRC32',
  input: testData.substring(0, 30) + '...',
  hash: crc32Hash.toString(16),
  hashLength: 8,
});

// Adler32 hash (fast checksum)
const adler32Hash = Bun.hash.adler32(testData);
hashResults.push({
  algorithm: 'Adler32',
  input: testData.substring(0, 30) + '...',
  hash: adler32Hash.toString(16),
  hashLength: 8,
});

// ============================================
// 3. Password Hashing with Bun.password
// ============================================

interface PasswordResult {
  password: string;
  algorithm: string;
  hash: string;
  verified: boolean;
  verifyTime: string;
}

const testPassword = 'SuperSecure123!';

// Hash with bcrypt (default)
const bcryptHash = await Bun.password.hash(testPassword, {
  algorithm: 'bcrypt',
  cost: 4, // Low cost for demo (use 10+ in production)
});

const verifyStart = Bun.nanoseconds();
const bcryptVerified = await Bun.password.verify(testPassword, bcryptHash);
const verifyTime = Number(Bun.nanoseconds() - verifyStart);

const passwordResults: PasswordResult[] = [
  {
    password: testPassword.substring(0, 4) + '****',
    algorithm: 'bcrypt (cost=4)',
    hash: bcryptHash.substring(0, 30) + '...',
    verified: bcryptVerified,
    verifyTime: formatNanoseconds(verifyTime),
  },
];

// ============================================
// 4. Runtime Information
// ============================================

interface RuntimeInfo {
  bunVersion: string;
  bunRevision: string;
  platform: string;
  arch: string;
  mainFile: string;
  peakMemory: string;
}

const runtimeInfo: RuntimeInfo = {
  bunVersion: Bun.version,
  bunRevision: Bun.revision.substring(0, 8),
  platform: process.platform,
  arch: process.arch,
  mainFile: Bun.main.split('/').pop() || 'unknown',
  peakMemory: formatBytes(process.memoryUsage().heapUsed),
};

// ============================================
// 5. Fast UUID Generation
// ============================================

interface UUIDResult {
  method: string;
  samples: string[];
  generationTime: string;
}

const uuidStart = Bun.nanoseconds();
const uuids: string[] = [];
for (let i = 0; i < 5; i++) {
  uuids.push(crypto.randomUUID());
}
const uuidTime = Number(Bun.nanoseconds() - uuidStart);

const uuidResult: UUIDResult = {
  method: 'crypto.randomUUID()',
  samples: uuids,
  generationTime: formatNanoseconds(uuidTime / 5),
};

// ============================================
// Result Type
// ============================================

interface PlaygroundResult {
  runtimeInfo: RuntimeInfo;
  benchmarks: TimingResult[];
  hashing: HashResult[];
  passwords: PasswordResult[];
  uuids: UUIDResult;
  summary: {
    fastestOperation: string;
    slowestOperation: string;
    hashAlgorithmsAvailable: number;
  };
}

// Find fastest and slowest operations
const sorted = [...timingResults].sort((a, b) => a.avgNs - b.avgNs);

const result: PlaygroundResult = {
  runtimeInfo,
  benchmarks: timingResults,
  hashing: hashResults,
  passwords: passwordResults,
  uuids: uuidResult,
  summary: {
    fastestOperation: sorted[0].operation,
    slowestOperation: sorted[sorted.length - 1].operation,
    hashAlgorithmsAvailable: hashResults.length,
  },
};

export default result;
