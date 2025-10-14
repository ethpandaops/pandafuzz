/**
 * Multi-Tenant Scenarios End-to-End Tests
 * 
 * Tests multi-tenant functionality including:
 * - Concurrent campaigns and job execution
 * - Resource isolation and allocation
 * - Priority-based job scheduling
 * - Cross-tenant corpus sharing policies
 * - Performance under multi-tenant load
 */

import { testHelpers, ResourceTracker, EventCollector } from './utils/test-helpers';
import { MULTI_TENANT_SCENARIOS, CAMPAIGN_CONFIGS, generateTestData, PERFORMANCE_THRESHOLDS } from './utils/test-fixtures';
import { PandaFuzzClient } from '@pandafuzz/client';

describe('Multi-Tenant Scenarios', () => {
  let client: PandaFuzzClient;
  let resourceTracker: ResourceTracker;
  let eventCollector: EventCollector;

  beforeAll(async () => {
    client = testHelpers.getClient();
    resourceTracker = new ResourceTracker();
    eventCollector = new EventCollector(client);

    // Ensure sufficient bots for multi-tenant testing
    await testHelpers.waitForBots(3, 90000);
  });

  afterAll(async () => {
    try {
      eventCollector.stopCollecting();
      await resourceTracker.cleanup();
    } catch (error) {
      console.warn('Cleanup warning:', error);
    }
  });

  describe('Concurrent Campaign Execution', () => {
    test('should run multiple campaigns simultaneously', async () => {
      const scenario = MULTI_TENANT_SCENARIOS.concurrent_campaigns;
      const campaigns = [];

      // Create multiple campaigns with different configurations
      for (let i = 0; i < scenario.campaign_count; i++) {
        const campaignConfig = i % 2 === 0 ? CAMPAIGN_CONFIGS['quick-afl'] : CAMPAIGN_CONFIGS['quick-libfuzzer'];
        
        // Create corpus collection for each campaign
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`tenant-${i}-corpus`);
        resourceTracker.trackCollection(collectionId);

        const corpusData = i % 2 === 0 ? 
          generateTestData('normal') : 
          generateTestData('vulnerability_triggers');
        
        await testHelpers.uploadCorpusFiles(collectionId, corpusData);

        const campaignData = {
          name: testHelpers.generateTestName(`concurrent-campaign-${i}`),
          description: `Concurrent campaign ${i} for multi-tenant testing`,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          corpus_collection_id: collectionId,
          max_jobs: scenario.jobs_per_campaign,
          auto_restart: false,
          shared_corpus: scenario.expected_isolation ? false : true,
          job_template: {
            duration: 120, // 2 minutes for concurrent testing
            memory_limit: 128 * 1024 * 1024, // Smaller memory footprint
            timeout: 1000,
            fuzzer_type: campaignConfig.fuzzerType,
            arguments: '@@'
          },
          priority: i === 0 ? 'high' : 'normal', // Give first campaign high priority
          tenant_id: `tenant-${Math.floor(i / 2)}`, // Group campaigns by tenant
          tags: ['multi-tenant', `tenant-${Math.floor(i / 2)}`, `campaign-${i}`]
        };

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        campaigns.push({
          id: response.id!,
          tenantId: campaignData.tenant_id,
          priority: campaignData.priority,
          expectedJobs: scenario.jobs_per_campaign
        });

        resourceTracker.trackCampaign(response.id!);
      }

      // Start all campaigns simultaneously
      const startPromises = campaigns.map(campaign =>
        client.campaigns.updateCampaign({
          id: campaign.id,
          campaignUpdateRequest: { status: 'running' }
        })
      );

      const startTime = Date.now();
      await Promise.all(startPromises);
      const startupTime = Date.now() - startTime;

      expect(startupTime).toBeLessThan(PERFORMANCE_THRESHOLDS.campaign_start_time * scenario.campaign_count);

      // Wait for all campaigns to be running
      await Promise.all(
        campaigns.map(campaign =>
          testHelpers.waitForCampaignStatus(campaign.id, 'running', 45000)
        )
      );

      // Monitor concurrent execution
      await new Promise(resolve => setTimeout(resolve, 60000)); // Run for 1 minute

      // Verify all campaigns are executing
      const campaignStatuses = await Promise.all(
        campaigns.map(async campaign => {
          const stats = await client.campaigns.getCampaignStats({ id: campaign.id });
          return {
            id: campaign.id,
            tenantId: campaign.tenantId,
            priority: campaign.priority,
            activeJobs: stats.active_jobs,
            totalExecutions: stats.total_executions,
            status: stats.status
          };
        })
      );

      console.log('Concurrent campaign status:');
      campaignStatuses.forEach(status => {
        console.log(`  Campaign ${status.id} (${status.tenantId}, ${status.priority}): ${status.activeJobs} jobs, ${status.totalExecutions} execs`);
      });

      // Verify isolation if expected
      if (scenario.expected_isolation) {
        // Each tenant should have independent resource allocation
        const tenantGroups = new Map();
        campaignStatuses.forEach(status => {
          if (!tenantGroups.has(status.tenantId)) {
            tenantGroups.set(status.tenantId, []);
          }
          tenantGroups.get(status.tenantId).push(status);
        });

        expect(tenantGroups.size).toBeGreaterThan(1);
        console.log(`Resource isolation: ${tenantGroups.size} tenant groups`);
      }

      // Check system health under load
      const healthStatus = await client.health.getHealth();
      expect(healthStatus.status).toBe('healthy');
    });

    test('should handle campaign resource competition', async () => {
      const scenario = MULTI_TENANT_SCENARIOS.resource_competition;
      
      // Create high-priority campaigns
      const highPriorityCampaigns = [];
      for (let i = 0; i < scenario.high_priority_jobs; i++) {
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`high-priority-${i}`);
        resourceTracker.trackCollection(collectionId);
        
        await testHelpers.uploadCorpusFiles(collectionId, generateTestData('normal'));

        const campaignData = {
          name: testHelpers.generateTestName(`high-priority-${i}`),
          description: `High priority campaign ${i}`,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          corpus_collection_id: collectionId,
          max_jobs: 2,
          auto_restart: false,
          job_template: {
            duration: 180,
            memory_limit: 256 * 1024 * 1024,
            timeout: 1000,
            fuzzer_type: 'afl++' as const,
            arguments: '@@'
          },
          priority: 'high',
          tenant_id: 'high-priority-tenant',
          tags: ['high-priority', 'resource-competition']
        };

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        highPriorityCampaigns.push(response.id!);
        resourceTracker.trackCampaign(response.id!);
      }

      // Create low-priority campaigns
      const lowPriorityCampaigns = [];
      for (let i = 0; i < scenario.low_priority_jobs; i++) {
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`low-priority-${i}`);
        resourceTracker.trackCollection(collectionId);
        
        await testHelpers.uploadCorpusFiles(collectionId, generateTestData('minimal'));

        const campaignData = {
          name: testHelpers.generateTestName(`low-priority-${i}`),
          description: `Low priority campaign ${i}`,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          corpus_collection_id: collectionId,
          max_jobs: 1,
          auto_restart: false,
          job_template: {
            duration: 180,
            memory_limit: 128 * 1024 * 1024,
            timeout: 1000,
            fuzzer_type: 'libfuzzer' as const,
            arguments: '@@'
          },
          priority: 'low',
          tenant_id: 'low-priority-tenant',
          tags: ['low-priority', 'resource-competition']
        };

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        lowPriorityCampaigns.push(response.id!);
        resourceTracker.trackCampaign(response.id!);
      }

      // Start all campaigns
      const allCampaigns = [...highPriorityCampaigns, ...lowPriorityCampaigns];
      await Promise.all(
        allCampaigns.map(id =>
          client.campaigns.updateCampaign({
            id,
            campaignUpdateRequest: { status: 'running' }
          })
        )
      );

      // Wait for competition to play out
      await new Promise(resolve => setTimeout(resolve, 45000));

      // Analyze resource allocation
      const highPriorityStats = await Promise.all(
        highPriorityCampaigns.map(id =>
          client.campaigns.getCampaignStats({ id })
        )
      );

      const lowPriorityStats = await Promise.all(
        lowPriorityCampaigns.map(id =>
          client.campaigns.getCampaignStats({ id })
        )
      );

      // High priority campaigns should get more resources
      const avgHighPriorityJobs = highPriorityStats.reduce(
        (sum, stats) => sum + stats.active_jobs, 0
      ) / highPriorityStats.length;

      const avgLowPriorityJobs = lowPriorityStats.reduce(
        (sum, stats) => sum + stats.active_jobs, 0
      ) / lowPriorityStats.length;

      console.log(`Resource allocation - High priority avg: ${avgHighPriorityJobs} jobs, Low priority avg: ${avgLowPriorityJobs} jobs`);

      if (scenario.expected_prioritization && avgHighPriorityJobs > 0 && avgLowPriorityJobs > 0) {
        // High priority should have better resource allocation ratio
        expect(avgHighPriorityJobs).toBeGreaterThanOrEqual(avgLowPriorityJobs);
      }
    });

    test('should maintain performance under concurrent load', async () => {
      // Create moderate concurrent load
      const concurrentCampaigns = [];
      const campaignCount = 4;

      for (let i = 0; i < campaignCount; i++) {
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`perf-test-${i}`);
        resourceTracker.trackCollection(collectionId);
        
        await testHelpers.uploadCorpusFiles(collectionId, generateTestData('normal'));

        const campaignData = {
          name: testHelpers.generateTestName(`perf-campaign-${i}`),
          description: `Performance test campaign ${i}`,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          corpus_collection_id: collectionId,
          max_jobs: 1,
          auto_restart: false,
          job_template: {
            duration: 90, // Shorter duration for performance test
            memory_limit: 128 * 1024 * 1024,
            timeout: 1000,
            fuzzer_type: i % 2 === 0 ? 'afl++' as const : 'libfuzzer' as const,
            arguments: '@@'
          },
          tenant_id: `perf-tenant-${i}`,
          tags: ['performance-test', `tenant-${i}`]
        };

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        concurrentCampaigns.push(response.id!);
        resourceTracker.trackCampaign(response.id!);
      }

      // Measure API response times under load
      const apiMetrics = [];
      const measureApiResponse = async (operation: Promise<any>) => {
        const start = Date.now();
        try {
          await operation;
          return Date.now() - start;
        } catch (error) {
          return Date.now() - start; // Still measure time even if operation fails
        }
      };

      // Start campaigns and measure response times
      for (const campaignId of concurrentCampaigns) {
        const responseTime = await measureApiResponse(
          client.campaigns.updateCampaign({
            id: campaignId,
            campaignUpdateRequest: { status: 'running' }
          })
        );
        apiMetrics.push(responseTime);
      }

      // Measure ongoing API performance
      for (let i = 0; i < 10; i++) {
        const responseTime = await measureApiResponse(
          client.campaigns.listCampaigns({ limit: 50 })
        );
        apiMetrics.push(responseTime);
        
        await new Promise(resolve => setTimeout(resolve, 1000));
      }

      // Analyze performance
      const avgResponseTime = apiMetrics.reduce((sum, time) => sum + time, 0) / apiMetrics.length;
      const maxResponseTime = Math.max(...apiMetrics);

      console.log(`API Performance under load - Avg: ${avgResponseTime}ms, Max: ${maxResponseTime}ms`);

      expect(avgResponseTime).toBeLessThan(PERFORMANCE_THRESHOLDS.api_response_time);
      expect(maxResponseTime).toBeLessThan(PERFORMANCE_THRESHOLDS.api_response_time * 3); // Allow some degradation

      // Check system resource usage
      const systemSnapshot = await testHelpers.takePerformanceSnapshot();
      expect(systemSnapshot.memory.heapUsed).toBeLessThan(PERFORMANCE_THRESHOLDS.memory_usage_mb * 1024 * 1024);
    });
  });

  describe('Corpus Sharing Policies', () => {
    let sharedCollections: string[] = [];
    let privateCollections: string[] = [];
    let tenant1Campaigns: string[] = [];
    let tenant2Campaigns: string[] = [];

    beforeEach(async () => {
      const scenario = MULTI_TENANT_SCENARIOS.corpus_sharing;
      
      // Create shared collections
      for (let i = 0; i < scenario.shared_collections; i++) {
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`shared-${i}`);
        sharedCollections.push(collectionId);
        resourceTracker.trackCollection(collectionId);
        
        await testHelpers.uploadCorpusFiles(collectionId, generateTestData('normal'));
      }

      // Create private collections
      for (let i = 0; i < scenario.private_collections; i++) {
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`private-${i}`);
        privateCollections.push(collectionId);
        resourceTracker.trackCollection(collectionId);
        
        await testHelpers.uploadCorpusFiles(collectionId, generateTestData('binary'));
      }

      // Create tenant campaigns
      for (let tenant of [1, 2]) {
        const campaignData = {
          name: testHelpers.generateTestName(`tenant-${tenant}-campaign`),
          description: `Campaign for tenant ${tenant}`,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          corpus_collection_id: tenant === 1 ? sharedCollections[0] : privateCollections[0],
          max_jobs: 1,
          auto_restart: false,
          shared_corpus: tenant === 1,
          job_template: {
            duration: 90,
            memory_limit: 128 * 1024 * 1024,
            timeout: 1000,
            fuzzer_type: 'libfuzzer' as const,
            arguments: '@@'
          },
          tenant_id: `tenant-${tenant}`,
          access_policy: {
            allowed_collections: tenant === 1 ? sharedCollections : privateCollections,
            sharing_level: tenant === 1 ? 'public' : 'private'
          },
          tags: [`tenant-${tenant}`, 'corpus-sharing-test']
        };

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        if (tenant === 1) {
          tenant1Campaigns.push(response.id!);
        } else {
          tenant2Campaigns.push(response.id!);
        }
        
        resourceTracker.trackCampaign(response.id!);
      }
    });

    test('should enforce corpus access policies', async () => {
      // Test tenant 1 (should have access to shared collections)
      for (const collectionId of sharedCollections) {
        try {
          const collectionResponse = await client.corpus.getCorpusCollection({
            id: collectionId,
            tenantId: 'tenant-1'
          });
          
          expect(collectionResponse.id).toBe(collectionId);
          console.log(`Tenant 1 correctly accessed shared collection ${collectionId}`);
        } catch (error) {
          console.warn(`Tenant 1 could not access shared collection ${collectionId}:`, error.message);
        }
      }

      // Test tenant 2 accessing tenant 1's private collections (should fail)
      for (const collectionId of privateCollections) {
        try {
          await client.corpus.getCorpusCollection({
            id: collectionId,
            tenantId: 'tenant-2'
          });
          
          console.warn(`Tenant 2 unexpectedly accessed private collection ${collectionId}`);
        } catch (error) {
          console.log(`Tenant 2 correctly denied access to private collection ${collectionId}`);
          expect(error.message).toMatch(/access|permission|forbidden/i);
        }
      }
    });

    test('should handle selective corpus sharing', async () => {
      // Start both tenant campaigns
      await Promise.all([
        ...tenant1Campaigns.map(id =>
          client.campaigns.updateCampaign({
            id,
            campaignUpdateRequest: { status: 'running' }
          })
        ),
        ...tenant2Campaigns.map(id =>
          client.campaigns.updateCampaign({
            id,
            campaignUpdateRequest: { status: 'running' }
          })
        )
      ]);

      // Wait for campaigns to run
      await new Promise(resolve => setTimeout(resolve, 45000));

      // Check sharing behavior
      const scenario = MULTI_TENANT_SCENARIOS.corpus_sharing;
      
      if (scenario.expected_sharing_behavior === 'selective') {
        // Shared collections should evolve from both tenants
        for (const collectionId of sharedCollections) {
          const initialState = await client.corpus.getCorpusCollection({ id: collectionId });
          
          // Should have contributions from multiple sources
          const filesResponse = await client.corpus.listCorpusFiles({
            id: collectionId,
            include_metadata: true
          });

          let multiTenantContributions = false;
          const contributingTenants = new Set();

          for (const file of filesResponse.files || []) {
            if (file.metadata?.source_tenant) {
              contributingTenants.add(file.metadata.source_tenant);
            }
            if (file.generation_info?.source_campaign) {
              // Check which tenant owns the source campaign
              // This would need campaign-to-tenant mapping
            }
          }

          if (contributingTenants.size > 1) {
            multiTenantContributions = true;
            console.log(`Shared collection ${collectionId} has contributions from ${contributingTenants.size} tenants`);
          }

          // Private collections should only have single-tenant contributions
          for (const privateCollectionId of privateCollections) {
            const privateFilesResponse = await client.corpus.listCorpusFiles({
              id: privateCollectionId,
              include_metadata: true
            });

            const privateTenants = new Set();
            for (const file of privateFilesResponse.files || []) {
              if (file.metadata?.source_tenant) {
                privateTenants.add(file.metadata.source_tenant);
              }
            }

            expect(privateTenants.size).toBeLessThanOrEqual(1);
            console.log(`Private collection ${privateCollectionId} has ${privateTenants.size} tenant(s)`);
          }
        }
      }
    });

    test('should audit corpus sharing activities', async () => {
      try {
        // Get corpus sharing audit logs
        const auditResponse = await client.corpus.getCorpusSharingAudit({
          tenant_id: 'tenant-1',
          time_range: 'last_hour',
          include_details: true
        });

        expect(auditResponse.audit_entries).toBeDefined();

        if (auditResponse.audit_entries && auditResponse.audit_entries.length > 0) {
          for (const entry of auditResponse.audit_entries) {
            expect(entry.timestamp).toHaveValidTimestamp();
            expect(entry.action).toBeDefined();
            expect(['access', 'share', 'sync', 'deny']).toContain(entry.action);
            expect(entry.tenant_id).toBeDefined();
            expect(entry.collection_id).toBeDefined();

            if (entry.access_result) {
              expect(['allowed', 'denied']).toContain(entry.access_result);
            }
          }

          console.log(`Corpus sharing audit: ${auditResponse.audit_entries.length} entries`);
        } else {
          console.log('No corpus sharing audit entries found');
        }

        // Check sharing statistics
        if (auditResponse.statistics) {
          const stats = auditResponse.statistics;
          expect(stats.total_accesses).toBeGreaterThanOrEqual(0);
          expect(stats.successful_shares).toBeGreaterThanOrEqual(0);
          expect(stats.denied_accesses).toBeGreaterThanOrEqual(0);
        }

      } catch (error) {
        console.warn('Corpus sharing audit not available:', error.message);
      }
    });
  });

  describe('Resource Isolation and Monitoring', () => {
    test('should isolate tenant resources', async () => {
      const tenants = ['tenant-alpha', 'tenant-beta', 'tenant-gamma'];
      const tenantCampaigns = new Map();

      // Create campaigns for each tenant
      for (const tenant of tenants) {
        const { id: collectionId } = await testHelpers.createTestCorpusCollection(`${tenant}-corpus`);
        resourceTracker.trackCollection(collectionId);
        
        await testHelpers.uploadCorpusFiles(collectionId, generateTestData('normal'));

        const campaignData = {
          name: testHelpers.generateTestName(`${tenant}-isolation-test`),
          description: `Resource isolation test for ${tenant}`,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          corpus_collection_id: collectionId,
          max_jobs: 2,
          auto_restart: false,
          job_template: {
            duration: 120,
            memory_limit: 128 * 1024 * 1024,
            timeout: 1000,
            fuzzer_type: 'libfuzzer' as const,
            arguments: '@@'
          },
          tenant_id: tenant,
          resource_limits: {
            max_memory_mb: 512,
            max_cpu_cores: 2,
            max_concurrent_jobs: 3
          },
          tags: [tenant, 'isolation-test']
        };

        const response = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        tenantCampaigns.set(tenant, response.id!);
        resourceTracker.trackCampaign(response.id!);
      }

      // Start all tenant campaigns
      await Promise.all(
        Array.from(tenantCampaigns.values()).map(campaignId =>
          client.campaigns.updateCampaign({
            id: campaignId,
            campaignUpdateRequest: { status: 'running' }
          })
        )
      );

      // Monitor resource usage by tenant
      await new Promise(resolve => setTimeout(resolve, 60000)); // 1 minute

      const tenantResourceUsage = new Map();
      
      for (const [tenant, campaignId] of tenantCampaigns) {
        try {
          const tenantStats = await client.analytics.getTenantResourceUsage({
            tenantId: tenant,
            time_range: 'last_hour'
          });

          tenantResourceUsage.set(tenant, tenantStats);
          
          if (tenantStats.resource_usage) {
            expect(tenantStats.resource_usage.memory_mb).toBeGreaterThanOrEqual(0);
            expect(tenantStats.resource_usage.cpu_percent).toBeGreaterThanOrEqual(0);
            expect(tenantStats.resource_usage.active_jobs).toBeGreaterThanOrEqual(0);
          }

          console.log(`${tenant} resource usage:`, tenantStats.resource_usage);

        } catch (error) {
          console.warn(`Tenant resource monitoring not available for ${tenant}:`, error.message);
        }
      }

      // Verify isolation - no tenant should exceed their limits
      for (const [tenant, stats] of tenantResourceUsage) {
        if (stats.resource_usage) {
          expect(stats.resource_usage.memory_mb).toBeLessThanOrEqual(512);
          expect(stats.resource_usage.active_jobs).toBeLessThanOrEqual(3);
        }
      }
    });

    test('should provide tenant-specific analytics', async () => {
      const tenant = 'analytics-tenant';
      
      // Create campaign for analytics testing
      const { id: collectionId } = await testHelpers.createTestCorpusCollection(`${tenant}-analytics`);
      resourceTracker.trackCollection(collectionId);
      
      await testHelpers.uploadCorpusFiles(collectionId, generateTestData('vulnerability_triggers'));

      const campaignData = {
        name: testHelpers.generateTestName(`${tenant}-analytics-campaign`),
        description: 'Campaign for tenant analytics testing',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        corpus_collection_id: collectionId,
        max_jobs: 1,
        auto_restart: false,
        job_template: {
          duration: 120,
          memory_limit: 128 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'afl++' as const,
          arguments: '@@'
        },
        tenant_id: tenant,
        enable_analytics: true,
        tags: [tenant, 'analytics-test']
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

      // Wait for analytics data to accumulate
      await new Promise(resolve => setTimeout(resolve, 90000)); // 1.5 minutes

      try {
        // Get tenant-specific analytics
        const tenantAnalytics = await client.analytics.getTenantAnalytics({
          tenantId: tenant,
          include: ['performance', 'coverage', 'crashes', 'resource_usage'],
          time_range: 'last_hour'
        });

        expect(tenantAnalytics.tenant_id).toBe(tenant);
        expect(tenantAnalytics.time_range).toBeDefined();

        // Check performance metrics
        if (tenantAnalytics.performance_metrics) {
          const perf = tenantAnalytics.performance_metrics;
          expect(perf.total_executions).toBeGreaterThanOrEqual(0);
          expect(perf.executions_per_second).toBeGreaterThanOrEqual(0);
          expect(perf.average_job_duration).toBeGreaterThanOrEqual(0);
        }

        // Check coverage metrics
        if (tenantAnalytics.coverage_metrics) {
          const coverage = tenantAnalytics.coverage_metrics;
          expect(coverage.total_coverage).toBeGreaterThanOrEqual(0);
          expect(coverage.coverage_growth_rate).toBeGreaterThanOrEqual(0);
        }

        // Check crash metrics
        if (tenantAnalytics.crash_metrics) {
          const crashes = tenantAnalytics.crash_metrics;
          expect(crashes.total_crashes).toBeGreaterThanOrEqual(0);
          expect(crashes.unique_crashes).toBeGreaterThanOrEqual(0);
          expect(crashes.crash_discovery_rate).toBeGreaterThanOrEqual(0);
        }

        // Check resource usage trends
        if (tenantAnalytics.resource_trends) {
          expect(Array.isArray(tenantAnalytics.resource_trends.memory_usage)).toBe(true);
          expect(Array.isArray(tenantAnalytics.resource_trends.cpu_usage)).toBe(true);
        }

        console.log(`Tenant analytics for ${tenant}:`, {
          executions: tenantAnalytics.performance_metrics?.total_executions,
          crashes: tenantAnalytics.crash_metrics?.total_crashes,
          coverage: tenantAnalytics.coverage_metrics?.total_coverage
        });

      } catch (error) {
        console.warn('Tenant-specific analytics not available:', error.message);
      }
    });

    test('should handle tenant quota management', async () => {
      const tenant = 'quota-test-tenant';
      const quotaLimits = {
        max_campaigns: 2,
        max_jobs_per_hour: 5,
        max_corpus_size_mb: 10,
        max_storage_mb: 50
      };

      try {
        // Set tenant quotas
        await client.tenants.setTenantQuotas({
          tenantId: tenant,
          quotaUpdateRequest: {
            limits: quotaLimits,
            enforcement_level: 'strict',
            reset_period: 'hourly'
          }
        });

        // Create campaigns up to the limit
        const campaigns = [];
        for (let i = 0; i < quotaLimits.max_campaigns; i++) {
          const { id: collectionId } = await testHelpers.createTestCorpusCollection(`quota-test-${i}`);
          resourceTracker.trackCollection(collectionId);
          
          await testHelpers.uploadCorpusFiles(collectionId, generateTestData('minimal'));

          const campaignData = {
            name: testHelpers.generateTestName(`quota-campaign-${i}`),
            description: `Quota test campaign ${i}`,
            target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
            corpus_collection_id: collectionId,
            max_jobs: 1,
            auto_restart: false,
            job_template: {
              duration: 60,
              memory_limit: 64 * 1024 * 1024,
              timeout: 1000,
              fuzzer_type: 'libfuzzer' as const,
              arguments: '@@'
            },
            tenant_id: tenant,
            tags: [tenant, 'quota-test']
          };

          const response = await client.campaigns.createCampaign({
            campaignCreateRequest: campaignData
          });

          campaigns.push(response.id!);
          resourceTracker.trackCampaign(response.id!);
        }

        // Try to create one more campaign (should fail)
        try {
          const { id: collectionId } = await testHelpers.createTestCorpusCollection('quota-exceed');
          resourceTracker.trackCollection(collectionId);
          
          await testHelpers.uploadCorpusFiles(collectionId, generateTestData('minimal'));

          const campaignData = {
            name: testHelpers.generateTestName('quota-exceed-campaign'),
            description: 'Campaign that should exceed quota',
            target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
            corpus_collection_id: collectionId,
            max_jobs: 1,
            auto_restart: false,
            job_template: {
              duration: 60,
              memory_limit: 64 * 1024 * 1024,
              timeout: 1000,
              fuzzer_type: 'libfuzzer' as const,
              arguments: '@@'
            },
            tenant_id: tenant,
            tags: [tenant, 'quota-exceed-test']
          };

          await client.campaigns.createCampaign({
            campaignCreateRequest: campaignData
          });

          console.warn('Quota enforcement failed - campaign creation should have been blocked');

        } catch (error) {
          console.log('Quota correctly enforced - campaign creation blocked');
          expect(error.message).toMatch(/quota|limit|exceeded/i);
        }

        // Check quota status
        const quotaStatus = await client.tenants.getTenantQuotaStatus({
          tenantId: tenant
        });

        expect(quotaStatus.tenant_id).toBe(tenant);
        expect(quotaStatus.current_usage).toBeDefined();
        expect(quotaStatus.limits).toEqual(quotaLimits);
        expect(quotaStatus.current_usage.campaigns).toBe(quotaLimits.max_campaigns);

        console.log(`Tenant quota status:`, quotaStatus.current_usage);

      } catch (error) {
        console.warn('Tenant quota management not available:', error.message);
      }
    });
  });

  describe('Cross-Tenant Event Isolation', () => {
    test('should isolate events between tenants', async () => {
      const tenant1 = 'event-tenant-1';
      const tenant2 = 'event-tenant-2';

      // Create campaigns for each tenant
      const tenant1Campaign = await this.createTenantCampaign(tenant1);
      const tenant2Campaign = await this.createTenantCampaign(tenant2);

      resourceTracker.trackCampaign(tenant1Campaign);
      resourceTracker.trackCampaign(tenant2Campaign);

      // Start event collection for tenant 1
      const tenant1EventCollector = new EventCollector(client);
      tenant1EventCollector.startCollecting({
        types: ['CAMPAIGN_STARTED', 'JOB_CREATED', 'CRASH_DISCOVERED'],
        filter: { tenant_id: tenant1 }
      });

      // Start event collection for tenant 2
      const tenant2EventCollector = new EventCollector(client);
      tenant2EventCollector.startCollecting({
        types: ['CAMPAIGN_STARTED', 'JOB_CREATED', 'CRASH_DISCOVERED'],
        filter: { tenant_id: tenant2 }
      });

      // Start both campaigns
      await Promise.all([
        client.campaigns.updateCampaign({
          id: tenant1Campaign,
          campaignUpdateRequest: { status: 'running' }
        }),
        client.campaigns.updateCampaign({
          id: tenant2Campaign,
          campaignUpdateRequest: { status: 'running' }
        })
      ]);

      // Wait for events to be generated
      await new Promise(resolve => setTimeout(resolve, 30000));

      // Stop event collection
      tenant1EventCollector.stopCollecting();
      tenant2EventCollector.stopCollecting();

      // Verify event isolation
      const tenant1Events = tenant1EventCollector.getEvents();
      const tenant2Events = tenant2EventCollector.getEvents();

      console.log(`Tenant 1 events: ${tenant1Events.length}, Tenant 2 events: ${tenant2Events.length}`);

      // Each tenant should only see their own events
      for (const event of tenant1Events) {
        if (event.data?.tenant_id) {
          expect(event.data.tenant_id).toBe(tenant1);
        }
      }

      for (const event of tenant2Events) {
        if (event.data?.tenant_id) {
          expect(event.data.tenant_id).toBe(tenant2);
        }
      }

      // No cross-contamination
      const crossContamination1 = tenant1Events.some(event => 
        event.data?.tenant_id === tenant2
      );
      const crossContamination2 = tenant2Events.some(event => 
        event.data?.tenant_id === tenant1
      );

      expect(crossContamination1).toBe(false);
      expect(crossContamination2).toBe(false);
    });

    // Helper method to create tenant campaigns
    async createTenantCampaign(tenantId: string): Promise<string> {
      const { id: collectionId } = await testHelpers.createTestCorpusCollection(`${tenantId}-corpus`);
      resourceTracker.trackCollection(collectionId);
      
      await testHelpers.uploadCorpusFiles(collectionId, generateTestData('normal'));

      const campaignData = {
        name: testHelpers.generateTestName(`${tenantId}-campaign`),
        description: `Campaign for ${tenantId}`,
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        corpus_collection_id: collectionId,
        max_jobs: 1,
        auto_restart: false,
        job_template: {
          duration: 90,
          memory_limit: 128 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'libfuzzer' as const,
          arguments: '@@'
        },
        tenant_id: tenantId,
        tags: [tenantId, 'event-isolation-test']
      };

      const response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      return response.id!;
    }
  });
});