/**
 * Campaign Management Example
 * 
 * This example demonstrates comprehensive campaign management workflows
 * including creation, monitoring, and optimization of fuzzing campaigns.
 */

import { 
  PandaFuzzClient, 
  Campaign, 
  CampaignCreateRequest,
  CampaignStats,
  JobCreateRequest,
  EventTypes 
} from '@pandafuzz/client';

class CampaignManager {
  private client: PandaFuzzClient;
  private activeCampaigns: Map<string, Campaign> = new Map();

  constructor(baseUrl: string = 'http://localhost:8080/api/v1', apiKey?: string) {
    this.client = new PandaFuzzClient({
      baseUrl,
      apiKey,
      sse: {
        debug: false,
        reconnectInterval: 5000
      }
    });

    this.setupEventMonitoring();
  }

  /**
   * Set up event monitoring for campaign-related events
   */
  private setupEventMonitoring(): void {
    // Monitor campaign events
    this.client.sse.on(EventTypes.CAMPAIGN_STARTED, (event) => {
      console.log(`🚀 Campaign started: ${event.data.name} (${event.data.campaignId})`);
    });

    this.client.sse.on(EventTypes.CAMPAIGN_COMPLETED, (event) => {
      console.log(`🏁 Campaign completed: ${event.data.campaignId}`);
      console.log(`   Duration: ${event.data.duration}s, Crashes: ${event.data.totalCrashes}`);
    });

    this.client.sse.on(EventTypes.CAMPAIGN_FAILED, (event) => {
      console.log(`❌ Campaign failed: ${event.data.campaignId} - ${event.data.reason}`);
    });

    // Monitor job events within campaigns
    this.client.sse.on(EventTypes.JOB_COMPLETED, (event) => {
      console.log(`✅ Job completed in campaign: ${event.data.campaignId} (${event.data.jobId})`);
    });

    this.client.sse.on(EventTypes.CRASH_DISCOVERED, (event) => {
      console.log(`🐛 Crash discovered in campaign: ${event.data.campaignId} (Severity: ${event.data.severity})`);
    });

    // Connect to event stream
    this.client.connectEvents();
  }

  /**
   * Create a comprehensive security testing campaign
   */
  async createSecurityCampaign(
    name: string,
    targetBinary: string,
    description?: string
  ): Promise<Campaign> {
    console.log(`📋 Creating security campaign: ${name}`);

    const campaignRequest: CampaignCreateRequest = {
      name,
      description: description || `Comprehensive security testing for ${targetBinary}`,
      jobTemplate: {
        fuzzerType: 'libfuzzer',
        targetBinary,
        duration: 3600, // 1 hour per job
        arguments: [
          '-max_len=4096',
          '-timeout=30',
          '-rss_limit_mb=2048',
          '-malloc_limit_mb=2048',
          '-detect_leaks=1'
        ],
        environment: {
          ASAN_OPTIONS: 'detect_leaks=1:abort_on_error=1:detect_stack_use_after_return=1',
          UBSAN_OPTIONS: 'halt_on_error=1:abort_on_error=1',
          MSAN_OPTIONS: 'halt_on_error=1:abort_on_error=1'
        },
        corpusPath: `/tmp/corpus/${name.toLowerCase().replace(/\s+/g, '-')}`,
        outputPath: `/tmp/output/${name.toLowerCase().replace(/\s+/g, '-')}`,
        metadata: {
          priority: 'high',
          tags: ['security', 'automated'],
          target_type: 'binary',
          testing_phase: 'comprehensive'
        }
      }
    };

    try {
      const campaign = await this.client.campaigns.createCampaign(campaignRequest);
      this.activeCampaigns.set(campaign.id!, campaign);
      
      console.log(`✅ Campaign created: ${campaign.id}`);
      console.log(`   Name: ${campaign.name}`);
      console.log(`   Status: ${campaign.status}`);
      
      return campaign;
    } catch (error) {
      console.error(`❌ Failed to create campaign: ${error}`);
      throw error;
    }
  }

