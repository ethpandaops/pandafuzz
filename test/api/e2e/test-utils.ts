/**
 * Test utilities for PandaFuzz API v1 e2e tests
 */

import { APIRequestContext, expect } from '@playwright/test';
import { PandaFuzzClient } from '@pandafuzz/client';

export interface TestConfig {
  baseUrl: string;
  apiTimeout: number;
  pollInterval: number;
  maxWaitTime: number;
}

export interface BotRegistrationData {
  id: string;
  name: string;
  capabilities: {
    fuzzers: string[];
    max_concurrent_jobs: number;
    supported_platforms: string[];
    memory_mb: number;
    cpu_cores: number;
  };
  tags?: { [key: string]: string };
}

export interface JobCreationData {
  name: string;
  description: string;
  fuzzer_type: 'afl++' | 'libfuzzer' | 'honggfuzz';
  target_binary: string;
  duration: number;
  memory_limit: number;
  timeout: number;
  arguments: string;
  corpus_files?: string[];
}

export interface CampaignCreationData {
  name: string;
  description: string;
  job_template: {
    fuzzer_type: 'afl++' | 'libfuzzer' | 'honggfuzz';
    target_binary: string;
    duration: number;
    memory_limit: number;
    timeout: number;
    arguments: string;
  };
  max_concurrent_jobs: number;
  total_jobs: number;
}

/**
 * Setup and teardown utilities
 */
export class TestSetup {
  private client: PandaFuzzClient;
  private trackedResources: {
    bots: string[];
    jobs: string[];
    campaigns: string[];
    crashes: string[];
  };

  constructor(client: PandaFuzzClient) {
    this.client = client;
    this.trackedResources = {
      bots: [],
      jobs: [],
      campaigns: [],
      crashes: []
    };
  }

  /**
   * Track a resource for cleanup
   */
  trackBot(botId: string): void {
    this.trackedResources.bots.push(botId);
  }

  trackJob(jobId: string): void {
    this.trackedResources.jobs.push(jobId);
  }

  trackCampaign(campaignId: string): void {
    this.trackedResources.campaigns.push(campaignId);
  }

  trackCrash(crashId: string): void {
    this.trackedResources.crashes.push(crashId);
  }

  /**
   * Clean up all tracked resources
   */
  async cleanup(): Promise<void> {
    console.log('🧹 Cleaning up test resources...');

    // Clean up campaigns first (they may have dependent jobs)
    for (const campaignId of this.trackedResources.campaigns) {
      try {
        await this.client.campaigns.stopCampaign({ 
          id: campaignId, 
          stopCampaignRequest: { reason: 'Test cleanup' }
        });
        await this.client.campaigns.deleteCampaign({ id: campaignId });
      } catch (error) {
        console.warn(`Failed to cleanup campaign ${campaignId}:`, error);
      }
    }

    // Clean up jobs
    for (const jobId of this.trackedResources.jobs) {
      try {
        await this.client.jobs.cancelJob({ id: jobId });
        await this.client.jobs.deleteJob({ id: jobId });
      } catch (error) {
        console.warn(`Failed to cleanup job ${jobId}:`, error);
      }
    }

    // Clean up bots
    for (const botId of this.trackedResources.bots) {
      try {
        await this.client.bots.deleteBot({ id: botId });
      } catch (error) {
        console.warn(`Failed to cleanup bot ${botId}:`, error);
      }
    }

    // Clean up crashes (if API supports deletion)
    for (const crashId of this.trackedResources.crashes) {
      try {
        // Note: Assuming there's a delete crash endpoint
        // await this.client.crashes.deleteCrash({ id: crashId });
        console.log(`Crash ${crashId} tracked for cleanup`);
      } catch (error) {
        console.warn(`Failed to cleanup crash ${crashId}:`, error);
      }
    }

    // Clear tracking arrays
    this.trackedResources.bots = [];
    this.trackedResources.jobs = [];
    this.trackedResources.campaigns = [];
    this.trackedResources.crashes = [];

    console.log('✅ Test resources cleaned up');
  }
}

/**
 * Event collection utility for testing SSE events
 */
export class EventCollector {
  private client: PandaFuzzClient;
  private events: any[] = [];
  private isCollecting = false;
  private eventPromises: Map<string, { resolve: Function, reject: Function, timeout: NodeJS.Timeout }> = new Map();

  constructor(client: PandaFuzzClient) {
    this.client = client;
  }

