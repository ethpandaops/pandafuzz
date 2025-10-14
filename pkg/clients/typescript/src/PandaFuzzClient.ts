/**
 * Enhanced PandaFuzz Client with SSE support
 */

import { Configuration, ConfigurationParameters } from './runtime';
import {
  AnalyticsApi,
  BatchApi,
  BotsApi,
  CampaignsApi,
  CorpusApi,
  CrashesApi,
  EventsApi,
  HealthApi,
  JobsApi
} from './apis';
import { SSEClient, SSEEventFilter, SSEClientOptions, EventTypes, EventType } from './sse/SSEClient';

export interface PandaFuzzClientOptions {
  baseUrl?: string;
  apiKey?: string;
  accessToken?: string;
  sse?: SSEClientOptions;
}

export interface RetryOptions {
  maxAttempts?: number;
  initialDelay?: number;
  maxDelay?: number;
  backoffFactor?: number;
}

/**
 * Enhanced PandaFuzz client with full API access and SSE support
 */
export class PandaFuzzClient {
  private configuration: Configuration;
  private _analytics: AnalyticsApi;
  private _batch: BatchApi;
  private _bots: BotsApi;
  private _campaigns: CampaignsApi;
  private _corpus: CorpusApi;
  private _crashes: CrashesApi;
  private _events: EventsApi;
  private _health: HealthApi;
  private _jobs: JobsApi;
  private _sse: SSEClient;

  constructor(options: PandaFuzzClientOptions = {}) {
    const configParams: ConfigurationParameters = {
      basePath: options.baseUrl || 'http://localhost:8080/api/v1'
    };

    if (options.apiKey) {
      configParams.apiKey = options.apiKey;
    }

    if (options.accessToken) {
      configParams.accessToken = options.accessToken;
    }

    this.configuration = new Configuration(configParams);

    // Initialize API clients
    this._analytics = new AnalyticsApi(this.configuration);
    this._batch = new BatchApi(this.configuration);
    this._bots = new BotsApi(this.configuration);
    this._campaigns = new CampaignsApi(this.configuration);
    this._corpus = new CorpusApi(this.configuration);
    this._crashes = new CrashesApi(this.configuration);
    this._events = new EventsApi(this.configuration);
    this._health = new HealthApi(this.configuration);
    this._jobs = new JobsApi(this.configuration);

    // Initialize SSE client
    const sseOptions: SSEClientOptions = {
      baseUrl: options.baseUrl,
      configuration: this.configuration,
      ...options.sse
    };
    this._sse = new SSEClient(sseOptions);
  }

  // API accessors
  public get analytics(): AnalyticsApi { return this._analytics; }
  public get batch(): BatchApi { return this._batch; }
  public get bots(): BotsApi { return this._bots; }
  public get campaigns(): CampaignsApi { return this._campaigns; }
  public get corpus(): CorpusApi { return this._corpus; }
  public get crashes(): CrashesApi { return this._crashes; }
  public get events(): EventsApi { return this._events; }
  public get health(): HealthApi { return this._health; }
  public get jobs(): JobsApi { return this._jobs; }
  public get sse(): SSEClient { return this._sse; }

  /**
   * Connect to real-time event stream
   */
  public connectEvents(filter?: SSEEventFilter): void {
    this._sse.connect(filter);
  }

  /**
   * Disconnect from real-time event stream
   */
  public disconnectEvents(): void {
    this._sse.disconnect();
  }

  /**
   * Check if connected to event stream
   */
  public get eventsConnected(): boolean {
    return this._sse.connected;
  }

  /**
   * Perform an operation with automatic retry logic
   */
  public async withRetry<T>(
    operation: () => Promise<T>,
    options: RetryOptions = {}
  ): Promise<T> {
    const {
      maxAttempts = 3,
      initialDelay = 1000,
      maxDelay = 30000,
      backoffFactor = 2
    } = options;

    let attempt = 1;
    let delay = initialDelay;

    while (attempt <= maxAttempts) {
      try {
        return await operation();
      } catch (error) {
        if (attempt === maxAttempts) {
          throw error;
        }

        // Check if error is retryable
        if (this.isRetryableError(error)) {
          await this.sleep(delay);
          delay = Math.min(delay * backoffFactor, maxDelay);
          attempt++;
        } else {
          throw error;
        }
      }
    }

    throw new Error('Maximum retry attempts exceeded');
  }