  /**
   * Create a performance testing campaign
   */
  async createPerformanceCampaign(
    name: string,
    targetBinary: string,
    maxJobs: number = 10
  ): Promise<Campaign> {
    console.log(`⚡ Creating performance campaign: ${name}`);

    const campaignRequest: CampaignCreateRequest = {
      name,
      description: `Performance and stress testing for ${targetBinary}`,
      jobTemplate: {
        fuzzerType: 'afl++',
        targetBinary,
        duration: 7200, // 2 hours per job
        arguments: [
          '-m', '8192', // 8GB memory limit
          '-t', '5000', // 5 second timeout
        ],
        environment: {
          AFL_SKIP_CPUFREQ: '1',
          AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES: '1',
          AFL_FAST_CAL: '1'
        },
        corpusPath: `/tmp/corpus/perf-${name.toLowerCase().replace(/\s+/g, '-')}`,
        outputPath: `/tmp/output/perf-${name.toLowerCase().replace(/\s+/g, '-')}`,
        metadata: {
          priority: 'medium',
          tags: ['performance', 'stress-test'],
          max_jobs: maxJobs.toString(),
          target_type: 'binary'
        }
      }
    };

    try {
      const campaign = await this.client.campaigns.createCampaign(campaignRequest);
      this.activeCampaigns.set(campaign.id!, campaign);
      
      console.log(`✅ Performance campaign created: ${campaign.id}`);
      return campaign;
    } catch (error) {
      console.error(`❌ Failed to create performance campaign: ${error}`);
      throw error;
    }
  }

  /**
   * Start a campaign with pre-flight checks
   */
  async startCampaign(campaignId: string): Promise<void> {
    console.log(`🚀 Starting campaign: ${campaignId}`);

    try {
      // Pre-flight health check
      const isHealthy = await this.client.healthCheck();
      if (!isHealthy) {
        throw new Error('System is not healthy - cannot start campaign');
      }

      // Check available bots
      const bots = await this.client.bots.listBots();
      const availableBots = bots.data?.filter(bot => bot.status === 'idle') || [];
      
      if (availableBots.length === 0) {
        console.warn('⚠️ No idle bots available - campaign may queue');
      } else {
        console.log(`✅ ${availableBots.length} bots available for campaign`);
      }

      // Start the campaign
      await this.client.campaigns.startCampaign({ id: campaignId });
      
      console.log(`✅ Campaign started successfully: ${campaignId}`);

      // Update local state
      const campaign = this.activeCampaigns.get(campaignId);
      if (campaign) {
        campaign.status = 'running';
      }

    } catch (error) {
      console.error(`❌ Failed to start campaign: ${error}`);
      throw error;
    }
  }

  /**
   * Monitor campaign progress and provide insights
   */
  async monitorCampaign(campaignId: string): Promise<void> {
    console.log(`📊 Monitoring campaign: ${campaignId}`);

    try {
      const stats = await this.client.campaigns.getCampaignStats({ id: campaignId });
      
      console.log('Campaign Statistics:');
      console.log(`  Total Jobs: ${stats.totalJobs}`);
      console.log(`  Completed Jobs: ${stats.completedJobs}`);
      console.log(`  Running Jobs: ${stats.runningJobs}`);
      console.log(`  Failed Jobs: ${stats.failedJobs}`);
      console.log(`  Active Bots: ${stats.activeBots}`);
      console.log(`  Total Crashes: ${stats.totalCrashes}`);
      console.log(`  Unique Crashes: ${stats.uniqueCrashes}`);

      // Calculate derived metrics
      const completionRate = stats.totalJobs > 0 ? 
        (stats.completedJobs / stats.totalJobs * 100).toFixed(1) : '0';
      
      const failureRate = stats.totalJobs > 0 ? 
        (stats.failedJobs / stats.totalJobs * 100).toFixed(1) : '0';

      console.log(`  Completion Rate: ${completionRate}%`);
      console.log(`  Failure Rate: ${failureRate}%`);

      // Performance metrics
      if (stats.performanceMetrics) {
        console.log('Performance Metrics:');
        console.log(`  Avg Exec/sec: ${stats.performanceMetrics.averageExecPerSecond}`);
        console.log(`  Coverage: ${stats.performanceMetrics.totalCoverage}%`);
        console.log(`  Corpus Size: ${stats.performanceMetrics.corpusSize}`);
      }

      // Provide insights
      this.provideCampaignInsights(stats);

    } catch (error) {
      console.error(`❌ Failed to monitor campaign: ${error}`);
    }
  }

