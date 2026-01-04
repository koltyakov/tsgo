// Context: Task queue and worker configuration (Bun only)

interface WorkerPool {
  maxConcurrency: number;
  taskTimeout: number;
  retryPolicy: {
    maxRetries: number;
    backoffMs: number;
  };
}

export const workerPool: WorkerPool = {
  maxConcurrency: 4,
  taskTimeout: 10000,
  retryPolicy: {
    maxRetries: 3,
    backoffMs: 100
  }
};

export function generateTaskId(): string {
  return 'task_' + Date.now().toString(36) + '_' + Math.random().toString(36).substring(2, 8);
}

export function calculateEfficiency(actualMs: number, sequentialMs: number): number {
  return Math.round(((sequentialMs - actualMs) / sequentialMs) * 100);
}

// --- Code ---

// Parallel Tasks - Concurrent async operations (Bun only)
// Demonstrates Promise.all, Promise.race, and concurrent patterns

interface TaskResult {
  taskId: string;
  duration: number;
  result: string;
}

// Simulate an async task with variable duration
async function simulateTask(taskId: string, durationMs: number): Promise<TaskResult> {
  const start = Date.now();
  
  // Simulate work
  await new Promise(resolve => setTimeout(resolve, durationMs));
  
  return {
    taskId,
    duration: Date.now() - start,
    result: `Task ${taskId} completed`,
  };
}

// Run multiple tasks in parallel
async function runParallel(tasks: { id: string; duration: number }[]): Promise<{
  results: TaskResult[];
  totalDuration: number;
  sequentialWouldTake: number;
}> {
  const start = Date.now();
  
  const results = await Promise.all(
    tasks.map(t => simulateTask(t.id, t.duration))
  );
  
  const totalDuration = Date.now() - start;
  const sequentialWouldTake = tasks.reduce((sum, t) => sum + t.duration, 0);
  
  return {
    results,
    totalDuration,
    sequentialWouldTake,
  };
}

// Run tasks with concurrency limit
async function runWithLimit<T>(
  tasks: (() => Promise<T>)[],
  concurrency: number
): Promise<T[]> {
  const results: T[] = [];
  const executing: Promise<void>[] = [];
  
  for (const task of tasks) {
    const p = task().then(result => {
      results.push(result);
    });
    
    executing.push(p);
    
    if (executing.length >= concurrency) {
      await Promise.race(executing);
      executing.splice(
        executing.findIndex(e => e === p),
        1
      );
    }
  }
  
  await Promise.all(executing);
  return results;
}

// Race multiple data sources
// API response type
interface Todo {
  userId: number;
  id: number;
  title: string;
  completed: boolean;
}

// Race result type
interface RaceResult {
  source: string;
  data: Todo;
}

async function fetchWithFallback(): Promise<RaceResult> {
  const sources = [
    { name: "primary", url: "https://jsonplaceholder.typicode.com/todos/1" },
    { name: "secondary", url: "https://jsonplaceholder.typicode.com/todos/2" },
  ];
  
  // Race between sources - first to respond wins
  const result = await Promise.race(
    sources.map(async source => {
      const response = await fetch(source.url);
      const data: Todo = await response.json();
      return { source: source.name, data };
    })
  );
  
  return result;
}

// Main execution
async function main() {
  // Define parallel tasks with generated IDs
  const tasks = [
    { id: generateTaskId(), duration: 100 },
    { id: generateTaskId(), duration: 150 },
    { id: generateTaskId(), duration: 80 },
    { id: generateTaskId(), duration: 120 },
  ];
  
  // Run all tasks in parallel
  const parallelResult = await runParallel(tasks);
  
  // Run with concurrency limit from worker pool config
  const limitedTasks = tasks.map(t => () => simulateTask(t.id + "-limited", t.duration));
  const limitedStart = Date.now();
  const limitedResults = await runWithLimit(limitedTasks, workerPool.maxConcurrency);
  const limitedDuration = Date.now() - limitedStart;
  
  // Race for fastest response
  const raceResult = await fetchWithFallback();
  
  // Calculate efficiency using injected function
  const efficiency = calculateEfficiency(
    parallelResult.totalDuration,
    parallelResult.sequentialWouldTake
  );
  
  return {
    workerPoolConfig: {
      maxConcurrency: workerPool.maxConcurrency,
      taskTimeout: workerPool.taskTimeout + 'ms',
      maxRetries: workerPool.retryPolicy.maxRetries,
    },
    parallelExecution: {
      taskCount: parallelResult.results.length,
      actualDuration: `${parallelResult.totalDuration}ms`,
      sequentialWouldTake: `${parallelResult.sequentialWouldTake}ms`,
      timeSaved: `${parallelResult.sequentialWouldTake - parallelResult.totalDuration}ms`,
      efficiency: `${efficiency}%`,
    },
    limitedConcurrency: {
      concurrencyLimit: workerPool.maxConcurrency,
      tasksCompleted: limitedResults.length,
      duration: `${limitedDuration}ms`,
    },
    racingFetch: {
      winner: raceResult.source,
      data: raceResult.data,
    },
  };
}

export default await main();
