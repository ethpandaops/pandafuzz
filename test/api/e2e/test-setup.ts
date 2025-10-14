/**
 * Test setup configuration for Jest
 */

import { testHelpers } from './utils/test-helpers';

// Extend Jest timeout for integration tests
jest.setTimeout(120000); // 2 minutes

// Global test setup
beforeAll(async () => {
  console.log('🚀 Starting PandaFuzz E2E Test Suite');
  
  // Wait for API to be ready
  try {
    await testHelpers.waitForAPI(60); // Wait up to 2 minutes
    console.log('✅ API is ready');
  } catch (error) {
    console.error('❌ Failed to connect to API:', error);
    throw error;
  }

  // Wait for system readiness
  try {
    await testHelpers.waitForReadiness(60000); // Wait up to 1 minute
    console.log('✅ System is ready');
  } catch (error) {
    console.error('⚠️ System readiness check failed, continuing anyway:', error);
    // Don't fail here as some services might still be starting
  }

  // Wait for at least one bot to be online
  try {
    await testHelpers.waitForBots(1, 60000); // Wait up to 1 minute
    console.log('✅ At least one bot is online');
  } catch (error) {
    console.error('⚠️ No bots detected, some tests may fail:', error);
    // Don't fail here as tests should handle missing bots gracefully
  }
});

// Global test teardown
afterAll(async () => {
  console.log('🧹 Cleaning up after E2E tests');
  
  try {
    // Close any open SSE connections
    testHelpers.getClient().close();
    console.log('✅ Closed client connections');
  } catch (error) {
    console.error('⚠️ Error closing connections:', error);
  }
});

// Error handling for unhandled rejections
process.on('unhandledRejection', (reason, promise) => {
  console.error('Unhandled Rejection at:', promise, 'reason:', reason);
  // Don't exit the process in tests
});

process.on('uncaughtException', (error) => {
  console.error('Uncaught Exception:', error);
  // Don't exit the process in tests
});

// Custom Jest matchers
declare global {
  namespace jest {
    interface Matchers<R> {
      toBeWithinRange(min: number, max: number): R;
      toMatchApiResponse(): R;
      toHaveValidTimestamp(): R;
      toBeValidJobStatus(): R;
      toBeCampaignStatus(): R;
    }
  }
}

// Add custom matchers
expect.extend({
  toBeWithinRange(received: number, min: number, max: number) {
    const pass = received >= min && received <= max;
    return {
      message: () =>
        `expected ${received} ${pass ? 'not ' : ''}to be within range ${min}-${max}`,
      pass,
    };
  },

  toMatchApiResponse(received: any) {
    const hasTimestamp = received.timestamp || received.created_at || received.updated_at;
    const hasId = received.id || received.ID;
    
    return {
      message: () =>
        `expected ${JSON.stringify(received)} to be a valid API response with id and timestamp`,
      pass: Boolean(hasTimestamp && hasId),
    };
  },

  toHaveValidTimestamp(received: any) {
    const timestamp = received.timestamp || received.created_at || received.updated_at;
    const isValid = timestamp && !isNaN(new Date(timestamp).getTime());
    
    return {
      message: () =>
        `expected ${JSON.stringify(received)} to have a valid timestamp`,
      pass: Boolean(isValid),
    };
  },

  toBeValidJobStatus(received: string) {
    const validStatuses = ['pending', 'queued', 'running', 'completed', 'failed', 'cancelled'];
    const isValid = validStatuses.includes(received);
    
    return {
      message: () =>
        `expected ${received} to be a valid job status (${validStatuses.join(', ')})`,
      pass: isValid,
    };
  },

  toBeCampaignStatus(received: string) {
    const validStatuses = ['created', 'starting', 'running', 'stopping', 'stopped', 'completed', 'failed'];
    const isValid = validStatuses.includes(received);
    
    return {
      message: () =>
        `expected ${received} to be a valid campaign status (${validStatuses.join(', ')})`,
      pass: isValid,
    };
  },
});

// Console logging configuration
const originalLog = console.log;
const originalError = console.error;
const originalWarn = console.warn;

// Add timestamps to console output
console.log = (...args) => {
  originalLog(`[${new Date().toISOString()}]`, ...args);
};

console.error = (...args) => {
  originalError(`[${new Date().toISOString()}] ERROR:`, ...args);
};

console.warn = (...args) => {
  originalWarn(`[${new Date().toISOString()}] WARN:`, ...args);
};

// Test environment information
console.log('📋 Test Environment Information:');
console.log(`   Node.js: ${process.version}`);
console.log(`   Platform: ${process.platform} ${process.arch}`);
console.log(`   Memory: ${Math.round(process.memoryUsage().heapTotal / 1024 / 1024)}MB`);
console.log(`   PandaFuzz Base URL: ${process.env.PANDAFUZZ_BASE_URL || 'http://localhost:8080/api/v1'}`);
console.log(`   Test Timeout: ${120000}ms`);

export {};