  /**
   * Health check with automatic retry
   */
  public async healthCheck(retryOptions?: RetryOptions): Promise<boolean> {
    try {
      await this.withRetry(() => this.health.getHealth(), retryOptions);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Wait for system to be ready
   */
  public async waitForReady(
    timeoutMs: number = 30000,
    intervalMs: number = 1000
  ): Promise<void> {
    const startTime = Date.now();
    
    while (Date.now() - startTime < timeoutMs) {
      try {
        await this.health.getReadiness();
        return;
      } catch {
        await this.sleep(intervalMs);
      }
    }
    
    throw new Error(`System not ready within ${timeoutMs}ms timeout`);
  }

  /**
   * Subscribe to bot status changes
   */
  public onBotStatusChange(callback: (data: any) => void): void {
    this._sse.on(EventTypes.BOT_STATUS_CHANGED, (event) => {
      callback(event.data);
    });
  }

  /**
   * Subscribe to job completion events
   */
  public onJobCompleted(callback: (data: any) => void): void {
    this._sse.on(EventTypes.JOB_COMPLETED, (event) => {
      callback(event.data);
    });
  }

  /**
   * Subscribe to crash discovery events
   */
  public onCrashDiscovered(callback: (data: any) => void): void {
    this._sse.on(EventTypes.CRASH_DISCOVERED, (event) => {
      callback(event.data);
    });
  }

  /**
   * Subscribe to campaign status changes
   */
  public onCampaignStatusChange(callback: (data: any) => void): void {
    const campaignEvents = [
      EventTypes.CAMPAIGN_STARTED,
      EventTypes.CAMPAIGN_STOPPED,
      EventTypes.CAMPAIGN_COMPLETED,
      EventTypes.CAMPAIGN_FAILED
    ];

    campaignEvents.forEach(eventType => {
      this._sse.on(eventType, (event) => {
        callback(event.data);
      });
    });
  }

  /**
   * Subscribe to all events
   */
  public onAnyEvent(callback: (event: any) => void): void {
    this._sse.onAny(callback);
  }

  /**
   * Get current configuration
   */
  public getConfiguration(): Configuration {
    return this.configuration;
  }

  /**
   * Update configuration
   */
  public updateConfiguration(params: Partial<ConfigurationParameters>): void {
    this.configuration = new Configuration({
      ...this.configuration,
      ...params
    });

    // Update all API clients
    this._analytics = new AnalyticsApi(this.configuration);
    this._batch = new BatchApi(this.configuration);
    this._bots = new BotsApi(this.configuration);
    this._campaigns = new CampaignsApi(this.configuration);
    this._corpus = new CorpusApi(this.configuration);
    this._crashes = new CrashesApi(this.configuration);
    this._events = new EventsApi(this.configuration);
    this._health = new HealthApi(this.configuration);
    this._jobs = new JobsApi(this.configuration);
  }

  /**
   * Close all connections and cleanup
   */
  public close(): void {
    this.disconnectEvents();
  }

  private isRetryableError(error: any): boolean {
    // Check for network errors or server errors (5xx)
    if (error?.status >= 500 && error?.status < 600) {
      return true;
    }

    // Check for network timeouts
    if (error?.name === 'TimeoutError' || error?.code === 'ECONNRESET') {
      return true;
    }

    // Check for rate limiting (429)
    if (error?.status === 429) {
      return true;
    }

    return false;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

// Re-export important types and constants
export { EventTypes, EventType };
export type { SSEEventFilter, SSEClientOptions };

// Re-export all models and APIs for convenience
export * from './models';
export * from './apis';
export * from './runtime';