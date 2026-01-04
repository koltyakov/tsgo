// Context: Scheduler configuration (Bun only)

interface SchedulerConfig {
  maxConcurrent: number;
  defaultTimeout: number;
  retryDelay: number;
  maxRetries: number;
}

export const schedulerConfig: SchedulerConfig = {
  maxConcurrent: 5,
  defaultTimeout: 10000,
  retryDelay: 1000,
  maxRetries: 3,
};

export function generateTaskId(): string {
  return 'task_' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
}

export function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// --- Code ---

// Task Scheduler - Async task queue with priorities and retries (Bun only)
// Demonstrates async/await, Promise patterns, and concurrent execution

type TaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
type TaskPriority = 'low' | 'normal' | 'high' | 'critical';

interface Task {
  id: string;
  name: string;
  priority: TaskPriority;
  status: TaskStatus;
  fn: () => Promise<unknown>;
  result?: unknown;
  error?: string;
  retries: number;
  createdAt: number;
  startedAt?: number;
  completedAt?: number;
}

interface SchedulerStats {
  total: number;
  pending: number;
  running: number;
  completed: number;
  failed: number;
  cancelled: number;
  avgExecutionTime: number;
}

interface TaskResult<T> {
  taskId: string;
  success: boolean;
  result?: T;
  error?: string;
  executionTime: number;
  retries: number;
}

const priorityValues: Record<TaskPriority, number> = {
  critical: 4,
  high: 3,
  normal: 2,
  low: 1,
};

class TaskScheduler {
  private tasks: Map<string, Task> = new Map();
  private running: Set<string> = new Set();
  private executionTimes: number[] = [];
  private isProcessing = false;

  // Add a task to the queue
  schedule<T>(
    name: string,
    fn: () => Promise<T>,
    options: { priority?: TaskPriority } = {}
  ): string {
    const { priority = 'normal' } = options;
    const id = generateTaskId();

    const task: Task = {
      id,
      name,
      priority,
      status: 'pending',
      fn,
      retries: 0,
      createdAt: Date.now(),
    };

    this.tasks.set(id, task);
    console.log(`[Scheduler] Task '${name}' scheduled (id: ${id}, priority: ${priority})`);

    return id;
  }

  // Cancel a pending task
  cancel(taskId: string): boolean {
    const task = this.tasks.get(taskId);
    if (!task || task.status !== 'pending') return false;

    task.status = 'cancelled';
    console.log(`[Scheduler] Task '${task.name}' cancelled`);
    return true;
  }

  // Process all tasks with concurrency limit
  async processAll(): Promise<TaskResult<unknown>[]> {
    if (this.isProcessing) {
      throw new Error('Scheduler is already processing');
    }

    this.isProcessing = true;
    const results: TaskResult<unknown>[] = [];

    try {
      while (this.hasPendingTasks()) {
        // Get next batch of tasks (up to max concurrent)
        const batch = this.getNextBatch();
        
        if (batch.length === 0) {
          // Wait for running tasks to complete
          await sleep(100);
          continue;
        }

        // Execute batch in parallel
        const batchResults = await Promise.all(
          batch.map(task => this.executeTask(task))
        );

        results.push(...batchResults);
      }
    } finally {
      this.isProcessing = false;
    }

    return results;
  }

  // Get next batch of tasks to execute
  private getNextBatch(): Task[] {
    const available = schedulerConfig.maxConcurrent - this.running.size;
    if (available <= 0) return [];

    const pending = Array.from(this.tasks.values())
      .filter(t => t.status === 'pending')
      .sort((a, b) => {
        // Sort by priority (high first), then by creation time (FIFO)
        const priorityDiff = priorityValues[b.priority] - priorityValues[a.priority];
        if (priorityDiff !== 0) return priorityDiff;
        return a.createdAt - b.createdAt;
      });

    return pending.slice(0, available);
  }

