/**
 * Server-Sent Events (SSE) client for PandaFuzz real-time event streaming
 */

import EventSource from 'eventsource';
import { Configuration } from '../runtime';

export interface SSEEventData {
  id?: string;
  type: string;
  data: any;
  timestamp: string;
}

export interface SSEEventFilter {
  types?: string[];
  campaignId?: string;
  botId?: string;
}

export interface SSEClientOptions {
  baseUrl?: string;
  configuration?: Configuration;
  reconnectInterval?: number;
  maxReconnectAttempts?: number;
  debug?: boolean;
}

export interface SSEEventListener {
  (event: SSEEventData): void;
}

export interface SSEErrorListener {
  (error: Error): void;
}

export interface SSEConnectionListener {
  (): void;
}

/**
 * Enhanced SSE client for PandaFuzz real-time events
 */
export class SSEClient {
  private eventSource?: EventSource;
  private baseUrl: string;
  private configuration?: Configuration;
  private reconnectInterval: number;
  private maxReconnectAttempts: number;
  private debug: boolean;
  private reconnectAttempts: number = 0;
  private isConnected: boolean = false;
  private isReconnecting: boolean = false;

  private eventListeners: Map<string, Set<SSEEventListener>> = new Map();
  private errorListeners: Set<SSEErrorListener> = new Set();
  private openListeners: Set<SSEConnectionListener> = new Set();
  private closeListeners: Set<SSEConnectionListener> = new Set();

  constructor(options: SSEClientOptions = {}) {
    this.baseUrl = options.baseUrl || 'http://localhost:8080/api/v1';
    this.configuration = options.configuration;
    this.reconnectInterval = options.reconnectInterval || 5000;
    this.maxReconnectAttempts = options.maxReconnectAttempts || 10;
    this.debug = options.debug || false;
  }

  /**
   * Connect to the SSE event stream
   */
  public connect(filter: SSEEventFilter = {}): void {
    if (this.eventSource) {
      this.disconnect();
    }

    const url = this.buildEventStreamUrl(filter);
    const headers: { [key: string]: string } = {};

    // Add authentication headers
    if (this.configuration?.apiKey) {
      const apiKey = typeof this.configuration.apiKey === 'function' 
        ? this.configuration.apiKey('X-API-Key') 
        : this.configuration.apiKey;
      
      if (typeof apiKey === 'string') {
        headers['X-API-Key'] = apiKey;
      } else if (apiKey instanceof Promise) {
        // For async API keys, we need to handle this differently
        apiKey.then(key => {
          if (key) headers['X-API-Key'] = key;
        });
      }
    }

    if (this.configuration?.accessToken) {
      const token = typeof this.configuration.accessToken === 'function'
        ? this.configuration.accessToken('bearerAuth', [])
        : this.configuration.accessToken;
      
      if (typeof token === 'string') {
        headers['Authorization'] = `Bearer ${token}`;
      } else if (token instanceof Promise) {
        token.then(tokenValue => {
          if (tokenValue) headers['Authorization'] = `Bearer ${tokenValue}`;
        });
      }
    }

    this.eventSource = new EventSource(url, { headers });

    this.eventSource.onopen = () => {
      this.isConnected = true;
      this.isReconnecting = false;
      this.reconnectAttempts = 0;
      this.log('SSE connection opened');
      this.openListeners.forEach(listener => listener());
    };

    this.eventSource.onmessage = (event) => {
      try {
        const eventData: SSEEventData = {
          id: event.lastEventId,
          type: event.type || 'message',
          data: JSON.parse(event.data),
          timestamp: new Date().toISOString()
        };
        this.handleEvent(eventData);
      } catch (error) {
        this.handleError(new Error(`Failed to parse SSE event data: ${error instanceof Error ? error.message : 'Unknown error'}`));
      }
    };

    this.eventSource.onerror = (error) => {
      this.isConnected = false;
      this.log(`SSE connection error:`, error);
      
      if (!this.isReconnecting && this.reconnectAttempts < this.maxReconnectAttempts) {
        this.attemptReconnect(filter);
      } else if (this.reconnectAttempts >= this.maxReconnectAttempts) {
        this.handleError(new Error(`Max reconnection attempts (${this.maxReconnectAttempts}) exceeded`));
      }
    };

    // Listen for specific event types
    const eventTypes = [
      'bot.status_changed',
      'job.created',
      'job.completed',
      'job.failed',
      'campaign.started',
      'campaign.stopped',
      'campaign.completed',
      'corpus.updated',
      'crash.discovered',
      'system.alert'
    ];

    eventTypes.forEach(eventType => {
      if (this.eventSource) {
        this.eventSource.addEventListener(eventType, (event: any) => {
          try {
            const eventData: SSEEventData = {
              id: event.lastEventId,
              type: eventType,
              data: JSON.parse(event.data),
              timestamp: new Date().toISOString()
            };
            this.handleEvent(eventData);
          } catch (error) {
            this.handleError(new Error(`Failed to parse ${eventType} event: ${error instanceof Error ? error.message : 'Unknown error'}`));
          }
        });
      }
    });
  }

