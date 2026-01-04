// Context: Event system configuration

interface EventConfig {
  maxListeners: number;
  asyncDefault: boolean;
  debugMode: boolean;
}

export const eventConfig: EventConfig = {
  maxListeners: 100,
  asyncDefault: false,
  debugMode: true,
};

export function formatEventName(namespace: string, event: string): string {
  return `${namespace}:${event}`;
}

export function timestamp(): string {
  return new Date().toISOString();
}

// --- Code ---

// Event Emitter - Type-safe observer pattern implementation
// Demonstrates generics, callbacks, and event-driven architecture

type EventHandler<T = unknown> = (data: T) => void;

interface EventSubscription {
  id: string;
  event: string;
  priority: number;
  once: boolean;
  handler: EventHandler;
}

interface EmitResult {
  event: string;
  handled: boolean;
  listenerCount: number;
  errors: string[];
}

interface EventStats {
  event: string;
  emitCount: number;
  listenerCount: number;
  lastEmit?: string;
}

// Type-safe event emitter
class EventEmitter<TEvents extends { [K: string]: unknown }> {
  private subscriptions: EventSubscription[] = [];
  private idCounter = 0;
  private stats: Map<string, EventStats> = new Map();
  private history: { event: string; timestamp: string; data: unknown }[] = [];

  // Subscribe to an event
  on<K extends keyof TEvents>(
    event: K,
    handler: EventHandler<TEvents[K]>,
    options: { priority?: number; once?: boolean } = {}
  ): string {
    const id = `sub_${++this.idCounter}`;
    const { priority = 0, once = false } = options;

    this.subscriptions.push({
      id,
      event: event as string,
      priority,
      once,
      handler: handler as EventHandler,
    });

    // Sort by priority (higher first)
    this.subscriptions.sort((a, b) => b.priority - a.priority);

    // Update stats
    this.updateStats(event as string);

    if (eventConfig.debugMode) {
      console.log(`[EventEmitter] Subscribed to '${String(event)}' (id: ${id})`);
    }

    return id;
  }

  // Subscribe once
  once<K extends keyof TEvents>(
    event: K,
    handler: EventHandler<TEvents[K]>,
    priority = 0
  ): string {
    return this.on(event, handler, { priority, once: true });
  }

  // Unsubscribe by id
  off(subscriptionId: string): boolean {
    const index = this.subscriptions.findIndex(s => s.id === subscriptionId);
    if (index === -1) return false;

    const [removed] = this.subscriptions.splice(index, 1);
    this.updateStats(removed.event);

    if (eventConfig.debugMode) {
      console.log(`[EventEmitter] Unsubscribed '${removed.event}' (id: ${subscriptionId})`);
    }

    return true;
  }

  // Remove all listeners for an event
  removeAllListeners<K extends keyof TEvents>(event?: K): number {
    if (event === undefined) {
      const count = this.subscriptions.length;
      this.subscriptions = [];
      this.stats.clear();
      return count;
    }

    const eventStr = event as string;
    const before = this.subscriptions.length;
    this.subscriptions = this.subscriptions.filter(s => s.event !== eventStr);
    this.updateStats(eventStr);
    return before - this.subscriptions.length;
  }

  // Emit an event
  emit<K extends keyof TEvents>(event: K, data: TEvents[K]): EmitResult {
    const eventStr = event as string;
    const listeners = this.subscriptions.filter(s => s.event === eventStr);
    const errors: string[] = [];
    const toRemove: string[] = [];

    // Record in history
    this.history.push({
      event: eventStr,
      timestamp: timestamp(),
      data,
    });

    // Keep only last 50 events in history
    if (this.history.length > 50) {
      this.history = this.history.slice(-50);
    }

    // Call handlers
    for (const sub of listeners) {
      try {
        sub.handler(data);
        if (sub.once) {
          toRemove.push(sub.id);
        }
      } catch (err) {
        errors.push(`Handler ${sub.id}: ${(err as Error).message}`);
      }
    }

    // Remove one-time handlers
    for (const id of toRemove) {
      this.off(id);
    }

    // Update stats
    const stats = this.stats.get(eventStr);
    if (stats) {
      stats.emitCount++;
      stats.lastEmit = timestamp();
    }

    if (eventConfig.debugMode) {
      console.log(`[EventEmitter] Emitted '${eventStr}' to ${listeners.length} listeners`);
    }

    return {
      event: eventStr,
      handled: listeners.length > 0,
      listenerCount: listeners.length,
      errors,
    };
  }

