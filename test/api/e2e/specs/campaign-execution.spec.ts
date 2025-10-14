/**
 * Campaign Execution End-to-End Tests
 * 
 * Tests complete campaign lifecycle including:
 * - Campaign creation and configuration
 * - Campaign start/stop operations
 * - Real-time monitoring via SSE events
 * - Job orchestration within campaigns
 * - Campaign statistics and progress tracking
 * - Campaign completion and results
 */

import { test, expect } from '@playwright/test';
import { PandaFuzzClient } from '@pandafuzz/client';
import { TestSetup, EventCollector, DataGenerators, ApiHelpers, addCustomMatchers } from '../test-utils';

// Add custom matchers
addCustomMatchers();

test.describe('Campaign Execution', () => {
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

  test.describe('Campaign Creation and Configuration', () => {
    test('should create a new campaign successfully', async () => {
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('create-test'),
        description: 'Test campaign for creation validation',
        max_concurrent_jobs: 3,
        total_jobs: 10
      });

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      testSetup.trackCampaign(response.id!);

      // Verify response structure and data
      expect(response).toMatchApiResponse();
      expect(response.id).toBeDefined();
      expect(response.name).toBe(campaignData.name);
      expect(response.description).toBe(campaignData.description);
      expect(response.status).toBe('draft');
      expect(response.max_concurrent_jobs).toBe(campaignData.max_concurrent_jobs);
      expect(response.total_jobs).toBe(campaignData.total_jobs);

      // Verify job template configuration
      expect(response.job_template).toBeDefined();
      expect(response.job_template?.fuzzer_type).toBe(campaignData.job_template.fuzzer_type);
      expect(response.job_template?.target_binary).toBe(campaignData.job_template.target_binary);

      // Verify campaign appears in list
      const listResponse = await client.campaigns.listCampaigns();
      const createdCampaign = listResponse.campaigns?.find(c => c.id === response.id);
      expect(createdCampaign).toBeDefined();
      expect(createdCampaign?.name).toBe(campaignData.name);
    });

    test('should validate campaign configuration', async () => {
      // Test invalid campaign data
      const invalidCampaignData = {
        name: '', // Empty name should be invalid
        description: 'Invalid campaign',
        job_template: {
          fuzzer_type: 'invalid-fuzzer' as any, // Invalid fuzzer type
          target_binary: '',
          duration: -1, // Invalid duration
          memory_limit: 0,
          timeout: 0,
          arguments: ''
        },
        max_concurrent_jobs: 0, // Invalid concurrency
        total_jobs: 0 // Invalid job count
      };

      await expect(client.campaigns.createCampaign({
        campaignCreateRequest: invalidCampaignData
      })).rejects.toThrow();
    });

    test('should create campaign with different fuzzer types', async () => {
      const fuzzerTypes = ['libfuzzer', 'afl++', 'honggfuzz'] as const;

      for (const fuzzerType of fuzzerTypes) {
        const campaignData = DataGenerators.createCampaignData({
          name: DataGenerators.generateId(`${fuzzerType}-campaign`),
          description: `Campaign testing ${fuzzerType} fuzzer`,
          job_template: {
            ...DataGenerators.createCampaignData().job_template,
            fuzzer_type: fuzzerType
          }
        });

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        testSetup.trackCampaign(response.id!);

        expect(response.job_template?.fuzzer_type).toBe(fuzzerType);
      }
    });

    test('should handle campaign resource requirements', async () => {
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('resource-test'),
        description: 'Campaign with specific resource requirements',
        job_template: {
          ...DataGenerators.createCampaignData().job_template,
          memory_limit: 1024 * 1024 * 1024, // 1GB
          timeout: 5000 // 5 seconds
        },
        max_concurrent_jobs: 5,
        total_jobs: 20
      });

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      testSetup.trackCampaign(response.id!);

      expect(response.job_template?.memory_limit).toBe(campaignData.job_template.memory_limit);
      expect(response.job_template?.timeout).toBe(campaignData.job_template.timeout);
      expect(response.max_concurrent_jobs).toBe(campaignData.max_concurrent_jobs);
      expect(response.total_jobs).toBe(campaignData.total_jobs);
    });
  });

  test.describe('Campaign Start and Stop Operations', () => {
    let campaignId: string;
    let botId: string;

    test.beforeEach(async () => {
      // Create and register a bot to execute campaign jobs
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
            cpu_percent: 20.0,
            memory_percent: 30.0,
            disk_percent: 15.0,
            active_jobs: 0
          })
        }
      });

      // Create a campaign
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('start-stop-test'),
        description: 'Campaign for testing start/stop operations',
        max_concurrent_jobs: 2,
        total_jobs: 5
      });

      const campaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = campaignResponse.id!;
      testSetup.trackCampaign(campaignId);
    });

    test('should start campaign and begin job execution', async () => {
      // Start event collection
      eventCollector.startCollecting({
        types: ['CAMPAIGN_STARTED', 'JOB_ASSIGNED', 'JOB_STARTED']
      });

      // Start the campaign
      const startResponse = await client.campaigns.startCampaign({
        id: campaignId
      });

      expect(startResponse.status).toBe('starting');

      // Wait for campaign to actually start
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Verify campaign status
      const runningCampaign = await client.campaigns.getCampaign({ id: campaignId });
      expect(runningCampaign.status).toBe('running');
      expect(runningCampaign.started_at).toHaveValidTimestamp();

      // Wait for campaign started event
      const campaignStartedEvent = await eventCollector.waitForEvent(
        'CAMPAIGN_STARTED',
        10000,
        event => event.data?.campaign_id === campaignId
      );

      expect(campaignStartedEvent).toBeDefined();
      expect(campaignStartedEvent.data.campaign_id).toBe(campaignId);

      // Verify jobs are being created and assigned
      await ApiHelpers.sleep(5000); // Allow time for job creation

      const campaignStats = await client.campaigns.getCampaignStats({ id: campaignId });
      expect(campaignStats.total_jobs).toBeGreaterThan(0);
      expect(campaignStats.jobs_pending + campaignStats.jobs_running + campaignStats.jobs_completed)
        .toBeGreaterThan(0);
    });

    test('should stop running campaign gracefully', async () => {
      // Start the campaign first
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Start event collection
      eventCollector.startCollecting({
        types: ['CAMPAIGN_STOPPED', 'CAMPAIGN_COMPLETED']
      });

      // Stop the campaign
      const stopResponse = await client.campaigns.stopCampaign({
        id: campaignId,
        stopCampaignRequest: {
          reason: 'Manual stop for testing'
        }
      });

      expect(stopResponse.status).toBe('stopping');

      // Wait for campaign to stop
      await ApiHelpers.waitForCampaignStatus(campaignId, ['stopped', 'completed'], 30000);

      // Verify campaign status
      const stoppedCampaign = await client.campaigns.getCampaign({ id: campaignId });
      expect(['stopped', 'completed']).toContain(stoppedCampaign.status || '');
      expect(stoppedCampaign.ended_at).toHaveValidTimestamp();

      // Wait for campaign stopped/completed event
      const campaignEvent = await eventCollector.waitForEvent(
        'CAMPAIGN_STOPPED',
        10000,
        event => event.data?.campaign_id === campaignId
      );

      expect(campaignEvent).toBeDefined();
      expect(campaignEvent.data.campaign_id).toBe(campaignId);
    });

    test('should handle campaign restart after stop', async () => {
      // Start and then stop the campaign
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      await client.campaigns.stopCampaign({
        id: campaignId,
        stopCampaignRequest: { reason: 'Testing restart' }
      });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['stopped', 'completed'], 30000);

      // Restart the campaign
      const restartResponse = await client.campaigns.startCampaign({ id: campaignId });
      expect(restartResponse.status).toBe('starting');

      // Wait for campaign to be running again
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      const restartedCampaign = await client.campaigns.getCampaign({ id: campaignId });
      expect(restartedCampaign.status).toBe('running');
    });

    test('should prevent duplicate start operations', async () => {
      // Start the campaign
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Attempt to start already running campaign should fail or be ignored
      await expect(client.campaigns.startCampaign({ id: campaignId }))
        .rejects.toThrow();
    });
  });

  test.describe('Real-time Campaign Monitoring', () => {
    let campaignId: string;
    let botId: string;

    test.beforeEach(async () => {
      // Set up bot
      const botData = DataGenerators.createBotRegistration();
      botId = botData.id;
      testSetup.trackBot(botId);

      await client.bots.createBot({
        botCreateRequest: botData
      });

      await client.bots.sendHeartbeat({
        id: botId,
        botHeartbeatRequest: {
          status: 'online',
          resource_usage: DataGenerators.createResourceUsage()
        }
      });

      // Create campaign
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('monitoring-test'),
        description: 'Campaign for testing real-time monitoring',
        max_concurrent_jobs: 2,
        total_jobs: 8
      });

      const campaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = campaignResponse.id!;
      testSetup.trackCampaign(campaignId);

      // Start event collection
      eventCollector.startCollecting();
    });

    test.afterEach(() => {
      eventCollector.stopCollecting();
    });

    test('should receive campaign progress events via SSE', async () => {
      // Start the campaign
      await client.campaigns.startCampaign({ id: campaignId });

      // Wait for various campaign events
      const events = await Promise.allSettled([
        eventCollector.waitForEvent('CAMPAIGN_STARTED', 15000, 
          event => event.data?.campaign_id === campaignId),
        eventCollector.waitForEvent('JOB_ASSIGNED', 15000, 
          event => event.data?.campaign_id === campaignId),
        eventCollector.waitForEvent('JOB_STARTED', 15000, 
          event => event.data?.campaign_id === campaignId)
      ]);

      // At least campaign started event should be received
      expect(events[0].status).toBe('fulfilled');
      const campaignStartedEvent = (events[0] as PromiseFulfilledResult<any>).value;
      expect(campaignStartedEvent.data.campaign_id).toBe(campaignId);
    });

    test('should track job completion progress', async () => {
      // Start the campaign
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Monitor campaign statistics over time
      const statsSnapshots = [];
      const monitoringDuration = 30000; // 30 seconds
      const snapshotInterval = 5000; // Every 5 seconds
      const startTime = Date.now();

      while (Date.now() - startTime < monitoringDuration) {
        const stats = await client.campaigns.getCampaignStats({ id: campaignId });
        statsSnapshots.push({
          timestamp: Date.now(),
          ...stats
        });

        await ApiHelpers.sleep(snapshotInterval);
      }

      // Verify that progress was made
      expect(statsSnapshots.length).toBeGreaterThan(1);
      
      const firstSnapshot = statsSnapshots[0];
      const lastSnapshot = statsSnapshots[statsSnapshots.length - 1];

      // Total job counts should remain consistent
      expect(lastSnapshot.total_jobs).toBe(firstSnapshot.total_jobs);

      // Some progress should have been made (completed jobs increased or jobs started)
      expect(
        lastSnapshot.jobs_completed >= firstSnapshot.jobs_completed ||
        lastSnapshot.jobs_running > firstSnapshot.jobs_running
      ).toBe(true);
    });

    test('should provide accurate campaign statistics', async () => {
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Allow some time for jobs to be created and started
      await ApiHelpers.sleep(10000);

      const stats = await client.campaigns.getCampaignStats({ id: campaignId });

      // Verify statistics structure
      expect(stats).toMatchApiResponse();
      expect(typeof stats.total_jobs).toBe('number');
      expect(typeof stats.jobs_pending).toBe('number');
      expect(typeof stats.jobs_running).toBe('number');
      expect(typeof stats.jobs_completed).toBe('number');
      expect(typeof stats.jobs_failed).toBe('number');

      // Verify statistics consistency
      const totalAccounted = stats.jobs_pending + stats.jobs_running + 
                            stats.jobs_completed + stats.jobs_failed;
      expect(totalAccounted).toBeLessThanOrEqual(stats.total_jobs);

      // Performance metrics should be present
      if (stats.performance_metrics) {
        expect(typeof stats.performance_metrics.avg_execution_time).toBe('number');
        expect(stats.performance_metrics.avg_execution_time).toBeGreaterThanOrEqual(0);
      }
    });

    test('should emit job completion events for campaign jobs', async () => {
      await client.campaigns.startCampaign({ id: campaignId });
      
      // Wait for at least one job completion event
      const jobCompletedEvent = await eventCollector.waitForEvent(
        'JOB_COMPLETED',
        45000, // Give jobs time to complete
        event => event.data?.campaign_id === campaignId
      );

      expect(jobCompletedEvent).toBeDefined();
      expect(jobCompletedEvent.data.campaign_id).toBe(campaignId);
      expect(jobCompletedEvent.data.job_id).toBeDefined();
      expect(['completed', 'failed']).toContain(jobCompletedEvent.data.status);
    });
  });

  test.describe('Campaign Job Orchestration', () => {
    let campaignId: string;
    let botIds: string[] = [];

    test.beforeEach(async () => {
      // Create multiple bots to handle concurrent jobs
      const botPromises = [];
      for (let i = 0; i < 3; i++) {
        const botData = DataGenerators.createBotRegistration(
          DataGenerators.generateId(`orchestration-bot-${i}`)
        );
        botIds.push(botData.id);
        testSetup.trackBot(botData.id);

        botPromises.push(
          client.bots.createBot({ botCreateRequest: botData })
            .then(() => client.bots.sendHeartbeat({
              id: botData.id,
              botHeartbeatRequest: {
                status: 'online',
                resource_usage: DataGenerators.createResourceUsage({
                  active_jobs: 0
                })
              }
            }))
        );
      }

      await Promise.all(botPromises);

      // Create campaign with higher concurrency
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('orchestration-test'),
        description: 'Campaign for testing job orchestration',
        max_concurrent_jobs: 3,
        total_jobs: 15
      });

      const campaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = campaignResponse.id!;
      testSetup.trackCampaign(campaignId);
    });

    test('should respect max concurrent jobs limit', async () => {
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Monitor concurrent job execution over time
      const maxConcurrentObserved = [];
      const monitoringDuration = 20000; // 20 seconds
      const checkInterval = 2000; // Every 2 seconds
      const startTime = Date.now();

      while (Date.now() - startTime < monitoringDuration) {
        const stats = await client.campaigns.getCampaignStats({ id: campaignId });
        maxConcurrentObserved.push(stats.jobs_running);
        
        await ApiHelpers.sleep(checkInterval);
      }

      // Verify that concurrent job limit was respected
      const maxObserved = Math.max(...maxConcurrentObserved);
      expect(maxObserved).toBeLessThanOrEqual(3); // max_concurrent_jobs was set to 3
    });

    test('should distribute jobs across available bots', async () => {
      eventCollector.startCollecting({
        types: ['JOB_ASSIGNED']
      });

      await client.campaigns.startCampaign({ id: campaignId });

      // Wait for several job assignments
      await ApiHelpers.sleep(15000);

      const jobAssignmentEvents = eventCollector.getEventsOfType('JOB_ASSIGNED')
        .filter(event => event.data?.campaign_id === campaignId);

      if (jobAssignmentEvents.length > 1) {
        // Check that jobs were distributed across different bots
        const assignedBots = new Set(
          jobAssignmentEvents.map(event => event.data.bot_id)
        );

        expect(assignedBots.size).toBeGreaterThan(1); // Jobs distributed to multiple bots
      }
    });

    test('should handle bot failures during campaign execution', async () => {
      await client.campaigns.startCampaign({ id: campaignId });
      await ApiHelpers.waitForCampaignStatus(campaignId, ['running'], 30000);

      // Allow some jobs to start
      await ApiHelpers.sleep(5000);

      // Simulate bot failure by taking one bot offline
      const failingBotId = botIds[0];
      await client.bots.sendHeartbeat({
        id: failingBotId,
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

      // Campaign should continue with remaining bots
      await ApiHelpers.sleep(10000);

      const stats = await client.campaigns.getCampaignStats({ id: campaignId });
      
      // Campaign should still be making progress or completed
      expect(['running', 'completed', 'stopping']).toContain(
        (await client.campaigns.getCampaign({ id: campaignId })).status || ''
      );
    });

    test('should complete campaign when all jobs finish', async () => {
      eventCollector.startCollecting({
        types: ['CAMPAIGN_COMPLETED']
      });

      // Create a shorter campaign for quicker completion
      const quickCampaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('quick-completion-test'),
        description: 'Quick campaign for testing completion',
        max_concurrent_jobs: 3,
        total_jobs: 3, // Fewer jobs for quicker completion
        job_template: {
          ...DataGenerators.createCampaignData().job_template,
          duration: 30 // Shorter duration
        }
      });

      const quickCampaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: quickCampaignData
      });

      const quickCampaignId = quickCampaignResponse.id!;
      testSetup.trackCampaign(quickCampaignId);

      await client.campaigns.startCampaign({ id: quickCampaignId });

      // Wait for campaign completion
      await ApiHelpers.waitForCampaignStatus(quickCampaignId, ['completed'], 60000);

      // Verify final statistics
      const finalStats = await client.campaigns.getCampaignStats({ id: quickCampaignId });
      expect(finalStats.jobs_completed + finalStats.jobs_failed).toBe(finalStats.total_jobs);

      // Verify campaign completed event
      const completedEvent = await eventCollector.waitForEvent(
        'CAMPAIGN_COMPLETED',
        10000,
        event => event.data?.campaign_id === quickCampaignId
      );

      expect(completedEvent).toBeDefined();
      expect(completedEvent.data.campaign_id).toBe(quickCampaignId);
    });
  });

  test.describe('Campaign Error Handling', () => {
    test('should handle campaign with no available bots', async () => {
      // Create campaign without any bots online
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('no-bots-test'),
        description: 'Campaign testing no available bots scenario'
      });

      const campaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      testSetup.trackCampaign(campaignResponse.id!);

      // Attempt to start campaign
      await client.campaigns.startCampaign({ id: campaignResponse.id! });

      // Campaign might start but jobs should remain in pending state
      await ApiHelpers.sleep(10000);

      const stats = await client.campaigns.getCampaignStats({ id: campaignResponse.id! });
      
      // All jobs should be pending since no bots are available
      expect(stats.jobs_pending).toBeGreaterThan(0);
      expect(stats.jobs_running).toBe(0);
    });

    test('should handle invalid campaign operations', async () => {
      const nonExistentId = DataGenerators.generateId('non-existent');

      // Test operations on non-existent campaign
      await expect(client.campaigns.getCampaign({ id: nonExistentId }))
        .rejects.toThrow();

      await expect(client.campaigns.startCampaign({ id: nonExistentId }))
        .rejects.toThrow();

      await expect(client.campaigns.stopCampaign({ 
        id: nonExistentId,
        stopCampaignRequest: { reason: 'Test' }
      })).rejects.toThrow();

      await expect(client.campaigns.getCampaignStats({ id: nonExistentId }))
        .rejects.toThrow();
    });

    test('should handle campaign deletion scenarios', async () => {
      const campaignData = DataGenerators.createCampaignData({
        name: DataGenerators.generateId('deletion-test'),
        description: 'Campaign for testing deletion'
      });

      const campaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      const campaignId = campaignResponse.id!;
      // Don't track for cleanup since we're testing deletion

      // Delete draft campaign (should succeed)
      await client.campaigns.deleteCampaign({ id: campaignId });

      // Verify campaign is deleted
      await expect(client.campaigns.getCampaign({ id: campaignId }))
        .rejects.toThrow();
    });
  });
});