  /**
   * Disconnect from the SSE event stream
   */
  public disconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = undefined;
      this.isConnected = false;
      this.isReconnecting = false;
      this.log('SSE connection closed');
      this.closeListeners.forEach(listener => listener());
    }
  }

  /**
   * Subscribe to specific event types
   */
  public on(eventType: string, listener: SSEEventListener): void {
    if (!this.eventListeners.has(eventType)) {
      this.eventListeners.set(eventType, new Set());
    }
    this.eventListeners.get(eventType)!.add(listener);
  }

  /**
   * Subscribe to all events
   */
  public onAny(listener: SSEEventListener): void {
    this.on('*', listener);
  }

  /**
   * Unsubscribe from specific event types
   */
  public off(eventType: string, listener?: SSEEventListener): void {
    const listeners = this.eventListeners.get(eventType);
    if (!listeners) return;

    if (listener) {
      listeners.delete(listener);
    } else {
      listeners.clear();
    }

    if (listeners.size === 0) {
      this.eventListeners.delete(eventType);
    }
  }

  /**
   * Subscribe to error events
   */
  public onError(listener: SSEErrorListener): void {
    this.errorListeners.add(listener);
  }

  /**
   * Subscribe to connection open events
   */
  public onOpen(listener: SSEConnectionListener): void {
    this.openListeners.add(listener);
  }

  /**
   * Subscribe to connection close events
   */
  public onClose(listener: SSEConnectionListener): void {
    this.closeListeners.add(listener);
  }

  /**
   * Get connection status
   */
  public get connected(): boolean {
    return this.isConnected;
  }

  /**
   * Get reconnection status
   */
  public get reconnecting(): boolean {
    return this.isReconnecting;
  }

  /**
   * Get current reconnection attempt count
   */
  public get reconnectionAttempts(): number {
    return this.reconnectAttempts;
  }

  private buildEventStreamUrl(filter: SSEEventFilter): string {
    const url = new URL('/events', this.baseUrl);
    
    if (filter.types && filter.types.length > 0) {
      url.searchParams.set('types', filter.types.join(','));
    }
    
    if (filter.campaignId) {
      url.searchParams.set('campaign_id', filter.campaignId);
    }
    
    if (filter.botId) {
      url.searchParams.set('bot_id', filter.botId);
    }

    return url.toString();
  }

  private handleEvent(eventData: SSEEventData): void {
    this.log(`Received event: ${eventData.type}`, eventData);

    // Call specific event type listeners
    const typeListeners = this.eventListeners.get(eventData.type);
    if (typeListeners) {
      typeListeners.forEach(listener => {
        try {
          listener(eventData);
        } catch (error) {
          this.handleError(new Error(`Event listener error: ${error instanceof Error ? error.message : 'Unknown error'}`));
        }
      });
    }

    // Call wildcard listeners
    const wildcardListeners = this.eventListeners.get('*');
    if (wildcardListeners) {
      wildcardListeners.forEach(listener => {
        try {
          listener(eventData);
        } catch (error) {
          this.handleError(new Error(`Wildcard event listener error: ${error instanceof Error ? error.message : 'Unknown error'}`));
        }
      });
    }
  }

  private handleError(error: Error): void {
    this.log(`SSE Error: ${error.message}`);
    this.errorListeners.forEach(listener => {
      try {
        listener(error);
      } catch (listenerError) {
        console.error('Error in error listener:', listenerError);
      }
    });
  }

  private attemptReconnect(filter: SSEEventFilter): void {
    if (this.isReconnecting) return;
    
    this.isReconnecting = true;
    this.reconnectAttempts++;
    
    this.log(`Attempting to reconnect (${this.reconnectAttempts}/${this.maxReconnectAttempts}) in ${this.reconnectInterval}ms`);
    
    setTimeout(() => {
      if (this.isReconnecting) {
        this.connect(filter);
      }
    }, this.reconnectInterval);
  }

  private log(message: string, data?: any): void {
    if (this.debug) {
      if (data) {
        console.log(`[PandaFuzz SSE] ${message}`, data);
      } else {
        console.log(`[PandaFuzz SSE] ${message}`);
      }
    }
  }
}

/**
 * Event type constants for type safety
 */
export const EventTypes = {
  // Bot events
  BOT_STATUS_CHANGED: 'bot.status_changed',
  BOT_REGISTERED: 'bot.registered',
  BOT_UNREGISTERED: 'bot.unregistered',
  BOT_HEARTBEAT: 'bot.heartbeat',

  // Job events  
  JOB_CREATED: 'job.created',
  JOB_STARTED: 'job.started',
  JOB_COMPLETED: 'job.completed',
  JOB_FAILED: 'job.failed',
  JOB_CANCELLED: 'job.cancelled',
  JOB_PROGRESS: 'job.progress',

  // Campaign events
  CAMPAIGN_CREATED: 'campaign.created',
  CAMPAIGN_STARTED: 'campaign.started',
  CAMPAIGN_STOPPED: 'campaign.stopped',
  CAMPAIGN_COMPLETED: 'campaign.completed',
  CAMPAIGN_FAILED: 'campaign.failed',

  // Corpus events
  CORPUS_UPDATED: 'corpus.updated',
  CORPUS_SYNCED: 'corpus.synced',
  CORPUS_QUARANTINED: 'corpus.quarantined',

  // Crash events
  CRASH_DISCOVERED: 'crash.discovered',
  CRASH_DEDUPLICATED: 'crash.deduplicated',
  CRASH_MINIMIZED: 'crash.minimized',
  CRASH_REPRODUCED: 'crash.reproduced',

  // System events
  SYSTEM_ALERT: 'system.alert',
  SYSTEM_MAINTENANCE: 'system.maintenance',
  SYSTEM_STATUS: 'system.status'
} as const;

export type EventType = typeof EventTypes[keyof typeof EventTypes];