  // Get listener count for an event
  listenerCount<K extends keyof TEvents>(event: K): number {
    return this.subscriptions.filter(s => s.event === event as string).length;
  }

  // Get all events with listeners
  eventNames(): string[] {
    return [...new Set(this.subscriptions.map(s => s.event))];
  }

  // Get statistics
  getStats(): EventStats[] {
    return Array.from(this.stats.values());
  }

  // Get event history
  getHistory(): typeof this.history {
    return [...this.history];
  }

  private updateStats(event: string): void {
    const count = this.subscriptions.filter(s => s.event === event).length;
    const existing = this.stats.get(event);
    
    if (count === 0) {
      this.stats.delete(event);
    } else if (existing) {
      existing.listenerCount = count;
    } else {
      this.stats.set(event, {
        event,
        emitCount: 0,
        listenerCount: count,
      });
    }
  }
}

// Define event types for a user system
interface UserEvents {
  [key: string]: unknown;
  'user:login': { userId: string; timestamp: string };
  'user:logout': { userId: string; sessionDuration: number };
  'user:update': { userId: string; changes: Record<string, unknown> };
  'user:error': { userId: string; error: string };
}

// Create typed event emitter
const userEmitter = new EventEmitter<UserEvents>();

// Track all received events
const receivedEvents: { event: string; data: unknown }[] = [];

// Subscribe to events with different priorities
const loginSub1 = userEmitter.on('user:login', (data) => {
  receivedEvents.push({ event: 'login-analytics', data });
}, { priority: 10 });

const loginSub2 = userEmitter.on('user:login', (data) => {
  receivedEvents.push({ event: 'login-security', data });
}, { priority: 100 }); // Higher priority, runs first

const logoutSub = userEmitter.once('user:logout', (data) => {
  receivedEvents.push({ event: 'logout-once', data });
}); // Only fires once

const errorSub = userEmitter.on('user:error', (data) => {
  receivedEvents.push({ event: 'error-handler', data });
});

// Emit some events
const loginResult = userEmitter.emit('user:login', {
  userId: 'user123',
  timestamp: timestamp(),
});

const updateResult = userEmitter.emit('user:update', {
  userId: 'user123',
  changes: { name: 'John Doe', email: 'john@example.com' },
});

// First logout (triggers once handler)
const logoutResult1 = userEmitter.emit('user:logout', {
  userId: 'user123',
  sessionDuration: 3600,
});

// Second logout (once handler already removed)
const logoutResult2 = userEmitter.emit('user:logout', {
  userId: 'user123',
  sessionDuration: 1800,
});

// Remove a subscription
userEmitter.off(loginSub1);

// Final login (only security handler remains)
const loginResult2 = userEmitter.emit('user:login', {
  userId: 'user456',
  timestamp: timestamp(),
});

// Define result type for clear contract
interface PlaygroundResult {
  eventConfig: EventConfig;
  subscriptionIds: {
    loginAnalytics: string;
    loginSecurity: string;
    logoutOnce: string;
    errorHandler: string;
  };
  emitResults: {
    firstLogin: EmitResult;
    update: EmitResult;
    firstLogout: EmitResult;
    secondLogout: EmitResult;
    secondLogin: EmitResult;
  };
  currentState: {
    activeEvents: string[];
    stats: EventStats[];
    historyCount: number;
  };
  receivedEvents: { event: string; data: unknown }[];
}

const result: PlaygroundResult = {
  eventConfig,
  subscriptionIds: {
    loginAnalytics: loginSub1,
    loginSecurity: loginSub2,
    logoutOnce: logoutSub,
    errorHandler: errorSub,
  },
  emitResults: {
    firstLogin: loginResult,
    update: updateResult,
    firstLogout: logoutResult1,
    secondLogout: logoutResult2,
    secondLogin: loginResult2,
  },
  currentState: {
    activeEvents: userEmitter.eventNames(),
    stats: userEmitter.getStats(),
    historyCount: userEmitter.getHistory().length,
  },
  receivedEvents,
};

export default result;
