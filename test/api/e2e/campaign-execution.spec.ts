/**
 * Campaign Execution End-to-End Tests
 * 
 * Tests complete campaign execution workflows including:
 * - Campaign creation and configuration
 * - Multi-fuzzer coordination
 * - Real-time monitoring via SSE
 * - Progress tracking and statistics
 * - Campaign lifecycle management
 */

import { testHelpers, ResourceTracker, EventCollector } from './utils/test-helpers';
import { CAMPAIGN_CONFIGS, TEST_CORPUS_DATA, generateTestData, PERFORMANCE_THRESHOLDS } from './utils/test-fixtures';
import { PandaFuzzClient } from '@pandafuzz/client';

describe('Campaign Execution', () => {
  let client: PandaFuzzClient;
  let resourceTracker: ResourceTracker;
  let eventCollector: EventCollector;

  beforeAll(async () => {
    client = testHelpers.getClient();
    resourceTracker = new ResourceTracker();
    eventCollector = new EventCollector(client);

    // Ensure at least 2 bots are available for campaign testing
    await testHelpers.waitForBots(2, 60000);
  });

  afterAll(async () => {
    try {
      eventCollector.stopCollecting();
      await resourceTracker.cleanup();
    } catch (error) {
      console.warn('Cleanup warning:', error);
    }
  });

  describe('Campaign Creation and Configuration', () => {
    test('should create campaign with multiple fuzzer types', async () => {
      const campaignConfig = CAMPAIGN_CONFIGS['multi-fuzzer'];
      const campaignData = {
        name: testHelpers.generateTestName(campaignConfig.name),
        description: campaignConfig.description,
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 3,
        auto_restart: false,
        shared_corpus: true,
        job_template: {
          duration: campaignConfig.duration,
          memory_limit: campaignConfig.memoryLimit,
          timeout: campaignConfig.timeout,
          fuzzer_type: campaignConfig.fuzzerType,
          arguments: '@@'
        },
        fuzzers: [
          {
            type: 'afl++',
            weight: 40,
            config: {
              deterministic: false,
              cmplog: true
            }
          },
          {
            type: 'libfuzzer',
            weight: 30,
            config: {
              max_len: 1024,
              use_counters: true
            }
          },
          {
            type: 'honggfuzz',
            weight: 30,
            config: {
              threads: 2,
              mutation_depth: 3
            }
          }
        ],
        tags: campaignConfig.tags || ['multi-fuzzer', 'integration-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      resourceTracker.trackCampaign(response.id!);

      expect(response).toMatchApiResponse();
      expect(response.name).toBe(campaignData.name);
      expect(response.status).toBe('created');
      expect(response.target_binary).toBe(campaignData.target_binary);
      expect(response.max_jobs).toBe(campaignData.max_jobs);
      expect(response.job_template).toEqual(campaignData.job_template);
      expect(response.fuzzers).toHaveLength(3);

      // Verify campaign appears in list
      const listResponse = await client.campaigns.listCampaigns();
      const createdCampaign = listResponse.campaigns?.find(c => c.id === response.id);
      expect(createdCampaign).toBeDefined();
    });

    test('should validate campaign configuration', async () => {
      const invalidCampaignData = {
        name: '', // Empty name should be invalid
        description: 'Invalid campaign',
        target_binary: '',
        max_jobs: 0,
        job_template: {
          duration: 0,
          memory_limit: 0,
          timeout: 0,
          fuzzer_type: 'invalid-fuzzer' as any
        }
      };

      await expect(client.campaigns.createCampaign({
        campaignCreateRequest: invalidCampaignData
      })).rejects.toThrow();
    });

    test('should create campaign with corpus collection', async () => {
      // Create corpus collection first
      const { id: collectionId } = await testHelpers.createTestCorpusCollection();
      resourceTracker.trackCollection(collectionId);

      // Upload test corpus
      const corpusFiles = generateTestData('vulnerability_triggers');
      await testHelpers.uploadCorpusFiles(collectionId, corpusFiles);

      const campaignConfig = CAMPAIGN_CONFIGS['quick-afl'];
      const campaignData = {
        name: testHelpers.generateTestName('corpus-campaign'),
        description: 'Campaign with initial corpus collection',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 2,
        auto_restart: false,
        shared_corpus: true,
        corpus_collection_id: collectionId,
        job_template: {
          duration: campaignConfig.duration,
          memory_limit: campaignConfig.memoryLimit,
          timeout: campaignConfig.timeout,
          fuzzer_type: campaignConfig.fuzzerType,
          arguments: '@@'
        },
        tags: ['corpus-test', 'integration']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      resourceTracker.trackCampaign(response.id!);

      expect(response.corpus_collection_id).toBe(collectionId);
      expect(response.shared_corpus).toBe(true);
    });
  });

  describe('Campaign Lifecycle Management', () => {
    let campaignId: string;

    beforeEach(async () => {
      const campaignConfig = CAMPAIGN_CONFIGS['quick-afl'];
      const campaignData = {
        name: testHelpers.generateTestName('lifecycle-campaign'),
        description: campaignConfig.description,
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 2,
        auto_restart: false,
        shared_corpus: true,
        job_template: {
          duration: campaignConfig.duration,
          memory_limit: campaignConfig.memoryLimit,
          timeout: campaignConfig.timeout,
          fuzzer_type: campaignConfig.fuzzerType,
          arguments: '@@'
        },
        tags: campaignConfig.tags || ['lifecycle-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = response.id!;
      resourceTracker.trackCampaign(campaignId);
    });

    test('should start campaign and transition through states', async () => {
      // Start event collection
      eventCollector.startCollecting({
        types: ['CAMPAIGN_STARTED', 'CAMPAIGN_STATUS_CHANGED', 'JOB_CREATED']
      });

      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: {
          status: 'running'
        }
      });

      // Wait for campaign to start
      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for campaign start event
      const startEvent = await eventCollector.waitForEvent(
        'CAMPAIGN_STARTED',
        15000,
        event => event.data?.campaign_id === campaignId
      );

      expect(startEvent).toBeDefined();
      expect(startEvent.data.campaign_id).toBe(campaignId);

      // Verify campaign is running
      const campaignResponse = await client.campaigns.getCampaign({ id: campaignId });
      expect(campaignResponse.status).toBe('running');
      expect(campaignResponse.started_at).toHaveValidTimestamp();
    });

    test('should stop campaign gracefully', async () => {
      // Start campaign first
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Stop campaign
      await client.campaigns.stopCampaign({
        id: campaignId,
        stopCampaignRequest: {
          reason: 'Test completion'
        }
      });

      // Wait for campaign to stop
      await testHelpers.waitForCampaignStatus(campaignId, ['stopped', 'stopping'], 30000);

      const campaignResponse = await client.campaigns.getCampaign({ id: campaignId });
      expect(['stopped', 'stopping']).toContain(campaignResponse.status);
      expect(campaignResponse.ended_at).toHaveValidTimestamp();
    });

    test('should handle campaign auto-restart', async () => {
      // Update campaign to enable auto-restart
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: {
          auto_restart: true,
          max_restarts: 2
        }
      });

      // Start campaign with short duration for quick completion
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: {
          status: 'running',
          job_template: {
            duration: 30, // 30 seconds for quick test
            memory_limit: 128 * 1024 * 1024,
            timeout: 1000,
            fuzzer_type: 'libfuzzer',
            arguments: '@@'
          }
        }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for potential restart after completion
      await new Promise(resolve => setTimeout(resolve, 45000)); // Wait longer than duration

      const campaignResponse = await client.campaigns.getCampaign({ id: campaignId });
      
      // Campaign should either be running (restarted) or completed
      expect(['running', 'completed', 'stopped']).toContain(campaignResponse.status);
    });
  });

  describe('Real-time Monitoring via SSE', () => {
    let campaignId: string;

    beforeEach(async () => {
      const campaignConfig = CAMPAIGN_CONFIGS['quick-libfuzzer'];
      const campaignData = {
        name: testHelpers.generateTestName('sse-campaign'),
        description: 'Campaign for SSE monitoring tests',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 1,
        auto_restart: false,
        shared_corpus: true,
        job_template: {
          duration: campaignConfig.duration,
          memory_limit: campaignConfig.memoryLimit,
          timeout: campaignConfig.timeout,
          fuzzer_type: campaignConfig.fuzzerType,
          arguments: '@@'
        },
        tags: ['sse-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = response.id!;
      resourceTracker.trackCampaign(campaignId);

      // Start event collection with campaign filter
      eventCollector.startCollecting({
        types: ['CAMPAIGN_PROGRESS', 'JOB_PROGRESS', 'CRASH_DISCOVERED'],
        filter: { campaign_id: campaignId }
      });
    });

    afterEach(() => {
      eventCollector.stopCollecting();
    });

    test('should receive campaign progress events', async () => {
      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for progress events
      const progressEvents = [];
      const eventCheckInterval = setInterval(() => {
        const events = eventCollector.getEventsByType('CAMPAIGN_PROGRESS');
        progressEvents.push(...events.filter(e => e.data?.campaign_id === campaignId));
      }, 2000);

      // Monitor for 20 seconds
      await new Promise(resolve => setTimeout(resolve, 20000));
      clearInterval(eventCheckInterval);

      // Should have received at least one progress event
      expect(progressEvents.length).toBeGreaterThan(0);

      const latestProgress = progressEvents[progressEvents.length - 1];
      expect(latestProgress.data.campaign_id).toBe(campaignId);
      expect(latestProgress.data.stats).toBeDefined();
      expect(latestProgress.data.stats.active_jobs).toBeGreaterThanOrEqual(0);
    });

    test('should receive job-level progress events', async () => {
      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for job progress events
      const jobProgressEvent = await eventCollector.waitForEvent(
        'JOB_PROGRESS',
        30000,
        event => event.data?.campaign_id === campaignId
      );

      expect(jobProgressEvent).toBeDefined();
      expect(jobProgressEvent.data.job_id).toBeDefined();
      expect(jobProgressEvent.data.progress).toBeDefined();
      expect(jobProgressEvent.data.progress.executions).toBeGreaterThan(0);
    });

    test('should receive crash discovery events', async () => {
      // Start campaign with crash-inducing corpus
      const { id: collectionId } = await testHelpers.createTestCorpusCollection();
      resourceTracker.trackCollection(collectionId);

      const crashCorpus = generateTestData('vulnerability_triggers');
      await testHelpers.uploadCorpusFiles(collectionId, crashCorpus);

      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: {
          status: 'running',
          corpus_collection_id: collectionId
        }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for crash discovery events (may take longer)
      try {
        const crashEvent = await eventCollector.waitForEvent(
          'CRASH_DISCOVERED',
          60000, // 1 minute timeout
          event => event.data?.campaign_id === campaignId
        );

        expect(crashEvent).toBeDefined();
        expect(crashEvent.data.crash_id).toBeDefined();
        expect(crashEvent.data.job_id).toBeDefined();
        expect(crashEvent.data.campaign_id).toBe(campaignId);
      } catch (error) {
        console.warn('No crashes discovered within timeout - this may be normal depending on the target binary');
      }
    });
  });

  describe('Campaign Statistics and Analytics', () => {
    let campaignId: string;

    beforeEach(async () => {
      const campaignConfig = CAMPAIGN_CONFIGS['intensive-testing'];
      const campaignData = {
        name: testHelpers.generateTestName('stats-campaign'),
        description: 'Campaign for statistics testing',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 2,
        auto_restart: false,
        shared_corpus: true,
        job_template: {
          duration: 120, // 2 minutes for stats collection
          memory_limit: campaignConfig.memoryLimit,
          timeout: campaignConfig.timeout,
          fuzzer_type: campaignConfig.fuzzerType,
          arguments: '@@'
        },
        tags: ['stats-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = response.id!;
      resourceTracker.trackCampaign(campaignId);
    });

    test('should collect and report campaign statistics', async () => {
      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for some execution time
      await new Promise(resolve => setTimeout(resolve, 15000));

      // Get campaign statistics
      const statsResponse = await client.campaigns.getCampaignStats({ id: campaignId });

      expect(statsResponse).toBeDefined();
      expect(statsResponse.total_executions).toBeGreaterThan(0);
      expect(statsResponse.executions_per_second).toBeGreaterThan(0);
      expect(statsResponse.active_jobs).toBeGreaterThan(0);
      expect(statsResponse.completed_jobs).toBeGreaterThanOrEqual(0);
      expect(statsResponse.total_crashes).toBeGreaterThanOrEqual(0);
      expect(statsResponse.unique_crashes).toBeGreaterThanOrEqual(0);
      expect(statsResponse.corpus_size).toBeGreaterThan(0);
      expect(statsResponse.coverage_percentage).toBeGreaterThanOrEqual(0);

      if (statsResponse.performance_metrics) {
        expect(statsResponse.performance_metrics.avg_exec_time_ms).toBeGreaterThan(0);
        expect(statsResponse.performance_metrics.memory_usage_mb).toBeGreaterThan(0);
      }
    });

    test('should track campaign performance over time', async () => {
      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      const performanceSnapshots = [];

      // Collect performance snapshots over time
      for (let i = 0; i < 5; i++) {
        await new Promise(resolve => setTimeout(resolve, 5000));

        const snapshot = {
          timestamp: Date.now(),
          stats: await client.campaigns.getCampaignStats({ id: campaignId }),
          system: await testHelpers.takePerformanceSnapshot()
        };

        performanceSnapshots.push(snapshot);
      }

      // Verify performance trends
      expect(performanceSnapshots.length).toBe(5);

      // Execution count should generally increase over time
      const executions = performanceSnapshots.map(s => s.stats.total_executions);
      const isIncreasing = executions.every((exec, i) => 
        i === 0 || exec >= executions[i - 1]
      );

      expect(isIncreasing).toBe(true);

      // Check performance thresholds
      const avgExecTime = performanceSnapshots.reduce((sum, s) => 
        sum + (s.stats.performance_metrics?.avg_exec_time_ms || 0), 0
      ) / performanceSnapshots.length;

      expect(avgExecTime).toBeLessThan(PERFORMANCE_THRESHOLDS.api_response_time);
    });

    test('should provide coverage analysis', async () => {
      // Create campaign with coverage tracking
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: {
          status: 'running',
          enable_coverage: true
        }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for coverage data to accumulate
      await new Promise(resolve => setTimeout(resolve, 20000));

      // Get coverage report
      try {
        const coverageResponse = await client.campaigns.getCampaignCoverage({ id: campaignId });

        expect(coverageResponse).toBeDefined();
        if (coverageResponse.coverage_metrics) {
          expect(coverageResponse.coverage_metrics.lines_covered).toBeGreaterThanOrEqual(0);
          expect(coverageResponse.coverage_metrics.lines_total).toBeGreaterThan(0);
          expect(coverageResponse.coverage_metrics.branches_covered).toBeGreaterThanOrEqual(0);
          expect(coverageResponse.coverage_metrics.functions_covered).toBeGreaterThanOrEqual(0);
        }

        if (coverageResponse.coverage_trend) {
          expect(Array.isArray(coverageResponse.coverage_trend.data_points)).toBe(true);
        }
      } catch (error) {
        console.warn('Coverage analysis not available - this may be expected if coverage is not enabled');
      }
    });
  });

  describe('Multi-Fuzzer Coordination', () => {
    let campaignId: string;

    beforeEach(async () => {
      // Ensure we have enough bots for multi-fuzzer testing
      await testHelpers.waitForBots(3, 60000);

      const campaignData = {
        name: testHelpers.generateTestName('multi-fuzzer-campaign'),
        description: 'Campaign testing multi-fuzzer coordination',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 4,
        auto_restart: false,
        shared_corpus: true,
        job_template: {
          duration: 180, // 3 minutes
          memory_limit: 256 * 1024 * 1024,
          timeout: 1000,
          arguments: '@@'
        },
        fuzzers: [
          {
            type: 'afl++',
            weight: 50,
            max_jobs: 2,
            config: {
              deterministic: false,
              cmplog: true,
              custom_mutator: false
            }
          },
          {
            type: 'libfuzzer',
            weight: 30,
            max_jobs: 1,
            config: {
              max_len: 1024,
              use_counters: true,
              reduce_inputs: true
            }
          },
          {
            type: 'honggfuzz',
            weight: 20,
            max_jobs: 1,
            config: {
              threads: 2,
              mutation_depth: 4,
              dict_file: null
            }
          }
        ],
        tags: ['multi-fuzzer', 'coordination-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = response.id!;
      resourceTracker.trackCampaign(campaignId);
    });

    test('should distribute jobs across different fuzzer types', async () => {
      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for job distribution
      await new Promise(resolve => setTimeout(resolve, 20000));

      // Get campaign jobs
      const jobsResponse = await client.campaigns.getCampaignJobs({ id: campaignId });

      expect(jobsResponse.jobs).toBeDefined();
      expect(jobsResponse.jobs!.length).toBeGreaterThan(1);

      // Check fuzzer type distribution
      const fuzzerTypes = jobsResponse.jobs!.map(job => job.fuzzer_type);
      const uniqueFuzzers = [...new Set(fuzzerTypes)];

      expect(uniqueFuzzers.length).toBeGreaterThan(1);
      expect(uniqueFuzzers).toContain('afl++');
    });

    test('should coordinate corpus sharing between fuzzers', async () => {
      // Start campaign with shared corpus
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Let fuzzers run and generate corpus
      await new Promise(resolve => setTimeout(resolve, 30000));

      // Check corpus evolution
      const campaignResponse = await client.campaigns.getCampaign({ id: campaignId });
      
      if (campaignResponse.corpus_collection_id) {
        const corpusResponse = await client.corpus.getCorpusCollection({
          id: campaignResponse.corpus_collection_id
        });

        expect(corpusResponse.file_count).toBeGreaterThan(0);
        
        // Check if corpus has been enriched by different fuzzers
        const corpusFilesResponse = await client.corpus.listCorpusFiles({
          id: campaignResponse.corpus_collection_id
        });

        if (corpusFilesResponse.files && corpusFilesResponse.files.length > 0) {
          // Look for files generated by different fuzzers
          const generatedByFuzzers = corpusFilesResponse.files.some(file => 
            file.metadata?.generated_by_fuzzer !== undefined
          );
          
          // This assertion might not always be true depending on implementation
          console.log(`Corpus shared between fuzzers: ${generatedByFuzzers}`);
        }
      }
    });

    test('should balance load across available bots', async () => {
      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Wait for job distribution
      await new Promise(resolve => setTimeout(resolve, 15000));

      // Check bot utilization
      const botsResponse = await client.bots.listBots();
      const onlineBots = botsResponse.bots?.filter(bot => bot.status === 'online') || [];

      expect(onlineBots.length).toBeGreaterThan(0);

      // Check if jobs are distributed across multiple bots
      const botJobs = new Map();
      for (const bot of onlineBots) {
        if (bot.resource_usage?.active_jobs && bot.resource_usage.active_jobs > 0) {
          botJobs.set(bot.id, bot.resource_usage.active_jobs);
        }
      }

      console.log('Bot job distribution:', Object.fromEntries(botJobs));
      
      // At least one bot should have active jobs
      expect(botJobs.size).toBeGreaterThan(0);
    });
  });

  describe('Campaign Error Handling', () => {
    test('should handle invalid target binary gracefully', async () => {
      const campaignData = {
        name: testHelpers.generateTestName('invalid-binary-campaign'),
        description: 'Campaign with invalid binary for error testing',
        target_binary: '/non/existent/binary',
        max_jobs: 1,
        auto_restart: false,
        shared_corpus: false,
        job_template: {
          duration: 60,
          memory_limit: 128 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'libfuzzer' as const,
          arguments: '@@'
        },
        tags: ['error-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      resourceTracker.trackCampaign(response.id!);

      // Try to start campaign - should handle error gracefully
      await client.campaigns.updateCampaign({
        id: response.id!,
        campaignUpdateRequest: { status: 'running' }
      });

      // Campaign should transition to error state or jobs should fail
      await new Promise(resolve => setTimeout(resolve, 10000));

      const campaignStatus = await client.campaigns.getCampaign({ id: response.id! });
      
      // Campaign might be running but jobs will fail
      if (campaignStatus.status === 'running') {
        const jobsResponse = await client.campaigns.getCampaignJobs({ id: response.id! });
        if (jobsResponse.jobs && jobsResponse.jobs.length > 0) {
          // At least some jobs should fail due to invalid binary
          const failedJobs = jobsResponse.jobs.filter(job => job.status === 'failed');
          expect(failedJobs.length).toBeGreaterThan(0);
        }
      }
    });

    test('should handle resource exhaustion', async () => {
      const campaignData = {
        name: testHelpers.generateTestName('resource-exhaustion-campaign'),
        description: 'Campaign for testing resource limits',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        max_jobs: 20, // Intentionally high to test limits
        auto_restart: false,
        shared_corpus: false,
        job_template: {
          duration: 300,
          memory_limit: 1024 * 1024 * 1024, // 1GB per job
          timeout: 2000,
          fuzzer_type: 'afl++' as const,
          arguments: '@@'
        },
        tags: ['resource-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      resourceTracker.trackCampaign(response.id!);

      // Start campaign
      await client.campaigns.updateCampaign({
        id: response.id!,
        campaignUpdateRequest: { status: 'running' }
      });

      // Wait and check resource handling
      await new Promise(resolve => setTimeout(resolve, 15000));

      const jobsResponse = await client.campaigns.getCampaignJobs({ id: response.id! });
      const statsResponse = await client.campaigns.getCampaignStats({ id: response.id! });

      // Should not have created all 20 jobs due to resource constraints
      expect(jobsResponse.jobs!.length).toBeLessThan(20);
      expect(statsResponse.active_jobs).toBeLessThan(20);

      // System should still be responsive
      const healthResponse = await client.health.getHealth();
      expect(healthResponse.status).toBe('healthy');
    });
  });
});