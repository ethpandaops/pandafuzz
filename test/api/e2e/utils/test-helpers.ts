import { PandaFuzzClient } from '@pandafuzz/client';
import { readFileSync } from 'fs';
import * as path from 'path';

export interface TestConfig {
  baseUrl: string;
  timeout: number;
  maxRetries: number;
  pollInterval: number;
}

export const DEFAULT_CONFIG: TestConfig = {
  baseUrl: process.env.PANDAFUZZ_BASE_URL || 'http://localhost:8080/api/v1',
  timeout: 120000, // 2 minutes
  maxRetries: 30,
  pollInterval: 2000, // 2 seconds
};

export class TestHelpers {
  private client: PandaFuzzClient;
  private config: TestConfig;

  constructor(config: TestConfig = DEFAULT_CONFIG) {
    this.config = config;
    this.client = new PandaFuzzClient({
      baseUrl: config.baseUrl,
    });
  }

  getClient(): PandaFuzzClient {
    return this.client;
  }

  /**
   * Wait for the API to be ready
   */
  async waitForAPI(maxRetries: number = this.config.maxRetries): Promise<void> {
    for (let i = 0; i < maxRetries; i++) {
      try {
        await this.client.health.getHealth();
        return;
      } catch (error) {
        if (i === maxRetries - 1) {
          throw new Error(`API failed to become ready after ${maxRetries} attempts: ${error}`);
        }
        await this.sleep(this.config.pollInterval);
      }
    }
  }

  /**
   * Wait for system readiness
   */
  async waitForReadiness(timeoutMs: number = this.config.timeout): Promise<void> {
    await this.client.waitForReady(timeoutMs);
  }

  /**
   * Create test corpus data
   */
  createTestCorpus(type: 'normal' | 'crash' | 'mixed' = 'mixed'): Array<{ filename: string; content: Buffer; description: string }> {
    const corpus = [];

    if (type === 'normal' || type === 'mixed') {
      corpus.push(
        {
          filename: 'seed_normal.txt',
          content: Buffer.from('Hello World'),
          description: 'Normal text input'
        },
        {
          filename: 'seed_json.txt',
          content: Buffer.from('{"key": "value", "number": 42}'),
          description: 'JSON input'
        },
        {
          filename: 'seed_binary.dat',
          content: Buffer.from([0x89, 0x50, 0x4e, 0x47]), // PNG header
          description: 'Binary data'
        }
      );
    }

    if (type === 'crash' || type === 'mixed') {
      corpus.push(
        {
          filename: 'seed_overflow.txt',
          content: Buffer.from('A'.repeat(1000)),
          description: 'Long input for buffer overflow'
        },
        {
          filename: 'seed_format.txt',
          content: Buffer.from('%s%s%s%s%s%n'),
          description: 'Format string attack'
        },
        {
          filename: 'seed_null.txt',
          content: Buffer.from('\x00\x00\x00\x00'),
          description: 'Null bytes'
        }
      );
    }

    return corpus;
  }

  /**
   * Load test binary from test-resources
   */
  loadTestBinary(binaryName: string): Buffer {
    const binaryPath = path.join(__dirname, '../../../../test-resources/test-targets', binaryName);
    return readFileSync(binaryPath);
  }

  /**
   * Generate unique test name with timestamp
   */
  generateTestName(prefix: string): string {
    const timestamp = Date.now();
    const random = Math.random().toString(36).substring(2, 8);
    return `${prefix}_${timestamp}_${random}`;
  }

  /**
   * Wait for job to reach specific status
   */
  async waitForJobStatus(
    jobId: string, 
    targetStatus: string | string[], 
    timeoutMs: number = this.config.timeout
  ): Promise<any> {
    const startTime = Date.now();
    const statuses = Array.isArray(targetStatus) ? targetStatus : [targetStatus];

    while (Date.now() - startTime < timeoutMs) {
      try {
        const response = await this.client.jobs.getJob({ id: jobId });
        if (statuses.includes(response.status)) {
          return response;
        }
      } catch (error) {
        // Job might not exist yet, continue waiting
      }
      await this.sleep(this.config.pollInterval);
    }

    throw new Error(`Job ${jobId} did not reach status ${targetStatus} within ${timeoutMs}ms`);
  }