  // Execute a single task with retry support
  private async executeTask(task: Task): Promise<TaskResult<unknown>> {
    task.status = 'running';
    task.startedAt = Date.now();
    this.running.add(task.id);

    console.log(`[Scheduler] Starting task '${task.name}'`);

    let lastError: string | undefined;

    for (let attempt = 0; attempt <= schedulerConfig.maxRetries; attempt++) {
      try {
        if (attempt > 0) {
          console.log(`[Scheduler] Retrying task '${task.name}' (attempt ${attempt + 1})`);
          await sleep(schedulerConfig.retryDelay * attempt);
        }

        // Execute with timeout
        const result = await Promise.race([
          task.fn(),
          sleep(schedulerConfig.defaultTimeout).then(() => {
            throw new Error('Task timeout');
          }),
        ]);

        // Success
        task.status = 'completed';
        task.result = result;
        task.completedAt = Date.now();
        task.retries = attempt;

        const executionTime = task.completedAt - task.startedAt!;
        this.executionTimes.push(executionTime);
        this.running.delete(task.id);

        console.log(`[Scheduler] Task '${task.name}' completed in ${executionTime}ms`);

        return {
          taskId: task.id,
          success: true,
          result,
          executionTime,
          retries: attempt,
        };
      } catch (err) {
        lastError = (err as Error).message;
        task.retries = attempt;
      }
    }

    // All retries exhausted
    task.status = 'failed';
    task.error = lastError;
    task.completedAt = Date.now();
    
    const executionTime = task.completedAt - task.startedAt!;
    this.running.delete(task.id);

    console.log(`[Scheduler] Task '${task.name}' failed: ${lastError}`);

    return {
      taskId: task.id,
      success: false,
      error: lastError,
      executionTime,
      retries: schedulerConfig.maxRetries,
    };
  }

  // Check if there are pending tasks
  private hasPendingTasks(): boolean {
    return Array.from(this.tasks.values()).some(
      t => t.status === 'pending' || t.status === 'running'
    );
  }

  // Get scheduler statistics
  getStats(): SchedulerStats {
    const tasks = Array.from(this.tasks.values());
    const avgTime = this.executionTimes.length > 0
      ? this.executionTimes.reduce((a, b) => a + b, 0) / this.executionTimes.length
      : 0;

    return {
      total: tasks.length,
      pending: tasks.filter(t => t.status === 'pending').length,
      running: tasks.filter(t => t.status === 'running').length,
      completed: tasks.filter(t => t.status === 'completed').length,
      failed: tasks.filter(t => t.status === 'failed').length,
      cancelled: tasks.filter(t => t.status === 'cancelled').length,
      avgExecutionTime: Math.round(avgTime),
    };
  }

  // Get task details
  getTask(taskId: string): Task | undefined {
    return this.tasks.get(taskId);
  }
}

// Create scheduler and add tasks
const scheduler = new TaskScheduler();

// Simulate various async operations
const taskIds: string[] = [];

// Quick task (low priority)
taskIds.push(scheduler.schedule('Quick calculation', async () => {
  await sleep(50);
  return { value: 42 };
}, { priority: 'low' }));

// API fetch simulation (normal priority)
taskIds.push(scheduler.schedule('Fetch user data', async () => {
  await sleep(200);
  return { userId: 123, name: 'Alice' };
}));

// Database query simulation (high priority)
taskIds.push(scheduler.schedule('Database query', async () => {
  await sleep(150);
  return { records: 100 };
}, { priority: 'high' }));

// Critical system task
taskIds.push(scheduler.schedule('Health check', async () => {
  await sleep(30);
  return { status: 'healthy' };
}, { priority: 'critical' }));

// Task that might fail (demonstrates retry)
let attemptCount = 0;
taskIds.push(scheduler.schedule('Flaky service call', async () => {
  attemptCount++;
  if (attemptCount < 2) {
    throw new Error('Service temporarily unavailable');
  }
  return { attempt: attemptCount, success: true };
}));

// Batch of parallel tasks
for (let i = 1; i <= 3; i++) {
  taskIds.push(scheduler.schedule(`Batch task ${i}`, async () => {
    await sleep(100 + i * 20);
    return { batchIndex: i };
  }));
}

// Cancel one task
const cancelledId = taskIds[taskIds.length - 1];
scheduler.cancel(cancelledId);

// Process all tasks
const results = await scheduler.processAll();

// Get final statistics
const stats = scheduler.getStats();

// Define result type for clear contract
interface TaskResultSummary {
  taskId: string;
  success: boolean;
  executionTime: number;
  retries: number;
  result: unknown;
  error: string | undefined;
}

interface PlaygroundResult {
  config: SchedulerConfig;
  scheduledTasks: number;
  cancelledTask: string;
  results: TaskResultSummary[];
  statistics: SchedulerStats;
  summary: {
    successRate: string;
    avgExecutionTime: string;
  };
}

const result: PlaygroundResult = {
  config: schedulerConfig,
  scheduledTasks: taskIds.length,
  cancelledTask: cancelledId,
  results: results.map(r => ({
    taskId: r.taskId,
    success: r.success,
    executionTime: r.executionTime,
    retries: r.retries,
    result: r.result,
    error: r.error,
  })),
  statistics: stats,
  summary: {
    successRate: `${Math.round((stats.completed / stats.total) * 100)}%`,
    avgExecutionTime: `${stats.avgExecutionTime}ms`,
  },
};

export default result;