  /**
   * Start collecting events
   */
  startCollecting(filter?: { types?: string[] }): void {
    if (this.isCollecting) return;
    
    this.isCollecting = true;
    this.events = [];

    // Connect to SSE stream
    this.client.connectEvents(filter);

    // Listen for all events
    this.client.sse.onAny((event) => {
      this.events.push({
        ...event,
        timestamp: new Date()
      });

      // Resolve any waiting promises
      const key = this.getEventKey(event.type, event.data);
      const promise = this.eventPromises.get(key);
      if (promise) {
        clearTimeout(promise.timeout);
        this.eventPromises.delete(key);
        promise.resolve(event);
      }
    });
  }

  /**
   * Stop collecting events
   */
  stopCollecting(): void {
    if (!this.isCollecting) return;
    
    this.isCollecting = false;
    this.client.disconnectEvents();

    // Reject any pending promises
    for (const [key, promise] of this.eventPromises) {
      clearTimeout(promise.timeout);
      promise.reject(new Error(`Event collection stopped while waiting for ${key}`));
    }
    this.eventPromises.clear();
  }

  /**
   * Wait for a specific event
   */
  async waitForEvent(
    eventType: string, 
    timeoutMs: number = 10000,
    predicate?: (event: any) => boolean
  ): Promise<any> {
    return new Promise((resolve, reject) => {
      const key = eventType + (predicate ? '_filtered' : '');
      
      // Check if event already exists in collected events
      const existingEvent = this.events.find(event => {
        if (event.type !== eventType) return false;
        return predicate ? predicate(event) : true;
      });

      if (existingEvent) {
        resolve(existingEvent);
        return;
      }

      // Set up timeout
      const timeout = setTimeout(() => {
        this.eventPromises.delete(key);
        reject(new Error(`Timeout waiting for event ${eventType} after ${timeoutMs}ms`));
      }, timeoutMs);

      // Store promise for resolution when event arrives
      this.eventPromises.set(key, { resolve, reject, timeout });
    });
  }

  /**
   * Get collected events
   */
  getEvents(): any[] {
    return [...this.events];
  }

  /**
   * Get events of a specific type
   */
  getEventsOfType(eventType: string): any[] {
    return this.events.filter(event => event.type === eventType);
  }

  /**
   * Clear collected events
   */
  clearEvents(): void {
    this.events = [];
  }

  private getEventKey(eventType: string, eventData?: any): string {
    // Simple key generation for event promises
    return eventType;
  }
}

/**
 * Data generators for test fixtures
 */