  /**
   * Provide intelligent insights about campaign performance
   */
  private provideCampaignInsights(stats: CampaignStats): void {
    console.log('\n🧠 Campaign Insights:');

    // Job failure analysis
    if (stats.failedJobs > 0) {
      const failureRate = (stats.failedJobs / stats.totalJobs) * 100;
      if (failureRate > 20) {
        console.log('⚠️ High job failure rate detected - check target binary and configuration');
      } else if (failureRate > 10) {
        console.log('⚠️ Moderate job failure rate - monitor for patterns');
      }
    }

    // Bot utilization analysis
    if (stats.activeBots < 2) {
      console.log('📈 Consider adding more bots to increase parallelization');
    }

    // Crash analysis
    if (stats.totalCrashes === 0 && stats.completedJobs > 5) {
      console.log('🔍 No crashes found yet - consider adjusting fuzzer parameters or corpus');
    } else if (stats.uniqueCrashes > 0) {
      const duplicateRate = ((stats.totalCrashes - stats.uniqueCrashes) / stats.totalCrashes) * 100;
      if (duplicateRate > 50) {
        console.log('🔄 High crash duplication rate - deduplication is working well');
      }
    }

    // Performance insights
    if (stats.performanceMetrics) {
      if (stats.performanceMetrics.averageExecPerSecond < 100) {
        console.log('🐌 Low execution rate - check target binary performance');
      }
      
      if (stats.performanceMetrics.totalCoverage < 30) {
        console.log('🎯 Low coverage - consider improving seed corpus');
      }
    }
  }

  /**
   * Optimize campaign based on current performance
   */
  async optimizeCampaign(campaignId: string): Promise<void> {
    console.log(`🔧 Optimizing campaign: ${campaignId}`);

    try {
      const stats = await this.client.campaigns.getCampaignStats({ id: campaignId });
      const campaign = await this.client.campaigns.getCampaign({ id: campaignId });

      // Optimization recommendations
      const optimizations: string[] = [];

      // Check if we need more jobs
      if (stats.runningJobs < 3 && stats.activeBots > stats.runningJobs) {
        optimizations.push('Add more parallel jobs');
        
        // Create additional jobs
        await this.createAdditionalJobs(campaignId, 2);
      }

      // Check corpus quality
      if (stats.performanceMetrics?.corpusSize < 10) {
        optimizations.push('Enhance seed corpus');
        await this.enhanceCorpus(campaignId);
      }

      // Check for stalled jobs
      const jobs = await this.client.jobs.listJobs({ campaignId, status: 'running' });
      for (const job of jobs.data || []) {
        if (job.progress && job.progress.percentage < 5) {
          optimizations.push(`Review job ${job.id} - appears stalled`);
        }
      }

      if (optimizations.length > 0) {
        console.log('🎯 Applied optimizations:');
        optimizations.forEach(opt => console.log(`  - ${opt}`));
      } else {
        console.log('✅ Campaign is already well-optimized');
      }

    } catch (error) {
      console.error(`❌ Failed to optimize campaign: ${error}`);
    }
  }

  /**
   * Create additional jobs for a campaign
   */
  private async createAdditionalJobs(campaignId: string, count: number): Promise<void> {
    console.log(`➕ Creating ${count} additional jobs for campaign: ${campaignId}`);

    const campaign = await this.client.campaigns.getCampaign({ id: campaignId });
    
    for (let i = 0; i < count; i++) {
      const jobRequest: JobCreateRequest = {
        name: `${campaign.name} - Additional Job ${i + 1}`,
        campaignId,
        fuzzerType: campaign.jobTemplate?.fuzzerType || 'libfuzzer',
        targetBinary: campaign.jobTemplate?.targetBinary || '/tmp/target',
        duration: campaign.jobTemplate?.duration || 3600,
        arguments: campaign.jobTemplate?.arguments || [],
        environment: campaign.jobTemplate?.environment || {}
      };

      try {
        const job = await this.client.jobs.createJob(jobRequest);
        console.log(`✅ Created additional job: ${job.id}`);
      } catch (error) {
        console.error(`❌ Failed to create additional job: ${error}`);
      }
    }
  }