  /**
   * Wait for campaign to reach specific status
   */
  async waitForCampaignStatus(
    campaignId: string, 
    targetStatus: string | string[], 
    timeoutMs: number = this.config.timeout
  ): Promise<any> {
    const startTime = Date.now();
    const statuses = Array.isArray(targetStatus) ? targetStatus : [targetStatus];

    while (Date.now() - startTime < timeoutMs) {
      try {
        const response = await this.client.campaigns.getCampaign({ id: campaignId });
        if (statuses.includes(response.status)) {
          return response;
        }
      } catch (error) {
        // Campaign might not exist yet, continue waiting
      }
      await this.sleep(this.config.pollInterval);
    }

    throw new Error(`Campaign ${campaignId} did not reach status ${targetStatus} within ${timeoutMs}ms`);
  }

  /**
   * Wait for specific number of bots to be online
   */
  async waitForBots(minCount: number, timeoutMs: number = this.config.timeout): Promise<any[]> {
    const startTime = Date.now();

    while (Date.now() - startTime < timeoutMs) {
      try {
        const response = await this.client.bots.listBots();
        const onlineBots = response.bots?.filter(bot => bot.status === 'online') || [];
        
        if (onlineBots.length >= minCount) {
          return onlineBots;
        }
      } catch (error) {
        // Continue waiting
      }
      await this.sleep(this.config.pollInterval);
    }

    throw new Error(`Expected at least ${minCount} bots to be online within ${timeoutMs}ms`);
  }

  /**
   * Wait for crashes to be discovered
   */
  async waitForCrashes(
    jobId?: string, 
    campaignId?: string, 
    minCount: number = 1, 
    timeoutMs: number = this.config.timeout
  ): Promise<any[]> {
    const startTime = Date.now();

    while (Date.now() - startTime < timeoutMs) {
      try {
        let crashes = [];
        
        if (jobId) {
          const response = await this.client.jobs.getJobCrashes({ id: jobId });
          crashes = response.crashes || [];
        } else if (campaignId) {
          const response = await this.client.campaigns.getCampaignCrashes({ id: campaignId });
          crashes = response.crashes || [];
        } else {
          const response = await this.client.crashes.listCrashes();
          crashes = response.crashes || [];
        }

        if (crashes.length >= minCount) {
          return crashes;
        }
      } catch (error) {
        // Continue waiting
      }
      await this.sleep(this.config.pollInterval);
    }

    throw new Error(`Expected at least ${minCount} crashes within ${timeoutMs}ms`);
  }

  /**
   * Create a test corpus collection
   */
  async createTestCorpusCollection(name?: string): Promise<{ id: string; collection: any }> {
    const collectionName = name || this.generateTestName('test_corpus');
    
    const response = await this.client.corpus.createCorpusCollection({
      corpusCollectionCreateRequest: {
        name: collectionName,
        description: `Test corpus collection created at ${new Date().toISOString()}`,
        tags: ['test', 'automated', 'e2e']
      }
    });

    return { id: response.id!, collection: response };
  }

  /**
   * Upload corpus files to collection
   */
  async uploadCorpusFiles(collectionId: string, files: Array<{ filename: string; content: Buffer }>): Promise<void> {
    // Create form data
    const formData = new FormData();
    
    for (const file of files) {
      const blob = new Blob([file.content], { type: 'application/octet-stream' });
      formData.append('files', blob, file.filename);
    }

    // Upload using fetch directly since client might not support multipart
    const uploadUrl = `${this.config.baseUrl.replace('/api/v1', '')}/api/v1/corpus/collections/${collectionId}/files`;
    const response = await fetch(uploadUrl, {
      method: 'POST',
      body: formData
    });

    if (!response.ok) {
      throw new Error(`Failed to upload corpus files: ${response.statusText}`);
    }
  }

