/**
 * PandaFuzzClient unit tests
 */

import { PandaFuzzClient, Configuration } from '../src';

describe('PandaFuzzClient', () => {
  let client: PandaFuzzClient;

  beforeEach(() => {
    client = new PandaFuzzClient({
      baseUrl: 'http://localhost:8080/api/v1',
      apiKey: 'test-api-key'
    });
  });

  afterEach(() => {
    if (client) {
      client.close();
    }
  });

  describe('Initialization', () => {
    test('should initialize with default configuration', () => {
      const defaultClient = new PandaFuzzClient();
      expect(defaultClient).toBeDefined();
      expect(defaultClient.getConfiguration()).toBeDefined();
    });

    test('should initialize with custom configuration', () => {
      expect(client).toBeDefined();
      expect(client.getConfiguration()).toBeDefined();
    });

    test('should provide access to all API clients', () => {
      expect(client.analytics).toBeDefined();
      expect(client.batch).toBeDefined();
      expect(client.bots).toBeDefined();
      expect(client.campaigns).toBeDefined();
      expect(client.corpus).toBeDefined();
      expect(client.crashes).toBeDefined();
      expect(client.events).toBeDefined();
      expect(client.health).toBeDefined();
      expect(client.jobs).toBeDefined();
      expect(client.sse).toBeDefined();
    });
  });

  describe('Configuration Management', () => {
    test('should allow configuration updates', () => {
      const newToken = 'new-jwt-token';
      client.updateConfiguration({ accessToken: newToken });
      
      const config = client.getConfiguration();
      expect(config).toBeDefined();
    });

    test('should handle configuration with both API key and token', () => {
      const clientWithBoth = new PandaFuzzClient({
        baseUrl: 'http://localhost:8080/api/v1',
        apiKey: 'test-api-key',
        accessToken: 'jwt-token'
      });
      
      expect(clientWithBoth).toBeDefined();
      expect(clientWithBoth.getConfiguration()).toBeDefined();
    });
  });

  describe('Event Stream Management', () => {
    test('should connect and disconnect event stream', () => {
      expect(client.eventsConnected).toBe(false);
      
      // Note: In real tests, we'd mock the SSE connection
      // For now, just test the interface
      expect(() => client.connectEvents()).not.toThrow();
      expect(() => client.disconnectEvents()).not.toThrow();
    });

    test('should handle event filters', () => {
      const filter = {
        types: ['job.completed', 'crash.discovered'],
        campaignId: 'test-campaign-123'
      };
      
      expect(() => client.connectEvents(filter)).not.toThrow();
    });
  });

  describe('Convenience Methods', () => {
    test('should provide event subscription helpers', () => {
      const mockCallback = jest.fn();
      
      expect(() => client.onBotStatusChange(mockCallback)).not.toThrow();
      expect(() => client.onJobCompleted(mockCallback)).not.toThrow();
      expect(() => client.onCrashDiscovered(mockCallback)).not.toThrow();
      expect(() => client.onCampaignStatusChange(mockCallback)).not.toThrow();
      expect(() => client.onAnyEvent(mockCallback)).not.toThrow();
    });

    test('should handle retry operations', async () => {
      const mockOperation = jest.fn().mockResolvedValue('success');
      
      const result = await client.withRetry(mockOperation);
      expect(result).toBe('success');
      expect(mockOperation).toHaveBeenCalledTimes(1);
    });

    test('should handle failed retry operations', async () => {
      // Create an error that should be retryable (server error)
      const retryableError = { status: 500, message: 'Server error' };
      const mockOperation = jest.fn().mockRejectedValue(retryableError);
      
      await expect(client.withRetry(mockOperation, { maxAttempts: 2 }))
        .rejects.toMatchObject({ status: 500 });
      
      expect(mockOperation).toHaveBeenCalledTimes(2);
    });
  });

  describe('Error Handling', () => {
    test('should handle network errors gracefully', async () => {
      // Mock a network error
      const errorClient = new PandaFuzzClient({
        baseUrl: 'http://invalid-url:1234/api/v1'
      });

      await expect(errorClient.healthCheck()).resolves.toBe(false);
    });

    test('should identify retryable errors correctly', () => {
      // This tests the private isRetryableError method indirectly
      const retriableOperation = jest.fn()
        .mockRejectedValueOnce({ status: 500 })
        .mockResolvedValueOnce('success');

      expect(client.withRetry(retriableOperation)).resolves.toBe('success');
    });
  });

  describe('Type Safety', () => {
    test('should provide properly typed API responses', () => {
      // These tests ensure TypeScript compilation works correctly
      expect(typeof client.bots.listBots).toBe('function');
      expect(typeof client.campaigns.createCampaign).toBe('function');
      expect(typeof client.jobs.getJob).toBe('function');
    });

    test('should export all necessary types', () => {
      expect(Configuration).toBeDefined();
      expect(PandaFuzzClient).toBeDefined();
    });
  });
});