  /**
   * Enhance corpus for a campaign
   */
  private async enhanceCorpus(campaignId: string): Promise<void> {
    console.log(`📚 Enhancing corpus for campaign: ${campaignId}`);

    try {
      // Sync corpus with improved filters
      const syncResult = await this.client.corpus.syncCorpus({
        campaignId,
        corpusSyncRequest: {
          strategy: 'smart_merge',
          filters: {
            minSize: 1,
            maxSize: 8192,
            uniquenessThreshold: 0.8,
            excludePatterns: ['*.log', '*.tmp', '*.bak']
          }
        }
      });

      console.log(`✅ Corpus enhanced: ${syncResult.summary?.syncedEntries} entries synced`);
    } catch (error) {
      console.error(`❌ Failed to enhance corpus: ${error}`);
    }
  }

  /**
   * Generate comprehensive campaign report
   */
  async generateCampaignReport(campaignId: string): Promise<void> {
    console.log(`📑 Generating report for campaign: ${campaignId}`);

    try {
      const [campaign, stats, jobs, crashes] = await Promise.all([
        this.client.campaigns.getCampaign({ id: campaignId }),
        this.client.campaigns.getCampaignStats({ id: campaignId }),
        this.client.jobs.listJobs({ campaignId }),
        this.client.crashes.listCrashes({ campaignId })
      ]);

      console.log('\n' + '='.repeat(60));
      console.log(`📊 CAMPAIGN REPORT: ${campaign.name}`);
      console.log('='.repeat(60));
      
      console.log(`\n📋 Basic Information:`);
      console.log(`  ID: ${campaign.id}`);
      console.log(`  Status: ${campaign.status}`);
      console.log(`  Created: ${campaign.createdAt}`);
      console.log(`  Description: ${campaign.description}`);

      console.log(`\n📈 Execution Statistics:`);
      console.log(`  Total Jobs: ${stats.totalJobs}`);
      console.log(`  Completed: ${stats.completedJobs}`);
      console.log(`  Running: ${stats.runningJobs}`);
      console.log(`  Failed: ${stats.failedJobs}`);
      console.log(`  Success Rate: ${((stats.completedJobs / stats.totalJobs) * 100).toFixed(1)}%`);

      console.log(`\n🐛 Security Findings:`);
      console.log(`  Total Crashes: ${stats.totalCrashes}`);
      console.log(`  Unique Crashes: ${stats.uniqueCrashes}`);
      console.log(`  High Severity: ${crashes.data?.filter(c => c.severity === 'high').length || 0}`);
      console.log(`  Medium Severity: ${crashes.data?.filter(c => c.severity === 'medium').length || 0}`);
      console.log(`  Low Severity: ${crashes.data?.filter(c => c.severity === 'low').length || 0}`);

      if (stats.performanceMetrics) {
        console.log(`\n⚡ Performance Metrics:`);
        console.log(`  Average Exec/sec: ${stats.performanceMetrics.averageExecPerSecond}`);
        console.log(`  Total Coverage: ${stats.performanceMetrics.totalCoverage}%`);
        console.log(`  Corpus Size: ${stats.performanceMetrics.corpusSize}`);
      }

      console.log(`\n🤖 Resource Utilization:`);
      console.log(`  Active Bots: ${stats.activeBots}`);
      console.log(`  Bot Efficiency: ${((stats.activeBots / Math.max(stats.runningJobs, 1)) * 100).toFixed(1)}%`);

      // Top crashes by severity
      if (crashes.data && crashes.data.length > 0) {
        console.log(`\n🔥 Top Security Issues:`);
        crashes.data
          .filter(crash => crash.severity === 'high')
          .slice(0, 5)
          .forEach((crash, index) => {
            console.log(`  ${index + 1}. ${crash.type} - ${crash.crashInfo?.signal || 'Unknown signal'}`);
          });
      }

      console.log('\n' + '='.repeat(60));

    } catch (error) {
      console.error(`❌ Failed to generate campaign report: ${error}`);
    }
  }

