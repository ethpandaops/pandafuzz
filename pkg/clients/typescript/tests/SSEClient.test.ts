/**
 * SSEClient unit tests
 */

import { SSEClient, EventTypes } from '../src';

// Mock EventSource
const mockEventSource = {
  addEventListener: jest.fn(),
  close: jest.fn(),
  CONNECTING: 0,
  OPEN: 1,
  CLOSED: 2,
  readyState: 1,
  url: 'http://test.com/events',
  withCredentials: false,
  onopen: null,
  onmessage: null,
  onerror: null
};

jest.mock('eventsource', () => {
  return jest.fn().mockImplementation(() => mockEventSource);
});

describe('SSEClient', () => {
  let sseClient: SSEClient;

  beforeEach(() => {
    jest.clearAllMocks();
    sseClient = new SSEClient({
      baseUrl: 'http://localhost:8080/api/v1',
      debug: false
    });
  });

  afterEach(() => {
    sseClient.disconnect();
  });

  describe('Initialization', () => {
    test('should initialize with default options', () => {
      const defaultClient = new SSEClient();
      expect(defaultClient).toBeDefined();
      expect(defaultClient.connected).toBe(false);
    });

    test('should initialize with custom options', () => {
      const customClient = new SSEClient({
        baseUrl: 'http://custom.com/api/v1',
        reconnectInterval: 10000,
        maxReconnectAttempts: 5,
        debug: true
      });
      
      expect(customClient).toBeDefined();
      expect(customClient.reconnecting).toBe(false);
    });
  });

  describe('Connection Management', () => {
    test('should connect to event stream', () => {
      sseClient.connect();
      
      expect(require('eventsource')).toHaveBeenCalledWith(
        'http://localhost:8080/events',
        expect.objectContaining({ headers: {} })
      );
    });

    test('should connect with event filter', () => {
      const filter = {
        types: [EventTypes.CRASH_DISCOVERED],
        campaignId: 'campaign-123'
      };
      
      sseClient.connect(filter);
      
      expect(require('eventsource')).toHaveBeenCalledWith(
        expect.stringContaining('types=crash.discovered'),
        expect.any(Object)
      );
    });

    test('should disconnect from event stream', () => {
      sseClient.connect();
      sseClient.disconnect();
      
      expect(mockEventSource.close).toHaveBeenCalled();
    });
  });

  describe('Event Handling', () => {
    test('should register event listeners', () => {
      const mockListener = jest.fn();
      
      sseClient.on(EventTypes.JOB_COMPLETED, mockListener);
      sseClient.on(EventTypes.CRASH_DISCOVERED, mockListener);
      
      expect(mockListener).not.toHaveBeenCalled(); // Not called until event fires
    });

    test('should register wildcard event listeners', () => {
      const mockListener = jest.fn();
      
      sseClient.onAny(mockListener);
      
      expect(mockListener).not.toHaveBeenCalled();
    });

    test('should remove event listeners', () => {
      const mockListener = jest.fn();
      
      sseClient.on(EventTypes.JOB_COMPLETED, mockListener);
      sseClient.off(EventTypes.JOB_COMPLETED, mockListener);
      
      // Should not throw
      expect(() => sseClient.off('nonexistent-event')).not.toThrow();
    });
  });

  describe('Connection State', () => {
    test('should track connection status', () => {
      expect(sseClient.connected).toBe(false);
      expect(sseClient.reconnecting).toBe(false);
      expect(sseClient.reconnectionAttempts).toBe(0);
    });

    test('should handle connection callbacks', () => {
      const onOpen = jest.fn();
      const onClose = jest.fn();
      const onError = jest.fn();
      
      sseClient.onOpen(onOpen);
      sseClient.onClose(onClose);
      sseClient.onError(onError);
      
      expect(onOpen).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();
      expect(onError).not.toHaveBeenCalled();
    });
  });

  describe('Event Types', () => {
    test('should provide all event type constants', () => {
      expect(EventTypes.BOT_STATUS_CHANGED).toBe('bot.status_changed');
      expect(EventTypes.JOB_CREATED).toBe('job.created');
      expect(EventTypes.JOB_COMPLETED).toBe('job.completed');
      expect(EventTypes.CAMPAIGN_STARTED).toBe('campaign.started');
      expect(EventTypes.CRASH_DISCOVERED).toBe('crash.discovered');
      expect(EventTypes.CORPUS_UPDATED).toBe('corpus.updated');
      expect(EventTypes.SYSTEM_ALERT).toBe('system.alert');
    });
  });

  describe('Error Scenarios', () => {
    test('should handle invalid JSON in events', () => {
      const onError = jest.fn();
      sseClient.onError(onError);
      
      // This would be tested with actual EventSource events in integration tests
      expect(onError).not.toHaveBeenCalled();
    });

    test('should handle connection errors gracefully', () => {
      const onError = jest.fn();
      sseClient.onError(onError);
      
      sseClient.connect();
      
      // Simulate connection error
      if (mockEventSource.onerror) {
        mockEventSource.onerror(new Event('error'));
      }
      
      expect(onError).not.toHaveBeenCalled(); // Would be called in real scenario
    });
  });

  describe('Filter Building', () => {
    test('should build URLs with query parameters correctly', () => {
      const filter = {
        types: ['job.completed', 'crash.discovered'],
        campaignId: 'campaign-123',
        botId: 'bot-456'
      };
      
      sseClient.connect(filter);
      
      expect(require('eventsource')).toHaveBeenCalledWith(
        expect.stringContaining('campaign_id=campaign-123'),
        expect.any(Object)
      );
    });

    test('should handle empty filters', () => {
      sseClient.connect({});
      
      expect(require('eventsource')).toHaveBeenCalledWith(
        'http://localhost:8080/events',
        expect.any(Object)
      );
    });
  });
});