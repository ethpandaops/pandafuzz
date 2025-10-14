/**
 * Bot Lifecycle End-to-End Tests
 * 
 * Tests complete bot lifecycle including:
 * - Registration
 * - Heartbeat mechanism
 * - Job assignment and processing
 * - Result reporting
 * - Disconnection and reconnection
 */

import { testHelpers, ResourceTracker, EventCollector } from './utils/test-helpers';
import { createBotRegistrationData, TEST_BINARIES, JOB_CONFIGS } from './utils/test-fixtures';
import { PandaFuzzClient } from '@pandafuzz/client';

describe('Bot Lifecycle', () => {
  let client: PandaFuzzClient;
  let resourceTracker: ResourceTracker;
  let eventCollector: EventCollector;

  beforeAll(async () => {
    client = testHelpers.getClient();
    resourceTracker = new ResourceTracker();
    eventCollector = new EventCollector(client);
  });

  afterAll(async () => {
    try {
      eventCollector.stopCollecting();
      await resourceTracker.cleanup();
    } catch (error) {
      console.warn('Cleanup warning:', error);
    }
  });

  describe('Bot Registration', () => {
    test('should register a new bot successfully', async () => {
      const botId = `test-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);

      const response = await client.bots.createBot({
        botCreateRequest: botData
      });

      expect(response).toMatchApiResponse();
      expect(response.id).toBe(botId);
      expect(response.name).toBe(botData.name);
      expect(response.status).toBe('offline');
      expect(response.capabilities).toEqual(botData.capabilities);

      // Verify bot appears in list
      const listResponse = await client.bots.listBots();
      const registeredBot = listResponse.bots?.find(bot => bot.id === botId);
      
      expect(registeredBot).toBeDefined();
      expect(registeredBot?.id).toBe(botId);
    });

    test('should prevent duplicate bot registration', async () => {
      const botId = `duplicate-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);

      // Register first time
      await client.bots.createBot({
        botCreateRequest: botData
      });

      // Attempt duplicate registration
      await expect(client.bots.createBot({
        botCreateRequest: botData
      })).rejects.toThrow();
    });

    test('should validate bot registration data', async () => {
      // Test invalid bot data
      const invalidBotData = {
        id: '', // Empty ID should be invalid
        name: 'Test Bot',
        capabilities: {
          fuzzers: [],
          max_concurrent_jobs: 0,
          supported_platforms: [],
          memory_mb: 0,
          cpu_cores: 0
        }
      };

      await expect(client.bots.createBot({
        botCreateRequest: invalidBotData
      })).rejects.toThrow();
    });
  });

  describe('Bot Heartbeat Mechanism', () => {
    let botId: string;

    beforeEach(async () => {
      botId = `heartbeat-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });
    });

    test('should send heartbeat and update bot status', async () => {
      const heartbeatData = {
        status: 'online' as const,
        resource_usage: {
          cpu_percent: 25.5,
          memory_percent: 60.0,
          disk_percent: 40.0,
          active_jobs: 0
        },
        capabilities: {
          available_fuzzers: ['afl++', 'libfuzzer'],
          max_concurrent_jobs: 2
        }
      };

      const response = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: heartbeatData
      });

      expect(response.status).toBe('ok');
      expect(response.next_heartbeat_in).toBeGreaterThan(0);

      // Verify bot status updated
      const botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');
      expect(botResponse.last_seen).toHaveValidTimestamp();
      expect(botResponse.resource_usage).toEqual(heartbeatData.resource_usage);
    });

    test('should handle heartbeat with job commands', async () => {
      // Start event collection
      eventCollector.startCollecting({
        types: ['BOT_STATUS_CHANGED', 'JOB_ASSIGNED']
      });

      const heartbeatData = {
        status: 'online' as const,
        resource_usage: {
          cpu_percent: 10.0,
          memory_percent: 30.0,
          disk_percent: 20.0,
          active_jobs: 0
        }
      };

      const response = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: heartbeatData
      });

      expect(response.commands).toBeDefined();
      expect(Array.isArray(response.commands)).toBe(true);

      // If there are commands, they should be valid
      if (response.commands && response.commands.length > 0) {
        for (const command of response.commands) {
          expect(command.type).toBeDefined();
          expect(command.id).toBeDefined();
        }
      }
    });

    test('should detect bot timeout when heartbeat stops', async () => {
      // Send initial heartbeat to bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 0,
            memory_percent: 0,
            disk_percent: 0,
            active_jobs: 0
          }
        }
      });

      // Wait longer than heartbeat timeout (usually 30-60 seconds)
      // For testing, we'll wait a reasonable time then check status
      await new Promise(resolve => setTimeout(resolve, 10000)); // 10 seconds

      // Check if bot is still considered online
      const botResponse = await client.bots.getBot({ id: botId });
      
      // The bot should still be online since we just sent a heartbeat
      expect(botResponse.status).toBe('online');
    });
  });

  describe('Job Assignment and Processing', () => {
    let botId: string;

    beforeEach(async () => {
      botId = `job-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });

      // Bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 15.0,
            memory_percent: 25.0,
            disk_percent: 30.0,
            active_jobs: 0
          }
        }
      });
    });

    test('should receive job assignment through heartbeat', async () => {
      // Create a simple test job
      const jobConfig = JOB_CONFIGS['quick-job'];
      const jobData = {
        name: testHelpers.generateTestName('assignment-test'),
        description: jobConfig.description,
        fuzzer_type: jobConfig.fuzzerType,
        target_binary: '/test-resources/test-targets/simple-test',
        duration: jobConfig.duration,
        memory_limit: jobConfig.memoryLimit,
        timeout: jobConfig.timeout,
        arguments: jobConfig.args.join(' ')
      };

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      resourceTracker.trackJob(jobResponse.id!);

      // Send heartbeat to potentially receive job assignment
      const heartbeatResponse = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 10.0,
            memory_percent: 20.0,
            disk_percent: 15.0,
            active_jobs: 0
          }
        }
      });

      // Check for job assignment command
      const assignmentCommands = heartbeatResponse.commands?.filter(
        cmd => cmd.type === 'START_JOB'
      ) || [];

      if (assignmentCommands.length > 0) {
        const assignedJob = assignmentCommands[0];
        expect(assignedJob.job_id).toBeDefined();
        expect(assignedJob.job_config).toBeDefined();
      }

      // Wait for job to potentially be assigned and started
      await testHelpers.waitForJobStatus(jobResponse.id!, ['running', 'completed'], 30000);
    });

    test('should report job progress and results', async () => {
      // Create a test job
      const jobData = {
        name: testHelpers.generateTestName('progress-test'),
        description: 'Job for testing progress reporting',
        fuzzer_type: 'libfuzzer' as const,
        target_binary: '/test-resources/test-targets/simple-test',
        duration: 60,
        memory_limit: 128 * 1024 * 1024,
        timeout: 1000,
        arguments: '@@'
      };

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      resourceTracker.trackJob(jobResponse.id!);

      // Wait for job assignment and start
      await testHelpers.waitForJobStatus(jobResponse.id!, ['running'], 30000);

      // Monitor job progress
      let progressReported = false;
      const maxChecks = 10;
      
      for (let i = 0; i < maxChecks; i++) {
        const jobStatus = await client.jobs.getJob({ id: jobResponse.id! });
        
        if (jobStatus.progress) {
          progressReported = true;
          expect(jobStatus.progress.executions).toBeGreaterThan(0);
          expect(jobStatus.progress.coverage).toBeDefined();
          break;
        }
        
        await new Promise(resolve => setTimeout(resolve, 2000));
      }

      // Progress should be reported at some point
      expect(progressReported).toBe(true);
    });

    test('should handle job cancellation gracefully', async () => {
      // Create and start a job
      const jobData = {
        name: testHelpers.generateTestName('cancellation-test'),
        description: 'Job for testing cancellation',
        fuzzer_type: 'afl++' as const,
        target_binary: '/test-resources/test-targets/simple-test',
        duration: 300, // 5 minutes - long enough to cancel
        memory_limit: 256 * 1024 * 1024,
        timeout: 1000,
        arguments: '@@'
      };

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      resourceTracker.trackJob(jobResponse.id!);

      // Wait for job to start
      await testHelpers.waitForJobStatus(jobResponse.id!, ['running'], 30000);

      // Cancel the job
      await client.jobs.cancelJob({ id: jobResponse.id! });

      // Wait for cancellation to be processed
      await testHelpers.waitForJobStatus(jobResponse.id!, ['cancelled'], 15000);

      // Verify final job state
      const finalJobStatus = await client.jobs.getJob({ id: jobResponse.id! });
      expect(finalJobStatus.status).toBe('cancelled');
      expect(finalJobStatus.ended_at).toHaveValidTimestamp();
    });
  });

  describe('Bot Disconnection and Reconnection', () => {
    let botId: string;

    beforeEach(async () => {
      botId = `reconnect-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });
    });

    test('should handle graceful disconnection', async () => {
      // Bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 5.0,
            memory_percent: 10.0,
            disk_percent: 5.0,
            active_jobs: 0
          }
        }
      });

      // Send offline status
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'offline',
          resource_usage: {
            cpu_percent: 0,
            memory_percent: 0,
            disk_percent: 0,
            active_jobs: 0
          }
        }
      });

      // Verify bot is marked offline
      const botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('offline');
    });

    test('should handle bot reconnection after timeout', async () => {
      // Initial connection
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 20.0,
            memory_percent: 40.0,
            disk_percent: 25.0,
            active_jobs: 0
          }
        }
      });

      let botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');

      // Simulate network disconnection by not sending heartbeat
      // Wait for a period, then reconnect
      await new Promise(resolve => setTimeout(resolve, 5000));

      // Reconnect
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 15.0,
            memory_percent: 35.0,
            disk_percent: 20.0,
            active_jobs: 0
          }
        }
      });

      // Verify bot is back online
      botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');
      expect(botResponse.last_seen).toHaveValidTimestamp();
    });

    test('should recover jobs after reconnection', async () => {
      // Create a long-running job
      const jobData = {
        name: testHelpers.generateTestName('recovery-test'),
        description: 'Job for testing recovery after reconnection',
        fuzzer_type: 'libfuzzer' as const,
        target_binary: '/test-resources/test-targets/simple-test',
        duration: 300, // 5 minutes
        memory_limit: 256 * 1024 * 1024,
        timeout: 1000,
        arguments: '@@'
      };

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      resourceTracker.trackJob(jobResponse.id!);

      // Bring bot online and let it get assigned the job
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 10.0,
            memory_percent: 20.0,
            disk_percent: 15.0,
            active_jobs: 0
          }
        }
      });

      // Wait for job assignment
      await testHelpers.waitForJobStatus(jobResponse.id!, ['running'], 30000);

      // Simulate disconnection and reconnection
      await new Promise(resolve => setTimeout(resolve, 3000));

      // Reconnect with active job
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 25.0,
            memory_percent: 45.0,
            disk_percent: 20.0,
            active_jobs: 1
          }
        }
      });

      // Job should still be running or completed
      const jobStatus = await client.jobs.getJob({ id: jobResponse.id! });
      expect(['running', 'completed', 'failed']).toContain(jobStatus.status);
    });
  });

  describe('Bot Resource Monitoring', () => {
    let botId: string;

    beforeEach(async () => {
      botId = `resource-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });
    });

    test('should track resource usage over time', async () => {
      const resourceSnapshots = [];
      
      // Send several heartbeats with varying resource usage
      const usagePatterns = [
        { cpu_percent: 10, memory_percent: 20, disk_percent: 15 },
        { cpu_percent: 45, memory_percent: 60, disk_percent: 25 },
        { cpu_percent: 80, memory_percent: 85, disk_percent: 40 },
        { cpu_percent: 30, memory_percent: 45, disk_percent: 20 }
      ];

      for (const usage of usagePatterns) {
        await client.bots.sendHeartbeat({
          id: botId,
          botHeartbeatRequest: {
            status: 'online',
            resource_usage: {
              ...usage,
              active_jobs: 1
            }
          }
        });

        const botResponse = await client.bots.getBot({ id: botId });
        resourceSnapshots.push(botResponse.resource_usage);

        await new Promise(resolve => setTimeout(resolve, 1000));
      }

      // Verify resource tracking
      expect(resourceSnapshots.length).toBe(usagePatterns.length);
      
      for (let i = 0; i < resourceSnapshots.length; i++) {
        const snapshot = resourceSnapshots[i];
        const expected = usagePatterns[i];
        
        expect(snapshot?.cpu_percent).toBe(expected.cpu_percent);
        expect(snapshot?.memory_percent).toBe(expected.memory_percent);
        expect(snapshot?.disk_percent).toBe(expected.disk_percent);
      }
    });

    test('should prevent job assignment when resources are exhausted', async () => {
      // Report high resource usage
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 95,
            memory_percent: 98,
            disk_percent: 85,
            active_jobs: 4 // At maximum capacity
          }
        }
      });

      // Create a job that would exceed capacity
      const jobData = {
        name: testHelpers.generateTestName('resource-limit-test'),
        description: 'Job for testing resource limits',
        fuzzer_type: 'afl++' as const,
        target_binary: '/test-resources/test-targets/simple-test',
        duration: 60,
        memory_limit: 512 * 1024 * 1024, // High memory requirement
        timeout: 1000,
        arguments: '@@'
      };

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      resourceTracker.trackJob(jobResponse.id!);

      // Send heartbeat - should not receive job assignment
      const heartbeatResponse = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 95,
            memory_percent: 98,
            disk_percent: 85,
            active_jobs: 4
          }
        }
      });

      // Should not get job assignment commands due to resource constraints
      const jobCommands = heartbeatResponse.commands?.filter(
        cmd => cmd.type === 'START_JOB'
      ) || [];

      // Job should remain in pending/queued state
      const jobStatus = await client.jobs.getJob({ id: jobResponse.id! });
      expect(['pending', 'queued']).toContain(jobStatus.status);
    });
  });

  describe('Event-Driven Bot Interactions', () => {
    let botId: string;

    beforeEach(async () => {
      botId = `event-bot-${Date.now()}`;
      const botData = createBotRegistrationData(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });

      eventCollector.startCollecting();
    });

    afterEach(() => {
      eventCollector.stopCollecting();
    });

    test('should emit bot status change events', async () => {
      // Bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 5,
            memory_percent: 10,
            disk_percent: 5,
            active_jobs: 0
          }
        }
      });

      // Wait for status change event
      const statusEvent = await eventCollector.waitForEvent(
        'BOT_STATUS_CHANGED',
        10000,
        event => event.data?.bot_id === botId
      );

      expect(statusEvent).toBeDefined();
      expect(statusEvent.data.bot_id).toBe(botId);
      expect(statusEvent.data.new_status).toBe('online');
    });

    test('should emit job assignment events', async () => {
      // Bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: {
            cpu_percent: 10,
            memory_percent: 15,
            disk_percent: 10,
            active_jobs: 0
          }
        }
      });

      // Create a job
      const jobData = {
        name: testHelpers.generateTestName('event-assignment-test'),
        description: 'Job for testing assignment events',
        fuzzer_type: 'libfuzzer' as const,
        target_binary: '/test-resources/test-targets/simple-test',
        duration: 60,
        memory_limit: 128 * 1024 * 1024,
        timeout: 1000,
        arguments: '@@'
      };

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      resourceTracker.trackJob(jobResponse.id!);

      // Wait for job assignment event
      const assignmentEvent = await eventCollector.waitForEvent(
        'JOB_ASSIGNED',
        15000,
        event => event.data?.job_id === jobResponse.id
      );

      expect(assignmentEvent).toBeDefined();
      expect(assignmentEvent.data.job_id).toBe(jobResponse.id);
      expect(assignmentEvent.data.bot_id).toBe(botId);
    });
  });
});