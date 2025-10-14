/**
 * Basic CRUD Operations Example
 * 
 * This example demonstrates basic Create, Read, Update, Delete operations
 * using the PandaFuzz TypeScript client SDK.
 */

import { PandaFuzzClient, BotStatus, CampaignStatus, JobStatus } from '@pandafuzz/client';

async function basicOperationsExample() {
  // Initialize the client
  const client = new PandaFuzzClient({
    baseUrl: 'http://localhost:8080/api/v1',
    apiKey: 'your-api-key-here'
  });

  try {
    console.log('🐼 Starting PandaFuzz Basic Operations Example\n');

    // 1. Health Check
    console.log('1. Checking system health...');
    const healthStatus = await client.health.getHealth();
    console.log('Health status:', healthStatus.status);
    console.log('Uptime:', healthStatus.uptime);
    console.log();

    // 2. Bot Operations
    console.log('2. Bot Management Operations');
    
    // List all bots
    const botsList = await client.bots.listBots();
    console.log(`Found ${botsList.data?.length || 0} bots`);
    
    // Register a new bot (if none exist)
    if (!botsList.data || botsList.data.length === 0) {
      console.log('Registering a new bot...');
      const newBot = await client.bots.createBot({
        name: 'example-bot',
        capabilities: ['libfuzzer', 'afl++'],
        maxConcurrentJobs: 2,
        metadata: {
          environment: 'example',
          location: 'local'
        }
      });
      console.log('Bot registered:', newBot.id);
    }

    // Get the first bot for further operations
    const updatedBotsList = await client.bots.listBots();
    const firstBot = updatedBotsList.data?.[0];
    
    if (firstBot) {
      console.log(`Using bot: ${firstBot.name} (${firstBot.id})`);
      
      // Update bot information
      await client.bots.updateBot({
        id: firstBot.id!,
        botUpdateRequest: {
          name: `${firstBot.name}-updated`,
          maxConcurrentJobs: 3
        }
      });
      console.log('Bot updated successfully');
    }
    console.log();

    // 3. Campaign Operations
    console.log('3. Campaign Management Operations');
    
    // Create a new campaign
    const campaign = await client.campaigns.createCampaign({
      name: 'Example Security Campaign',
      description: 'Comprehensive security testing campaign',
      jobTemplate: {
        fuzzerType: 'libfuzzer',
        targetBinary: '/usr/bin/example-target',
        duration: 3600, // 1 hour
        corpusPath: '/tmp/corpus',
        outputPath: '/tmp/output',
        metadata: {
          priority: 'high',
          tags: ['security', 'example']
        }
      }
    });
    console.log('Campaign created:', campaign.id);

    // Get campaign details
    const campaignDetails = await client.campaigns.getCampaign({ 
      id: campaign.id! 
    });
    console.log('Campaign status:', campaignDetails.status);

    // Start the campaign
    await client.campaigns.startCampaign({ id: campaign.id! });
    console.log('Campaign started successfully');

    // Get campaign statistics
    const stats = await client.campaigns.getCampaignStats({ 
      id: campaign.id! 
    });
    console.log('Campaign stats:', {
      totalJobs: stats.totalJobs,
      completedJobs: stats.completedJobs,
      activeBots: stats.activeBots
    });
    console.log();

    // 4. Job Operations
    console.log('4. Job Management Operations');
    
    // List jobs for the campaign
    const jobs = await client.jobs.listJobs({
      campaignId: campaign.id,
      limit: 10
    });
    console.log(`Found ${jobs.data?.length || 0} jobs for campaign`);

    // Create a standalone job
    const job = await client.jobs.createJob({
      name: 'Example Fuzzing Job',
      campaignId: campaign.id,
      fuzzerType: 'libfuzzer',
      targetBinary: '/usr/bin/test-target',
      duration: 1800, // 30 minutes
      arguments: ['-max_len=1024', '-timeout=30'],
      environment: {
        ASAN_OPTIONS: 'detect_leaks=1',
        MSAN_OPTIONS: 'halt_on_error=1'
      }
    });
    console.log('Job created:', job.id);

    // Get job details
    const jobDetails = await client.jobs.getJob({ id: job.id! });
    console.log('Job status:', jobDetails.status);
    console.log('Job progress:', jobDetails.progress);
    console.log();

    // 5. Corpus Operations
    console.log('5. Corpus Management Operations');
    
    // List corpus entries
    const corpus = await client.corpus.listCorpus({
      campaignId: campaign.id,
      limit: 20
    });
    console.log(`Found ${corpus.data?.length || 0} corpus entries`);

    // Sync corpus (example of corpus synchronization)
    const syncResult = await client.corpus.syncCorpus({
      campaignId: campaign.id!,
      corpusSyncRequest: {
        strategy: 'merge',
        filters: {
          minSize: 1,
          maxSize: 10240, // 10KB max
          excludePatterns: ['*.tmp', '*.log']
        }
      }
    });
    console.log('Corpus sync result:', syncResult.summary);
    console.log();

    // 6. Crash Operations  
    console.log('6. Crash Analysis Operations');
    
    // List crashes
    const crashes = await client.crashes.listCrashes({
      campaignId: campaign.id,
      severity: 'high',
      limit: 10
    });
    console.log(`Found ${crashes.data?.length || 0} high-severity crashes`);

    if (crashes.data && crashes.data.length > 0) {
      const firstCrash = crashes.data[0];
      
      // Get crash details
      const crashDetails = await client.crashes.getCrash({ 
        id: firstCrash.id! 
      });
      console.log('Crash details:', {
        type: crashDetails.type,
        severity: crashDetails.severity,
        reproduced: crashDetails.reproductionInfo?.reproduced
      });
    }
    console.log();

    // 7. Analytics Operations
    console.log('7. Analytics and Metrics');
    
    // Get system analytics
    const analytics = await client.analytics.getAnalytics({
      timeRange: '24h',
      includeComponents: ['bots', 'jobs', 'crashes']
    });
    console.log('System overview:', analytics.systemOverview);
    console.log('Performance metrics:', analytics.performanceMetrics);

    // Get system metrics
    const metrics = await client.analytics.getMetrics();
    console.log('Current metrics:', {
      activeBots: metrics.metrics?.bots?.active,
      runningJobs: metrics.metrics?.jobs?.running,
      totalCrashes: metrics.metrics?.crashes?.total
    });
    console.log();

    // 8. Cleanup (Optional)
    console.log('8. Cleanup Operations');
    
    // Stop the campaign
    await client.campaigns.stopCampaign({
      id: campaign.id!,
      stopCampaignRequest: {
        reason: 'Example completed'
      }
    });
    console.log('Campaign stopped');

    console.log('\n✅ Basic operations example completed successfully!');

  } catch (error) {
    console.error('❌ Error in basic operations example:', error);
    
    // Enhanced error handling
    if (error instanceof Error) {
      console.error('Error message:', error.message);
    }
    
    // Check for HTTP errors
    if (typeof error === 'object' && error !== null && 'status' in error) {
      console.error('HTTP Status:', (error as any).status);
      console.error('Response body:', (error as any).body);
    }
  }
}

// Helper function to demonstrate retry logic
async function robustOperationExample() {
  const client = new PandaFuzzClient({
    baseUrl: 'http://localhost:8080/api/v1',
    apiKey: 'your-api-key-here'
  });

  console.log('🔄 Demonstrating robust operations with retry logic\n');

  try {
    // Use automatic retry for potentially flaky operations
    const healthCheck = await client.withRetry(
      () => client.health.getHealth(),
      {
        maxAttempts: 3,
        initialDelay: 1000,
        maxDelay: 5000,
        backoffFactor: 2
      }
    );
    console.log('Health check with retry:', healthCheck.status);

    // Wait for system to be ready
    await client.waitForReady(10000, 1000);
    console.log('System is ready!');

  } catch (error) {
    console.error('Retry example failed:', error);
  }
}

// Run the examples
if (require.main === module) {
  (async () => {
    await basicOperationsExample();
    console.log('\n' + '='.repeat(50) + '\n');
    await robustOperationExample();
  })();
}

export { basicOperationsExample, robustOperationExample };