export class DataGenerators {
  /**
   * Generate unique test ID
   */
  static generateId(prefix: string = 'test'): string {
    return `${prefix}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
  }

  /**
   * Generate bot registration data
   */
  static createBotRegistration(id?: string): BotRegistrationData {
    const botId = id || this.generateId('bot');
    
    return {
      id: botId,
      name: `Test Bot ${botId}`,
      capabilities: {
        fuzzers: ['afl++', 'libfuzzer', 'honggfuzz'],
        max_concurrent_jobs: 4,
        supported_platforms: ['linux'],
        memory_mb: 8192,
        cpu_cores: 4
      },
      tags: {
        environment: 'test',
        purpose: 'e2e-testing'
      }
    };
  }

  /**
   * Generate job creation data
   */
  static createJobData(overrides?: Partial<JobCreationData>): JobCreationData {
    const defaults: JobCreationData = {
      name: this.generateId('job'),
      description: 'End-to-end test job',
      fuzzer_type: 'libfuzzer',
      target_binary: '/test-resources/test-targets/simple-test',
      duration: 60, // 1 minute
      memory_limit: 256 * 1024 * 1024, // 256MB
      timeout: 1000, // 1 second
      arguments: '@@',
    };

    return { ...defaults, ...overrides };
  }

  /**
   * Generate campaign creation data
   */
  static createCampaignData(overrides?: Partial<CampaignCreationData>): CampaignCreationData {
    const defaults: CampaignCreationData = {
      name: this.generateId('campaign'),
      description: 'End-to-end test campaign',
      job_template: {
        fuzzer_type: 'libfuzzer',
        target_binary: '/test-resources/test-targets/simple-test',
        duration: 60,
        memory_limit: 256 * 1024 * 1024,
        timeout: 1000,
        arguments: '@@',
      },
      max_concurrent_jobs: 2,
      total_jobs: 5
    };

    return { ...defaults, ...overrides };
  }

  /**
   * Generate resource usage data
   */
  static createResourceUsage(overrides?: any) {
    return {
      cpu_percent: 25.0,
      memory_percent: 40.0,
      disk_percent: 15.0,
      active_jobs: 0,
      ...overrides
    };
  }
}

/**
 * API testing utilities
 */
export class ApiHelpers {
  static client: PandaFuzzClient;

  static init(baseUrl: string = 'http://localhost:8080/api/v1'): void {
    this.client = new PandaFuzzClient({ baseUrl });
  }

  /**
   * Wait for job to reach a specific status
   */
  static async waitForJobStatus(
    jobId: string, 
    targetStatuses: string[], 
    timeoutMs: number = 30000
  ): Promise<any> {
    const startTime = Date.now();
    const pollInterval = 1000; // 1 second

    while (Date.now() - startTime < timeoutMs) {
      try {
        const job = await this.client.jobs.getJob({ id: jobId });
        
        if (targetStatuses.includes(job.status || '')) {
          return job;
        }
        
        await this.sleep(pollInterval);
      } catch (error) {
        console.warn(`Error polling job status: ${error}`);
        await this.sleep(pollInterval);
      }
    }

    throw new Error(`Job ${jobId} did not reach status [${targetStatuses.join(', ')}] within ${timeoutMs}ms`);
  }

  /**
   * Wait for campaign to reach a specific status
   */
  static async waitForCampaignStatus(
    campaignId: string, 
    targetStatuses: string[], 
    timeoutMs: number = 60000
  ): Promise<any> {
    const startTime = Date.now();
    const pollInterval = 2000; // 2 seconds

    while (Date.now() - startTime < timeoutMs) {
      try {
        const campaign = await this.client.campaigns.getCampaign({ id: campaignId });
        
        if (targetStatuses.includes(campaign.status || '')) {
          return campaign;
        }
        
        await this.sleep(pollInterval);
      } catch (error) {
        console.warn(`Error polling campaign status: ${error}`);
        await this.sleep(pollInterval);
      }
    }

    throw new Error(`Campaign ${campaignId} did not reach status [${targetStatuses.join(', ')}] within ${timeoutMs}ms`);
  }

  /**
   * Wait for bot to reach a specific status
   */
  static async waitForBotStatus(
    botId: string, 
    targetStatus: string, 
    timeoutMs: number = 15000
  ): Promise<any> {
    const startTime = Date.now();
    const pollInterval = 1000; // 1 second

    while (Date.now() - startTime < timeoutMs) {
      try {
        const bot = await this.client.bots.getBot({ id: botId });
        
        if (bot.status === targetStatus) {
          return bot;
        }
        
        await this.sleep(pollInterval);
      } catch (error) {
        console.warn(`Error polling bot status: ${error}`);
        await this.sleep(pollInterval);
      }
    }

    throw new Error(`Bot ${botId} did not reach status ${targetStatus} within ${timeoutMs}ms`);
  }

  /**
   * Sleep utility
   */
  static sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

/**
 * Custom Playwright matchers for API testing
 */
export function addCustomMatchers() {
  expect.extend({
    toHaveValidTimestamp(received: any) {
      const timestamp = typeof received === 'string' ? new Date(received) : received;
      const isValid = timestamp instanceof Date && !isNaN(timestamp.getTime());
      
      return {
        pass: isValid,
        message: () => isValid 
          ? `Expected ${received} not to be a valid timestamp`
          : `Expected ${received} to be a valid timestamp`
      };
    },

    toMatchApiResponse(received: any) {
      const hasRequiredFields = received && typeof received === 'object';
      
      return {
        pass: hasRequiredFields,
        message: () => hasRequiredFields
          ? `Expected response not to match API structure`
          : `Expected response to match API structure with required fields`
      };
    },

    toBeValidResourceUsage(received: any) {
      const isValid = received &&
        typeof received.cpu_percent === 'number' &&
        typeof received.memory_percent === 'number' &&
        typeof received.disk_percent === 'number' &&
        received.cpu_percent >= 0 && received.cpu_percent <= 100 &&
        received.memory_percent >= 0 && received.memory_percent <= 100 &&
        received.disk_percent >= 0 && received.disk_percent <= 100;

      return {
        pass: isValid,
        message: () => isValid
          ? `Expected resource usage not to be valid`
          : `Expected resource usage to have valid percentage values`
      };
    }
  });
}