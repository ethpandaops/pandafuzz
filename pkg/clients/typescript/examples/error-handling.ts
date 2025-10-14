/**
 * Error Handling Patterns Example
 * 
 * This example demonstrates comprehensive error handling patterns
 * and resilient programming practices with the PandaFuzz client.
 */

import { 
  PandaFuzzClient, 
  ProblemDetails,
  Configuration 
} from '@pandafuzz/client';

/**
 * Custom error classes for better error handling
 */
class PandaFuzzError extends Error {
  constructor(
    message: string,
    public readonly status?: number,
    public readonly details?: ProblemDetails
  ) {
    super(message);
    this.name = 'PandaFuzzError';
  }
}

class NetworkError extends PandaFuzzError {
  constructor(message: string, status?: number) {
    super(message, status);
    this.name = 'NetworkError';
  }
}

class AuthenticationError extends PandaFuzzError {
  constructor(message: string = 'Authentication failed') {
    super(message, 401);
    this.name = 'AuthenticationError';
  }
}

class RateLimitError extends PandaFuzzError {
  constructor(message: string = 'Rate limit exceeded', retryAfter?: number) {
    super(message, 429);
    this.name = 'RateLimitError';
    this.retryAfter = retryAfter;
  }

  public readonly retryAfter?: number;
}

class ValidationError extends PandaFuzzError {
  constructor(message: string, public readonly validationErrors?: string[]) {
    super(message, 400);
    this.name = 'ValidationError';
  }
}

/**
 * Enhanced client with comprehensive error handling
 */
class ResilientPandaFuzzClient {
  private client: PandaFuzzClient;
  private retryCount: Map<string, number> = new Map();
  private circuitBreakerState: Map<string, { failures: number, lastFailure: Date, isOpen: boolean }> = new Map();

  constructor(baseUrl: string = 'http://localhost:8080/api/v1', apiKey?: string) {
    this.client = new PandaFuzzClient({
      baseUrl,
      apiKey
    });
  }

  /**
   * Enhanced health check with comprehensive error handling
   */
  async healthCheck(): Promise<boolean> {
    try {
      console.log('🔍 Performing health check...');
      
      await this.client.health.getHealth();
      console.log('✅ System is healthy');
      this.resetCircuitBreaker('health');
      return true;
      
    } catch (error) {
      console.error('❌ Health check failed:', this.formatError(error));
      this.recordFailure('health');
      return false;
    }
  }