  /**
   * Clean up test resources
   */
  async cleanup(resources: { jobs?: string[]; campaigns?: string[]; collections?: string[] }): Promise<void> {
    const errors = [];

    // Clean up jobs
    if (resources.jobs) {
      for (const jobId of resources.jobs) {
        try {
          await this.client.jobs.cancelJob({ id: jobId });
          await this.client.jobs.deleteJob({ id: jobId });
        } catch (error) {
          errors.push(`Failed to cleanup job ${jobId}: ${error}`);
        }
      }
    }

    // Clean up campaigns
    if (resources.campaigns) {
      for (const campaignId of resources.campaigns) {
        try {
          await this.client.campaigns.stopCampaign({ 
            id: campaignId,
            stopCampaignRequest: { reason: 'Test cleanup' }
          });
          await this.client.campaigns.deleteCampaign({ id: campaignId });
        } catch (error) {
          errors.push(`Failed to cleanup campaign ${campaignId}: ${error}`);
        }
      }
    }

    // Clean up corpus collections
    if (resources.collections) {
      for (const collectionId of resources.collections) {
        try {
          await this.client.corpus.deleteCorpusCollection({ id: collectionId });
        } catch (error) {
          errors.push(`Failed to cleanup collection ${collectionId}: ${error}`);
        }
      }
    }

    if (errors.length > 0) {
      console.warn('Cleanup warnings:', errors);
    }
  }

  /**
   * Sleep utility
   */
  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  /**
   * Monitor events for a specific duration
   */
  async monitorEvents(
    filter: any, 
    durationMs: number = 10000
  ): Promise<any[]> {
    const events: any[] = [];
    
    return new Promise((resolve, reject) => {
      this.client.connectEvents(filter);
      
      this.client.sse.onAny((event) => {
        events.push(event);
      });

      this.client.sse.onError((error) => {
        reject(error);
      });

      setTimeout(() => {
        this.client.disconnectEvents();
        resolve(events);
      }, durationMs);
    });
  }

  /**
   * Take performance snapshot
   */
  async takePerformanceSnapshot(): Promise<{ 
    timestamp: number; 
    system: any; 
    memory: any;
    analytics?: any;
  }> {
    const timestamp = Date.now();
    
    try {
      const [healthResponse, analyticsResponse] = await Promise.allSettled([
        this.client.health.getHealth(),
        this.client.analytics.getAnalytics()
      ]);

      return {
        timestamp,
        system: healthResponse.status === 'fulfilled' ? healthResponse.value : null,
        memory: process.memoryUsage(),
        analytics: analyticsResponse.status === 'fulfilled' ? analyticsResponse.value : null
      };
    } catch (error) {
      return {
        timestamp,
        system: null,
        memory: process.memoryUsage(),
        analytics: null
      };
    }
  }
}

/**
 * Default test helper instance
 */
export const testHelpers = new TestHelpers();

/**
 * Resource tracker for cleanup
 */
export class ResourceTracker {
  private resources = {
    jobs: [] as string[],
    campaigns: [] as string[],
    collections: [] as string[]
  };

  trackJob(jobId: string): void {
    this.resources.jobs.push(jobId);
  }

  trackCampaign(campaignId: string): void {
    this.resources.campaigns.push(campaignId);
  }

  trackCollection(collectionId: string): void {
    this.resources.collections.push(collectionId);
  }

  async cleanup(): Promise<void> {
    await testHelpers.cleanup(this.resources);
    this.resources = { jobs: [], campaigns: [], collections: [] };
  }

  getTrackedResources() {
    return { ...this.resources };
  }
}

/**
 * Event collector for testing SSE functionality
 */
export class EventCollector {
  private events: any[] = [];
  private client: PandaFuzzClient;

  constructor(client: PandaFuzzClient) {
    this.client = client;
  }

  startCollecting(filter?: any): void {
    this.events = [];
    this.client.connectEvents(filter);
    
    this.client.sse.onAny((event) => {
      this.events.push({
        ...event,
        timestamp: Date.now()
      });
    });
  }

  stopCollecting(): void {
    this.client.disconnectEvents();
  }

  getEvents(): any[] {
    return [...this.events];
  }

  getEventsByType(eventType: string): any[] {
    return this.events.filter(event => event.type === eventType);
  }

  waitForEvent(
    eventType: string, 
    timeoutMs: number = 30000, 
    filter?: (event: any) => boolean
  ): Promise<any> {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error(`Event ${eventType} not received within ${timeoutMs}ms`));
      }, timeoutMs);

      const checkForEvent = () => {
        const matchingEvents = this.events.filter(event => 
          event.type === eventType && (!filter || filter(event))
        );
        
        if (matchingEvents.length > 0) {
          clearTimeout(timeout);
          resolve(matchingEvents[0]);
        } else {
          setTimeout(checkForEvent, 100);
        }
      };

      checkForEvent();
    });
  }

  clear(): void {
    this.events = [];
  }
}