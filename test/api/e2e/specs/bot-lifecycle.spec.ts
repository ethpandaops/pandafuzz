/**
 * Bot Lifecycle End-to-End Tests
 * 
 * Tests complete bot lifecycle including:
 * - Bot registration
 * - Heartbeat mechanism  
 * - Job assignment and processing
 * - Result reporting
 * - Disconnection and reconnection scenarios
 * - Resource monitoring
 */

import { test, expect } from '@playwright/test';
import { PandaFuzzClient } from '@pandafuzz/client';
import { TestSetup, EventCollector, DataGenerators, ApiHelpers, addCustomMatchers } from '../test-utils';

// Add custom matchers
addCustomMatchers();

test.describe('Bot Lifecycle', () => {
  let client: PandaFuzzClient;
  let testSetup: TestSetup;
  let eventCollector: EventCollector;

  test.beforeAll(async () => {
    // Initialize client and utilities
    const baseUrl = process.env.PANDAFUZZ_API_URL || 'http://localhost:8080/api/v1';
    client = new PandaFuzzClient({ baseUrl });
    ApiHelpers.init(baseUrl);
    
    testSetup = new TestSetup(client);
    eventCollector = new EventCollector(client);

    // Verify API is accessible
    await client.waitForReady();
  });

  test.afterAll(async () => {
    // Clean up all test resources
    eventCollector.stopCollecting();
    await testSetup.cleanup();
    client.close();
  });

  test.describe('Bot Registration', () => {
    test('should register a new bot successfully', async () => {
      const botData = DataGenerators.createBotRegistration();
      testSetup.trackBot(botData.id);

      const response = await client.bots.createBot({
        botCreateRequest: botData
      });

      // Verify response structure and data
      expect(response).toMatchApiResponse();
      expect(response.id).toBe(botData.id);
      expect(response.name).toBe(botData.name);
      expect(response.status).toBe('offline');
      expect(response.capabilities?.fuzzers).toEqual(botData.capabilities.fuzzers);
      expect(response.capabilities?.max_concurrent_jobs).toBe(botData.capabilities.max_concurrent_jobs);

      // Verify bot appears in the list
      const listResponse = await client.bots.listBots();
      const registeredBot = listResponse.bots?.find(bot => bot.id === botData.id);
      
      expect(registeredBot).toBeDefined();
      expect(registeredBot?.id).toBe(botData.id);
      expect(registeredBot?.name).toBe(botData.name);
    });

    test('should prevent duplicate bot registration', async () => {
      const botData = DataGenerators.createBotRegistration();
      testSetup.trackBot(botData.id);

      // Register the bot first time
      await client.bots.createBot({
        botCreateRequest: botData
      });

      // Attempt duplicate registration should fail
      await expect(client.bots.createBot({
        botCreateRequest: botData
      })).rejects.toThrow();
    });

    test('should validate bot registration data', async () => {
      // Test with invalid bot data (empty ID)
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

    test('should handle bot registration with optional fields', async () => {
      const botData = DataGenerators.createBotRegistration();
      // Add optional fields
      botData.tags = { 
        environment: 'test', 
        region: 'us-east-1',
        purpose: 'e2e-testing'
      };
      
      testSetup.trackBot(botData.id);

      const response = await client.bots.createBot({
        botCreateRequest: botData
      });

      expect(response.tags).toEqual(botData.tags);
    });
  });

  test.describe('Bot Heartbeat Mechanism', () => {
    let botId: string;

    test.beforeEach(async () => {
      const botData = DataGenerators.createBotRegistration();
      botId = botData.id;
      testSetup.trackBot(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });
    });

    test('should send heartbeat and update bot status', async () => {
      const resourceUsage = DataGenerators.createResourceUsage({
        cpu_percent: 25.5,
        memory_percent: 60.0,
        disk_percent: 40.0,
        active_jobs: 0
      });

      const heartbeatData = {
        status: 'online' as const,
        resource_usage: resourceUsage,
        capabilities: {
          available_fuzzers: ['afl++', 'libfuzzer'],
          max_concurrent_jobs: 2
        }
      };

      const response = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: heartbeatData
      });

      // Verify heartbeat response
      expect(response.status).toBe('ok');
      expect(response.next_heartbeat_in).toBeGreaterThan(0);
      expect(response.next_heartbeat_in).toBeLessThanOrEqual(60000); // Should be reasonable

      // Verify bot status was updated
      const botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');
      expect(botResponse.last_seen).toHaveValidTimestamp();
      expect(botResponse.resource_usage).toBeValidResourceUsage();
      expect(botResponse.resource_usage?.cpu_percent).toBe(resourceUsage.cpu_percent);
      expect(botResponse.resource_usage?.memory_percent).toBe(resourceUsage.memory_percent);
    });

    test('should handle heartbeat with commands', async () => {
      const resourceUsage = DataGenerators.createResourceUsage({
        cpu_percent: 10.0,
        memory_percent: 30.0,
        disk_percent: 20.0,
        active_jobs: 0
      });

      const heartbeatData = {
        status: 'online' as const,
        resource_usage: resourceUsage
      };

      const response = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: heartbeatData
      });

      // Commands should be an array (even if empty)
      expect(response.commands).toBeDefined();
      expect(Array.isArray(response.commands)).toBe(true);

      // If there are commands, they should have the required structure
      if (response.commands && response.commands.length > 0) {
        for (const command of response.commands) {
          expect(command.type).toBeDefined();
          expect(command.id).toBeDefined();
          expect(['START_JOB', 'STOP_JOB', 'UPDATE_CONFIG'].includes(command.type)).toBe(true);
        }
      }
    });

    test('should track resource usage over time', async () => {
      const resourcePatterns = [
        { cpu_percent: 10, memory_percent: 20, disk_percent: 15 },
        { cpu_percent: 45, memory_percent: 60, disk_percent: 25 },
        { cpu_percent: 80, memory_percent: 85, disk_percent: 40 },
        { cpu_percent: 30, memory_percent: 45, disk_percent: 20 }
      ];

      const resourceSnapshots = [];

      for (const pattern of resourcePatterns) {
        const resourceUsage = DataGenerators.createResourceUsage({
          ...pattern,
          active_jobs: 1
        });

        await client.bots.sendHeartbeat({
          id: botId,
          botHeartbeatRequest: {
            status: 'online',
            resource_usage: resourceUsage
          }
        });

        const botResponse = await client.bots.getBot({ id: botId });
        resourceSnapshots.push(botResponse.resource_usage);

        // Small delay between heartbeats
        await ApiHelpers.sleep(500);
      }

      // Verify all resource snapshots were recorded correctly
      expect(resourceSnapshots).toHaveLength(resourcePatterns.length);
      
      for (let i = 0; i < resourceSnapshots.length; i++) {
        const snapshot = resourceSnapshots[i];
        const expected = resourcePatterns[i];
        
        expect(snapshot?.cpu_percent).toBe(expected.cpu_percent);
        expect(snapshot?.memory_percent).toBe(expected.memory_percent);
        expect(snapshot?.disk_percent).toBe(expected.disk_percent);
      }
    });

    test('should reject heartbeat from unregistered bot', async () => {
      const unregisteredBotId = DataGenerators.generateId('unregistered-bot');
      
      const heartbeatData = {
        status: 'online' as const,
        resource_usage: DataGenerators.createResourceUsage()
      };

      await expect(client.bots.sendHeartbeat({
        id: unregisteredBotId,
        botHeartbeatRequest: heartbeatData
      })).rejects.toThrow();
    });
  });

  test.describe('Job Assignment and Processing', () => {
    let botId: string;

    test.beforeEach(async () => {
      const botData = DataGenerators.createBotRegistration();
      botId = botData.id;
      testSetup.trackBot(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });

      // Bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 15.0,
            memory_percent: 25.0,
            disk_percent: 30.0,
            active_jobs: 0
          })
        }
      });
    });

    test('should receive job assignment through heartbeat', async () => {
      // Create a test job
      const jobData = DataGenerators.createJobData({
        name: DataGenerators.generateId('assignment-test'),
        description: 'Job for testing assignment mechanism',
        duration: 60, // Short duration for test
        fuzzer_type: 'libfuzzer'
      });

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      testSetup.trackJob(jobResponse.id!);

      // Send heartbeat to potentially receive job assignment
      const heartbeatResponse = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 10.0,
            memory_percent: 20.0,
            disk_percent: 15.0,
            active_jobs: 0
          })
        }
      });

      // Check for job assignment commands
      const assignmentCommands = heartbeatResponse.commands?.filter(
        cmd => cmd.type === 'START_JOB'
      ) || [];

      if (assignmentCommands.length > 0) {
        const assignedJob = assignmentCommands[0];
        expect(assignedJob.job_id).toBeDefined();
        expect(assignedJob.job_config).toBeDefined();
        expect(assignedJob.job_id).toBe(jobResponse.id);
      }

      // Wait for job to potentially be assigned and started
      await ApiHelpers.waitForJobStatus(
        jobResponse.id!, 
        ['running', 'completed', 'assigned'], 
        30000
      );
    });

    test('should report job progress during execution', async () => {
      // Create a longer-running job to observe progress
      const jobData = DataGenerators.createJobData({
        name: DataGenerators.generateId('progress-test'),
        description: 'Job for testing progress reporting',
        duration: 120, // 2 minutes to allow progress observation
        fuzzer_type: 'libfuzzer'
      });

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      testSetup.trackJob(jobResponse.id!);

      // Wait for job assignment and start
      await ApiHelpers.waitForJobStatus(jobResponse.id!, ['running'], 30000);

      // Monitor job progress over several checks
      let progressReported = false;
      const maxChecks = 10;
      
      for (let i = 0; i < maxChecks; i++) {
        const jobStatus = await client.jobs.getJob({ id: jobResponse.id! });
        
        if (jobStatus.progress) {
          progressReported = true;
          expect(jobStatus.progress.executions).toBeGreaterThanOrEqual(0);
          expect(typeof jobStatus.progress.executions).toBe('number');
          
          if (jobStatus.progress.coverage) {
            expect(typeof jobStatus.progress.coverage).toBe('object');
          }
          
          break;
        }
        
        await ApiHelpers.sleep(2000); // Wait 2 seconds between checks
      }

      // Progress should eventually be reported
      expect(progressReported).toBe(true);
    });

    test('should handle job cancellation gracefully', async () => {
      // Create a long-running job
      const jobData = DataGenerators.createJobData({
        name: DataGenerators.generateId('cancellation-test'),
        description: 'Job for testing cancellation',
        duration: 300, // 5 minutes - enough time to cancel
        fuzzer_type: 'afl++'
      });

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      testSetup.trackJob(jobResponse.id!);

      // Wait for job to start
      await ApiHelpers.waitForJobStatus(jobResponse.id!, ['running'], 30000);

      // Cancel the job
      await client.jobs.cancelJob({ id: jobResponse.id! });

      // Wait for cancellation to be processed
      await ApiHelpers.waitForJobStatus(jobResponse.id!, ['cancelled'], 15000);

      // Verify final job state
      const finalJobStatus = await client.jobs.getJob({ id: jobResponse.id! });
      expect(finalJobStatus.status).toBe('cancelled');
      expect(finalJobStatus.ended_at).toHaveValidTimestamp();
    });

    test('should prevent job assignment when resources are exhausted', async () => {
      // Report high resource usage indicating exhaustion
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 95,
            memory_percent: 98,
            disk_percent: 85,
            active_jobs: 4 // At maximum capacity
          })
        }
      });

      // Create a resource-intensive job
      const jobData = DataGenerators.createJobData({
        name: DataGenerators.generateId('resource-limit-test'),
        description: 'Job for testing resource limits',
        memory_limit: 512 * 1024 * 1024, // 512MB - high memory requirement
        fuzzer_type: 'afl++'
      });

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      testSetup.trackJob(jobResponse.id!);

      // Send heartbeat - should not receive job assignment due to resources
      const heartbeatResponse = await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 95,
            memory_percent: 98,
            disk_percent: 85,
            active_jobs: 4
          })
        }
      });

      // Should not get job assignment commands due to resource constraints
      const jobCommands = heartbeatResponse.commands?.filter(
        cmd => cmd.type === 'START_JOB'
      ) || [];

      // If no job commands were issued, job should remain in pending/queued state
      const jobStatus = await client.jobs.getJob({ id: jobResponse.id! });
      if (jobCommands.length === 0) {
        expect(['pending', 'queued']).toContain(jobStatus.status);
      }
    });
  });

  test.describe('Bot Disconnection and Reconnection', () => {
    let botId: string;

    test.beforeEach(async () => {
      const botData = DataGenerators.createBotRegistration();
      botId = botData.id;
      testSetup.trackBot(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });
    });

    test('should handle graceful disconnection', async () => {
      // Bring bot online first
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 5.0,
            memory_percent: 10.0,
            disk_percent: 5.0,
            active_jobs: 0
          })
        }
      });

      // Verify bot is online
      let botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');

      // Send offline status (graceful disconnection)
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'offline',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 0,
            memory_percent: 0,
            disk_percent: 0,
            active_jobs: 0
          })
        }
      });

      // Verify bot is marked offline
      botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('offline');
    });

    test('should handle bot reconnection after timeout', async () => {
      // Initial connection
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 20.0,
            memory_percent: 40.0,
            disk_percent: 25.0,
            active_jobs: 0
          })
        }
      });

      let botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');

      // Simulate network disconnection by not sending heartbeat for a period
      await ApiHelpers.sleep(5000); // 5 seconds

      // Reconnect with fresh heartbeat
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 15.0,
            memory_percent: 35.0,
            disk_percent: 20.0,
            active_jobs: 0
          })
        }
      });

      // Verify bot is back online
      botResponse = await client.bots.getBot({ id: botId });
      expect(botResponse.status).toBe('online');
      expect(botResponse.last_seen).toHaveValidTimestamp();
    });

    test('should recover job state after reconnection', async () => {
      // Create a long-running job
      const jobData = DataGenerators.createJobData({
        name: DataGenerators.generateId('recovery-test'),
        description: 'Job for testing recovery after reconnection',
        duration: 300, // 5 minutes
        fuzzer_type: 'libfuzzer'
      });

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      testSetup.trackJob(jobResponse.id!);

      // Bring bot online and let it get assigned the job
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 10.0,
            memory_percent: 20.0,
            disk_percent: 15.0,
            active_jobs: 0
          })
        }
      });

      // Wait for job assignment/start
      await ApiHelpers.waitForJobStatus(jobResponse.id!, ['running'], 30000);

      // Simulate brief disconnection
      await ApiHelpers.sleep(3000);

      // Reconnect with job in progress
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 25.0,
            memory_percent: 45.0,
            disk_percent: 20.0,
            active_jobs: 1 // Indicating job is still running
          })
        }
      });

      // Job should continue running or complete normally
      const jobStatus = await client.jobs.getJob({ id: jobResponse.id! });
      expect(['running', 'completed', 'failed']).toContain(jobStatus.status || '');
    });
  });

  test.describe('Event-Driven Bot Interactions', () => {
    let botId: string;

    test.beforeEach(async () => {
      const botData = DataGenerators.createBotRegistration();
      botId = botData.id;
      testSetup.trackBot(botId);
      
      await client.bots.createBot({
        botCreateRequest: botData
      });

      // Start event collection
      eventCollector.startCollecting();
    });

    test.afterEach(() => {
      eventCollector.stopCollecting();
    });

    test('should emit bot status change events', async () => {
      // Bring bot online
      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 5,
            memory_percent: 10,
            disk_percent: 5,
            active_jobs: 0
          })
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
          resource_usage: DataGenerators.createResourceUsage({
            cpu_percent: 10,
            memory_percent: 15,
            disk_percent: 10,
            active_jobs: 0
          })
        }
      });

      // Create a job
      const jobData = DataGenerators.createJobData({
        name: DataGenerators.generateId('event-assignment-test'),
        description: 'Job for testing assignment events'
      });

      const jobResponse = await client.jobs.createJob({
        jobCreateRequest: jobData
      });

      testSetup.trackJob(jobResponse.id!);

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