  /**
   * Resilient bot operations with automatic retry
   */
  async listBotsWithRetry(maxAttempts: number = 3): Promise<any[]> {
    const operation = 'listBots';
    
    if (this.isCircuitBreakerOpen(operation)) {
      throw new Error(`Circuit breaker is open for ${operation}`);
    }

    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        console.log(`🤖 Listing bots (attempt ${attempt}/${maxAttempts})...`);
        
        const result = await this.client.bots.listBots();
        
        // Reset retry count on success
        this.retryCount.delete(operation);
        this.resetCircuitBreaker(operation);
        
        console.log(`✅ Successfully retrieved ${result.data?.length || 0} bots`);
        return result.data || [];
        
      } catch (error) {
        console.error(`❌ Attempt ${attempt} failed:`, this.formatError(error));
        
        this.recordFailure(operation);
        
        // Don't retry on certain errors
        if (this.isNonRetryableError(error)) {
          throw this.enhanceError(error);
        }
        
        // Final attempt failed
        if (attempt === maxAttempts) {
          throw this.enhanceError(error);
        }
        
        // Wait before retry with exponential backoff
        const delay = Math.min(1000 * Math.pow(2, attempt - 1), 10000);
        console.log(`⏳ Retrying in ${delay}ms...`);
        await this.sleep(delay);
      }
    }
    
    throw new Error('All retry attempts exhausted');
  }

  /**
   * Campaign creation with validation and error recovery
   */
  async createCampaignSafely(campaignData: any): Promise<any> {
    console.log('📋 Creating campaign with enhanced error handling...');
    
    try {
      // Pre-validation
      this.validateCampaignData(campaignData);
      
      // Check system capacity
      await this.checkSystemCapacity();
      
      // Create campaign
      const campaign = await this.client.campaigns.createCampaign(campaignData);
      
      console.log(`✅ Campaign created successfully: ${campaign.id}`);
      return campaign;
      
    } catch (error) {
      console.error('❌ Campaign creation failed:', this.formatError(error));
      
      // Attempt recovery strategies
      if (error instanceof ValidationError) {
        console.log('🔧 Attempting to fix validation errors...');
        const fixedData = await this.fixValidationErrors(campaignData, error);
        if (fixedData) {
          console.log('🔄 Retrying with fixed data...');
          return this.client.campaigns.createCampaign(fixedData);
        }
      }
      
      throw this.enhanceError(error);
    }
  }

  /**
   * Job monitoring with graceful degradation
   */
  async monitorJobWithGracefulDegradation(jobId: string, timeoutMs: number = 300000): Promise<void> {
    console.log(`👁️ Monitoring job ${jobId} with graceful degradation...`);
    
    const startTime = Date.now();
    let consecutiveFailures = 0;
    let pollingInterval = 5000; // Start with 5 second intervals
    
    while (Date.now() - startTime < timeoutMs) {
      try {
        const job = await this.client.jobs.getJob({ id: jobId });
        
        console.log(`📊 Job ${jobId} status: ${job.status} (${job.progress?.percentage || 0}%)`);
        
        // Reset failure count on success
        consecutiveFailures = 0;
        pollingInterval = 5000;
        
        if (job.status === 'completed' || job.status === 'failed') {
          console.log(`🏁 Job ${jobId} finished with status: ${job.status}`);
          return;
        }
        
      } catch (error) {
        consecutiveFailures++;
        console.error(`❌ Failed to get job status (${consecutiveFailures} consecutive failures):`, 
          this.formatError(error));
        
        // Graceful degradation: increase polling interval
        pollingInterval = Math.min(pollingInterval * 1.5, 60000); // Max 1 minute
        
        // If too many failures, switch to event-based monitoring
        if (consecutiveFailures >= 5) {
          console.log('🔄 Switching to event-based monitoring due to API failures...');
          return this.monitorJobViaEvents(jobId, timeoutMs - (Date.now() - startTime));
        }
      }
      
      await this.sleep(pollingInterval);
    }
    
    throw new Error(`Job monitoring timed out after ${timeoutMs}ms`);
  }

  /**
   * Event-based monitoring as fallback
   */
  private async monitorJobViaEvents(jobId: string, remainingTimeMs: number): Promise<void> {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.client.sse.off(jobId);
        reject(new Error('Event-based monitoring timed out'));
      }, remainingTimeMs);
      
      this.client.sse.on('job.completed', (event) => {
        if (event.data.jobId === jobId) {
          clearTimeout(timeout);
          this.client.sse.off(jobId);
          console.log(`✅ Job ${jobId} completed (via events)`);
          resolve();
        }
      });
      
      this.client.sse.on('job.failed', (event) => {
        if (event.data.jobId === jobId) {
          clearTimeout(timeout);
          this.client.sse.off(jobId);
          console.log(`❌ Job ${jobId} failed (via events)`);
          resolve(); // Still resolve, as we successfully monitored the job
        }
      });
      
      this.client.connectEvents();
    });
  }

  /**
   * Bulk operations with partial failure handling
   */
  async bulkOperationWithPartialFailures<T>(
    items: T[],
    operation: (item: T) => Promise<any>,
    operationName: string
  ): Promise<{ successes: any[], failures: { item: T, error: Error }[] }> {
    console.log(`🔄 Performing bulk ${operationName} on ${items.length} items...`);
    
    const successes: any[] = [];
    const failures: { item: T, error: Error }[] = [];
    
    // Process in batches to avoid overwhelming the system
    const batchSize = 5;
    
    for (let i = 0; i < items.length; i += batchSize) {
      const batch = items.slice(i, i + batchSize);
      console.log(`Processing batch ${Math.floor(i / batchSize) + 1}/${Math.ceil(items.length / batchSize)}`);
      
      const batchPromises = batch.map(async (item) => {
        try {
          const result = await operation(item);
          successes.push(result);
          return { success: true, item, result };
        } catch (error) {
          const enhancedError = this.enhanceError(error);
          failures.push({ item, error: enhancedError });
          return { success: false, item, error: enhancedError };
        }
      });
      
      // Wait for batch to complete
      await Promise.all(batchPromises);
      
      // Brief pause between batches to avoid rate limiting
      if (i + batchSize < items.length) {
        await this.sleep(1000);
      }
    }
    
    console.log(`✅ Bulk ${operationName} completed: ${successes.length} successes, ${failures.length} failures`);
    
    return { successes, failures };
  }

  /**
   * Connection pool management with automatic recovery
   */
  async withConnectionRecovery<T>(operation: () => Promise<T>): Promise<T> {
    const maxRetries = 3;
    let lastError: Error;
    
    for (let attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        return await operation();
      } catch (error) {
        lastError = this.enhanceError(error);
        
        console.error(`❌ Operation failed (attempt ${attempt}/${maxRetries}):`, 
          this.formatError(lastError));
        
        // Check if it's a connection-related error
        if (this.isConnectionError(error)) {
          console.log('🔄 Detected connection error, attempting recovery...');
          
          // Attempt to recover connection
          await this.recoverConnection();
          
          if (attempt < maxRetries) {
            const delay = 2000 * attempt; // Increasing delay
            console.log(`⏳ Waiting ${delay}ms before retry...`);
            await this.sleep(delay);
          }
        } else {
          // Non-connection error, don't retry
          throw lastError;
        }
      }
    }
    
    throw lastError!;
  }

  // Helper methods
  
  private validateCampaignData(data: any): void {
    const errors: string[] = [];
    
    if (!data.name || data.name.trim().length === 0) {
      errors.push('Campaign name is required');
    }
    
    if (!data.jobTemplate) {
      errors.push('Job template is required');
    } else {
      if (!data.jobTemplate.targetBinary) {
        errors.push('Target binary is required in job template');
      }
      
      if (!data.jobTemplate.fuzzerType) {
        errors.push('Fuzzer type is required in job template');
      }
    }
    
    if (errors.length > 0) {
      throw new ValidationError('Campaign data validation failed', errors);
    }
  }

  private async checkSystemCapacity(): Promise<void> {
    try {
      const [bots, analytics] = await Promise.all([
        this.client.bots.listBots(),
        this.client.analytics.getMetrics()
      ]);
      
      const availableBots = bots.data?.filter(bot => bot.status === 'idle').length || 0;
      const systemLoad = analytics.metrics?.system?.cpuUsage || 0;
      
      if (availableBots === 0) {
        console.warn('⚠️ No idle bots available - campaign will be queued');
      }
      
      if (systemLoad > 90) {
        throw new Error('System is under high load - cannot create new campaigns');
      }
      
    } catch (error) {
      // Capacity check is advisory, don't fail the operation
      console.warn('⚠️ Could not check system capacity:', this.formatError(error));
    }
  }

  private async fixValidationErrors(originalData: any, error: ValidationError): Promise<any | null> {
    const fixedData = { ...originalData };
    let wasFixed = false;
    
    if (error.validationErrors) {
      for (const validationError of error.validationErrors) {
        if (validationError.includes('Campaign name is required')) {
          fixedData.name = `Auto-generated Campaign ${Date.now()}`;
          wasFixed = true;
        }
        
        if (validationError.includes('Target binary is required')) {
          fixedData.jobTemplate = fixedData.jobTemplate || {};
          fixedData.jobTemplate.targetBinary = '/tmp/default-target';
          wasFixed = true;
        }
        
        if (validationError.includes('Fuzzer type is required')) {
          fixedData.jobTemplate = fixedData.jobTemplate || {};
          fixedData.jobTemplate.fuzzerType = 'libfuzzer';
          wasFixed = true;
        }
      }
    }
    
    return wasFixed ? fixedData : null;
  }

  private isNonRetryableError(error: any): boolean {
    // Don't retry on client errors (4xx) except for rate limiting and auth
    if (error.status >= 400 && error.status < 500) {
      return error.status !== 429 && error.status !== 401;
    }
    return false;
  }

  private isConnectionError(error: any): boolean {
    return error.code === 'ECONNRESET' || 
           error.code === 'ECONNREFUSED' ||
           error.message?.includes('network') ||
           error.message?.includes('timeout');
  }

  private async recoverConnection(): Promise<void> {
    console.log('🔧 Attempting connection recovery...');
    
    try {
      // Simple health check to test connectivity
      await this.client.health.getHealth();
      console.log('✅ Connection recovered');
    } catch (error) {
      console.error('❌ Connection recovery failed:', this.formatError(error));
      // Could implement more sophisticated recovery here
    }
  }

  private enhanceError(error: any): Error {
    if (error.status) {
      switch (error.status) {
        case 401:
          return new AuthenticationError(error.body?.detail || 'Authentication failed');
        case 429:
          const retryAfter = error.headers?.['retry-after'];
          return new RateLimitError(
            error.body?.detail || 'Rate limit exceeded',
            retryAfter ? parseInt(retryAfter) : undefined
          );
        case 400:
          return new ValidationError(
            error.body?.detail || 'Validation failed',
            error.body?.errors || []
          );
        default:
          if (error.status >= 500) {
            return new NetworkError(error.body?.detail || 'Server error', error.status);
          }
      }
    }
    
    return error instanceof Error ? error : new Error(String(error));
  }

  private formatError(error: any): string {
    if (error instanceof PandaFuzzError) {
      let message = `${error.name}: ${error.message}`;
      if (error.status) {
        message += ` (HTTP ${error.status})`;
      }
      if (error instanceof ValidationError && error.validationErrors) {
        message += `\nValidation errors: ${error.validationErrors.join(', ')}`;
      }
      if (error instanceof RateLimitError && error.retryAfter) {
        message += `\nRetry after: ${error.retryAfter}s`;
      }
      return message;
    }
    
    return error.message || String(error);
  }

  private recordFailure(operation: string): void {
    const state = this.circuitBreakerState.get(operation) || { failures: 0, lastFailure: new Date(), isOpen: false };
    state.failures++;
    state.lastFailure = new Date();
    
    // Open circuit breaker after 5 failures
    if (state.failures >= 5) {
      state.isOpen = true;
      console.warn(`🚨 Circuit breaker opened for operation: ${operation}`);
    }
    
    this.circuitBreakerState.set(operation, state);
  }

  private resetCircuitBreaker(operation: string): void {
    this.circuitBreakerState.delete(operation);
  }

  private isCircuitBreakerOpen(operation: string): boolean {
    const state = this.circuitBreakerState.get(operation);
    if (!state || !state.isOpen) {
      return false;
    }
    
    // Reset circuit breaker after 60 seconds
    const timeSinceLastFailure = Date.now() - state.lastFailure.getTime();
    if (timeSinceLastFailure > 60000) {
      state.isOpen = false;
      state.failures = 0;
      return false;
    }
    
    return true;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

/**
 * Example: Comprehensive error handling patterns
 */
async function errorHandlingExample() {
  console.log('🛡️ Error Handling Patterns Example\n');
  
  const client = new ResilientPandaFuzzClient('http://localhost:8080/api/v1', 'test-key');
  
  try {
    // 1. Health check with error handling
    console.log('1. Testing health check error handling...');
    const isHealthy = await client.healthCheck();
    console.log(`System health: ${isHealthy ? 'Healthy' : 'Unhealthy'}\n`);
    
    // 2. Resilient list operation
    console.log('2. Testing resilient bot listing...');
    try {
      const bots = await client.listBotsWithRetry(3);
      console.log(`Retrieved ${bots.length} bots\n`);
    } catch (error) {
      console.error('Bot listing failed after retries:', error);
    }
    
    // 3. Campaign creation with validation
    console.log('3. Testing campaign creation with validation...');
    try {
      // Test with invalid data first
      await client.createCampaignSafely({
        // Missing required fields
        description: 'Test campaign'
      });
    } catch (error) {
      console.log('Expected validation error:', error.message);
    }
    
    // Test with valid data
    try {
      const campaign = await client.createCampaignSafely({
        name: 'Error Handling Test Campaign',
        description: 'Testing error handling patterns',
        jobTemplate: {
          fuzzerType: 'libfuzzer',
          targetBinary: '/usr/bin/test-target',
          duration: 3600
        }
      });
      console.log('Campaign created successfully:', campaign.id);
    } catch (error) {
      console.error('Campaign creation failed:', error.message);
    }
    
    // 4. Bulk operations with partial failures
    console.log('\n4. Testing bulk operations with partial failures...');
    const testItems = [
      { name: 'valid-bot-1', capabilities: ['libfuzzer'] },
      { name: 'valid-bot-2', capabilities: ['afl++'] },
      { /* invalid item - missing name */ capabilities: ['honggfuzz'] },
      { name: 'valid-bot-3', capabilities: ['libfuzzer', 'afl++'] }
    ];
    
    const results = await client.bulkOperationWithPartialFailures(
      testItems,
      async (item) => {
        // Simulate bot creation
        if (!item.name) {
          throw new ValidationError('Bot name is required');
        }
        return { id: `bot-${Math.random()}`, ...item };
      },
      'bot creation'
    );
    
    console.log(`Bulk operation completed: ${results.successes.length} successes, ${results.failures.length} failures`);
    
    // 5. Connection recovery example
    console.log('\n5. Testing connection recovery...');
    await client.withConnectionRecovery(async () => {
      // This would normally make an API call
      console.log('Simulated API operation with connection recovery');
      return 'success';
    });
    
  } catch (error) {
    console.error('Error handling example failed:', error);
  }
}

/**
 * Example: Rate limiting and backoff strategies
 */
async function rateLimitingExample() {
  console.log('⏱️ Rate Limiting and Backoff Example\n');
  
  const client = new PandaFuzzClient({
    baseUrl: 'http://localhost:8080/api/v1'
  });
  
  const makeRequestWithBackoff = async (attempt: number = 1): Promise<any> => {
    try {
      console.log(`Making request (attempt ${attempt})...`);
      return await client.health.getHealth();
    } catch (error: any) {
      if (error.status === 429) { // Rate limited
        const retryAfter = error.headers?.['retry-after'] || Math.pow(2, attempt);
        console.log(`Rate limited - waiting ${retryAfter}s before retry...`);
        
        await new Promise(resolve => setTimeout(resolve, retryAfter * 1000));
        
        if (attempt < 5) {
          return makeRequestWithBackoff(attempt + 1);
        } else {
          throw new RateLimitError('Rate limit exceeded after maximum retries');
        }
      }
      throw error;
    }
  };
  
  try {
    const result = await makeRequestWithBackoff();
    console.log('Request succeeded:', result.status);
  } catch (error) {
    console.error('Request failed:', error);
  }
}

// CLI interface
const command = process.argv[2];

switch (command) {
  case 'error-handling':
    errorHandlingExample();
    break;
  case 'rate-limiting':
    rateLimitingExample();
    break;
  default:
    console.log('Available commands:');
    console.log('  error-handling - Comprehensive error handling patterns');
    console.log('  rate-limiting - Rate limiting and backoff strategies');
    console.log('\nUsage: node error-handling.js <command>');
}

export { 
  ResilientPandaFuzzClient,
  PandaFuzzError,
  NetworkError,
  AuthenticationError,
  RateLimitError,
  ValidationError,
  errorHandlingExample,
  rateLimitingExample
};