  /**
   * Stop and cleanup campaign
   */
  async stopCampaign(campaignId: string, reason: string = 'Manual stop'): Promise<void> {
    console.log(`⏹️ Stopping campaign: ${campaignId}`);

    try {
      await this.client.campaigns.stopCampaign({
        id: campaignId,
        stopCampaignRequest: { reason }
      });

      // Remove from active campaigns
      this.activeCampaigns.delete(campaignId);
      
      console.log(`✅ Campaign stopped: ${campaignId}`);
    } catch (error) {
      console.error(`❌ Failed to stop campaign: ${error}`);
      throw error;
    }
  }

  /**
   * Cleanup and close connections
   */
  async cleanup(): Promise<void> {
    console.log('🧹 Cleaning up campaign manager...');
    this.client.close();
  }
}

/**
 * Example: Complete campaign lifecycle
 */
async function campaignLifecycleExample() {
  console.log('🔄 Campaign Lifecycle Example\n');

  const manager = new CampaignManager();

  try {
    // Create a security testing campaign
    const campaign = await manager.createSecurityCampaign(
      'Web Server Security Test',
      '/usr/bin/nginx',
      'Comprehensive security testing of nginx web server'
    );

    // Start the campaign
    await manager.startCampaign(campaign.id!);

    // Monitor for 2 minutes
    console.log('📊 Monitoring campaign for 2 minutes...');
    const monitoringInterval = setInterval(async () => {
      await manager.monitorCampaign(campaign.id!);
    }, 30000);

    // Optimize after 1 minute
    setTimeout(async () => {
      await manager.optimizeCampaign(campaign.id!);
    }, 60000);

    // Generate report and stop after 2 minutes
    setTimeout(async () => {
      clearInterval(monitoringInterval);
      await manager.generateCampaignReport(campaign.id!);
      await manager.stopCampaign(campaign.id!, 'Example completed');
      await manager.cleanup();
    }, 120000);

  } catch (error) {
    console.error('Campaign lifecycle example failed:', error);
    await manager.cleanup();
  }
}

/**
 * Example: Multi-campaign management
 */
async function multiCampaignExample() {
  console.log('🎯 Multi-Campaign Management Example\n');

  const manager = new CampaignManager();

  try {
    // Create multiple campaigns
    const campaigns = await Promise.all([
      manager.createSecurityCampaign('Database Security', '/usr/bin/mysql'),
      manager.createPerformanceCampaign('API Performance', '/usr/bin/api-server', 5),
      manager.createSecurityCampaign('Crypto Library', '/usr/lib/libcrypto.so')
    ]);

    console.log(`✅ Created ${campaigns.length} campaigns`);

    // Start all campaigns
    for (const campaign of campaigns) {
      await manager.startCampaign(campaign.id!);
      await new Promise(resolve => setTimeout(resolve, 5000)); // Stagger starts
    }

    // Monitor all campaigns
    const monitorAll = async () => {
      for (const campaign of campaigns) {
        console.log(`\n--- ${campaign.name} ---`);
        await manager.monitorCampaign(campaign.id!);
      }
    };

    // Monitor every minute for 5 minutes
    const interval = setInterval(monitorAll, 60000);

    setTimeout(async () => {
      clearInterval(interval);
      
      // Generate reports for all campaigns
      for (const campaign of campaigns) {
        await manager.generateCampaignReport(campaign.id!);
        await manager.stopCampaign(campaign.id!, 'Multi-campaign example completed');
      }
      
      await manager.cleanup();
    }, 300000); // 5 minutes

  } catch (error) {
    console.error('Multi-campaign example failed:', error);
    await manager.cleanup();
  }
}

// CLI interface
const command = process.argv[2];

switch (command) {
  case 'lifecycle':
    campaignLifecycleExample();
    break;
  case 'multi':
    multiCampaignExample();
    break;
  default:
    console.log('Available commands:');
    console.log('  lifecycle - Complete campaign lifecycle example');
    console.log('  multi - Multi-campaign management example');
    console.log('\nUsage: node campaign-management.js <command>');
}

export { CampaignManager, campaignLifecycleExample, multiCampaignExample };