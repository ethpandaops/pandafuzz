/**
 * Real-time Event Monitoring Example
 * 
 * This example demonstrates how to use Server-Sent Events (SSE) for real-time
 * monitoring of PandaFuzz system events.
 */

import { 
  PandaFuzzClient, 
  SSEClient, 
  EventTypes, 
  SSEEventData,
  SSEEventFilter 
} from '@pandafuzz/client';

class EventMonitoringDashboard {
  private client: PandaFuzzClient;
  private eventCounts: Map<string, number> = new Map();
  private lastEventTime: Date = new Date();

  constructor(baseUrl: string = 'http://localhost:8080/api/v1', apiKey?: string) {
    this.client = new PandaFuzzClient({
      baseUrl,
      apiKey,
      sse: {
        debug: true,
        reconnectInterval: 3000,
        maxReconnectAttempts: 5
      }
    });

    this.setupEventHandlers();
  }

  /**
   * Set up comprehensive event handlers for all event types
   */
  private setupEventHandlers(): void {
    // Bot events
    this.client.sse.on(EventTypes.BOT_STATUS_CHANGED, (event) => {
      this.handleBotStatusChange(event);
    });

    this.client.sse.on(EventTypes.BOT_REGISTERED, (event) => {
      this.handleBotRegistered(event);
    });

    this.client.sse.on(EventTypes.BOT_UNREGISTERED, (event) => {
      this.handleBotUnregistered(event);
    });

    this.client.sse.on(EventTypes.BOT_HEARTBEAT, (event) => {
      this.handleBotHeartbeat(event);
    });

    // Job events
    this.client.sse.on(EventTypes.JOB_CREATED, (event) => {
      this.handleJobCreated(event);
    });

    this.client.sse.on(EventTypes.JOB_STARTED, (event) => {
      this.handleJobStarted(event);
    });

    this.client.sse.on(EventTypes.JOB_COMPLETED, (event) => {
      this.handleJobCompleted(event);
    });

    this.client.sse.on(EventTypes.JOB_FAILED, (event) => {
      this.handleJobFailed(event);
    });

    this.client.sse.on(EventTypes.JOB_PROGRESS, (event) => {
      this.handleJobProgress(event);
    });

    // Campaign events
    this.client.sse.on(EventTypes.CAMPAIGN_STARTED, (event) => {
      this.handleCampaignStarted(event);
    });

    this.client.sse.on(EventTypes.CAMPAIGN_STOPPED, (event) => {
      this.handleCampaignStopped(event);
    });

    this.client.sse.on(EventTypes.CAMPAIGN_COMPLETED, (event) => {
      this.handleCampaignCompleted(event);
    });

    // Crash events
    this.client.sse.on(EventTypes.CRASH_DISCOVERED, (event) => {
      this.handleCrashDiscovered(event);
    });

    this.client.sse.on(EventTypes.CRASH_MINIMIZED, (event) => {
      this.handleCrashMinimized(event);
    });

    this.client.sse.on(EventTypes.CRASH_REPRODUCED, (event) => {
      this.handleCrashReproduced(event);
    });

    // Corpus events
    this.client.sse.on(EventTypes.CORPUS_UPDATED, (event) => {
      this.handleCorpusUpdated(event);
    });

    this.client.sse.on(EventTypes.CORPUS_SYNCED, (event) => {
      this.handleCorpusSynced(event);
    });

    // System events
    this.client.sse.on(EventTypes.SYSTEM_ALERT, (event) => {
      this.handleSystemAlert(event);
    });

    this.client.sse.on(EventTypes.SYSTEM_STATUS, (event) => {
      this.handleSystemStatus(event);
    });

    // Connection events
    this.client.sse.onOpen(() => {
      console.log('🔗 Connected to PandaFuzz event stream');
    });

    this.client.sse.onClose(() => {
      console.log('❌ Disconnected from PandaFuzz event stream');
    });

    this.client.sse.onError((error) => {
      console.error('🚨 SSE Error:', error.message);
    });
  }

  /**
   * Start monitoring with optional event filtering
   */
  public startMonitoring(filter?: SSEEventFilter): void {
    console.log('🐼 Starting PandaFuzz Event Monitoring Dashboard\n');
    
    if (filter) {
      console.log('Applied filters:', filter);
    }

    this.client.connectEvents(filter);
    
    // Start periodic status reporting
    this.startStatusReporting();
  }

  /**
   * Stop monitoring and cleanup
   */
  public stopMonitoring(): void {
    console.log('\n🛑 Stopping event monitoring...');
    this.client.disconnectEvents();
    this.client.close();
  }

  /**
   * Start periodic status reporting
   */
  private startStatusReporting(): void {
    setInterval(() => {
      this.printEventSummary();
    }, 30000); // Every 30 seconds
  }

  /**
   * Print event summary statistics
   */
  private printEventSummary(): void {
    console.log('\n📊 Event Summary (Last 30 seconds):');
    console.log('─'.repeat(50));
    
    if (this.eventCounts.size === 0) {
      console.log('No events received');
    } else {
      for (const [eventType, count] of this.eventCounts.entries()) {
        console.log(`  ${eventType}: ${count} events`);
      }
    }
    
    console.log(`Last event: ${this.lastEventTime.toISOString()}`);
    console.log('─'.repeat(50));
    
    // Reset counters
    this.eventCounts.clear();
  }

  /**
   * Track event occurrence
   */
  private trackEvent(eventType: string, event: SSEEventData): void {
    this.eventCounts.set(eventType, (this.eventCounts.get(eventType) || 0) + 1);
    this.lastEventTime = new Date();
    
    console.log(`[${new Date().toISOString()}] ${eventType}:`, 
      event.data?.id || event.data?.name || 'Event received');
  }

  // Event handlers
  private handleBotStatusChange(event: SSEEventData): void {
    this.trackEvent('BOT_STATUS_CHANGED', event);
    const { botId, status, previousStatus } = event.data;
    console.log(`  🤖 Bot ${botId} changed from ${previousStatus} to ${status}`);
  }

  private handleBotRegistered(event: SSEEventData): void {
    this.trackEvent('BOT_REGISTERED', event);
    const { botId, name, capabilities } = event.data;
    console.log(`  ✅ Bot registered: ${name} (${botId}) with capabilities: ${capabilities?.join(', ')}`);
  }

  private handleBotUnregistered(event: SSEEventData): void {
    this.trackEvent('BOT_UNREGISTERED', event);
    const { botId, reason } = event.data;
    console.log(`  ❌ Bot unregistered: ${botId} (Reason: ${reason})`);
  }

  private handleBotHeartbeat(event: SSEEventData): void {
    this.trackEvent('BOT_HEARTBEAT', event);
    // Only log heartbeats in debug mode to avoid spam
    if (process.env.DEBUG_HEARTBEATS) {
      const { botId, timestamp } = event.data;
      console.log(`  💓 Heartbeat from ${botId} at ${timestamp}`);
    }
  }

  private handleJobCreated(event: SSEEventData): void {
    this.trackEvent('JOB_CREATED', event);
    const { jobId, campaignId, fuzzerType } = event.data;
    console.log(`  🆕 Job created: ${jobId} (Campaign: ${campaignId}, Fuzzer: ${fuzzerType})`);
  }

  private handleJobStarted(event: SSEEventData): void {
    this.trackEvent('JOB_STARTED', event);
    const { jobId, botId, startTime } = event.data;
    console.log(`  ▶️  Job started: ${jobId} on bot ${botId} at ${startTime}`);
  }

  private handleJobCompleted(event: SSEEventData): void {
    this.trackEvent('JOB_COMPLETED', event);
    const { jobId, duration, crashesFound, executionsPerSecond } = event.data;
    console.log(`  ✅ Job completed: ${jobId}`);
    console.log(`     Duration: ${duration}s, Crashes: ${crashesFound}, Exec/sec: ${executionsPerSecond}`);
  }

  private handleJobFailed(event: SSEEventData): void {
    this.trackEvent('JOB_FAILED', event);
    const { jobId, error, reason } = event.data;
    console.log(`  ❌ Job failed: ${jobId}`);
    console.log(`     Reason: ${reason}, Error: ${error}`);
  }

  private handleJobProgress(event: SSEEventData): void {
    this.trackEvent('JOB_PROGRESS', event);
    const { jobId, progress, executions, crashesFound } = event.data;
    console.log(`  📈 Job progress: ${jobId} - ${progress}% (Execs: ${executions}, Crashes: ${crashesFound})`);
  }

  private handleCampaignStarted(event: SSEEventData): void {
    this.trackEvent('CAMPAIGN_STARTED', event);
    const { campaignId, name, targetJobs } = event.data;
    console.log(`  🚀 Campaign started: ${name} (${campaignId}) with ${targetJobs} jobs`);
  }

  private handleCampaignStopped(event: SSEEventData): void {
    this.trackEvent('CAMPAIGN_STOPPED', event);
    const { campaignId, reason, stats } = event.data;
    console.log(`  ⏹️  Campaign stopped: ${campaignId} (Reason: ${reason})`);
    console.log(`     Stats: ${stats.completedJobs}/${stats.totalJobs} jobs completed`);
  }

  private handleCampaignCompleted(event: SSEEventData): void {
    this.trackEvent('CAMPAIGN_COMPLETED', event);
    const { campaignId, duration, totalCrashes, coverage } = event.data;
    console.log(`  🏁 Campaign completed: ${campaignId}`);
    console.log(`     Duration: ${duration}s, Crashes: ${totalCrashes}, Coverage: ${coverage}%`);
  }

  private handleCrashDiscovered(event: SSEEventData): void {
    this.trackEvent('CRASH_DISCOVERED', event);
    const { crashId, jobId, severity, type } = event.data;
    console.log(`  🐛 Crash discovered: ${crashId} (Job: ${jobId})`);
    console.log(`     Severity: ${severity}, Type: ${type}`);
  }

  private handleCrashMinimized(event: SSEEventData): void {
    this.trackEvent('CRASH_MINIMIZED', event);
    const { crashId, originalSize, minimizedSize, reduction } = event.data;
    console.log(`  🔍 Crash minimized: ${crashId}`);
    console.log(`     Size reduced from ${originalSize} to ${minimizedSize} bytes (${reduction}% reduction)`);
  }

  private handleCrashReproduced(event: SSEEventData): void {
    this.trackEvent('CRASH_REPRODUCED', event);
    const { crashId, reproduced, attempts } = event.data;
    const status = reproduced ? '✅ Successfully' : '❌ Failed to';
    console.log(`  🔄 ${status} reproduce crash: ${crashId} (${attempts} attempts)`);
  }

  private handleCorpusUpdated(event: SSEEventData): void {
    this.trackEvent('CORPUS_UPDATED', event);
    const { campaignId, addedEntries, totalEntries } = event.data;
    console.log(`  📚 Corpus updated for campaign ${campaignId}: +${addedEntries} entries (Total: ${totalEntries})`);
  }

  private handleCorpusSynced(event: SSEEventData): void {
    this.trackEvent('CORPUS_SYNCED', event);
    const { campaignId, syncedEntries, strategy } = event.data;
    console.log(`  🔄 Corpus synced for campaign ${campaignId}: ${syncedEntries} entries (Strategy: ${strategy})`);
  }

  private handleSystemAlert(event: SSEEventData): void {
    this.trackEvent('SYSTEM_ALERT', event);
    const { level, message, component } = event.data;
    const emoji = level === 'critical' ? '🚨' : level === 'warning' ? '⚠️' : 'ℹ️';
    console.log(`  ${emoji} System Alert [${level.toUpperCase()}]: ${message} (Component: ${component})`);
  }

  private handleSystemStatus(event: SSEEventData): void {
    this.trackEvent('SYSTEM_STATUS', event);
    const { status, uptime, activeBots, runningJobs } = event.data;
    console.log(`  📊 System Status: ${status} (Uptime: ${uptime}, Bots: ${activeBots}, Jobs: ${runningJobs})`);
  }
}

/**
 * Example: Basic event monitoring
 */
async function basicEventMonitoring() {
  console.log('🔍 Basic Event Monitoring Example\n');
  
  const dashboard = new EventMonitoringDashboard();
  
  // Monitor all events
  dashboard.startMonitoring();
  
  // Run for 2 minutes then stop
  setTimeout(() => {
    dashboard.stopMonitoring();
    process.exit(0);
  }, 120000);
}

/**
 * Example: Filtered event monitoring
 */
async function filteredEventMonitoring() {
  console.log('🎯 Filtered Event Monitoring Example\n');
  
  const dashboard = new EventMonitoringDashboard();
  
  // Monitor only crash and job completion events for a specific campaign
  const filter: SSEEventFilter = {
    types: [EventTypes.CRASH_DISCOVERED, EventTypes.JOB_COMPLETED, EventTypes.JOB_FAILED],
    campaignId: 'campaign-123' // Replace with actual campaign ID
  };
  
  dashboard.startMonitoring(filter);
  
  // Run for 2 minutes then stop
  setTimeout(() => {
    dashboard.stopMonitoring();
    process.exit(0);
  }, 120000);
}

/**
 * Example: Custom SSE client usage
 */
async function customSSEClientExample() {
  console.log('⚙️ Custom SSE Client Example\n');
  
  const sseClient = new SSEClient({
    baseUrl: 'http://localhost:8080/api/v1',
    debug: true,
    reconnectInterval: 2000,
    maxReconnectAttempts: 10
  });

  // Custom event handler for crash discoveries
  sseClient.on(EventTypes.CRASH_DISCOVERED, (event) => {
    console.log('🚨 CRITICAL: New crash discovered!');
    console.log('Details:', JSON.stringify(event.data, null, 2));
    
    // Here you could trigger alerts, send notifications, etc.
    triggerAlert(event.data);
  });

  // Monitor connection status
  sseClient.onOpen(() => {
    console.log('✅ Connected to SSE stream');
  });

  sseClient.onError((error) => {
    console.error('❌ SSE Error:', error.message);
  });

  // Connect with specific filtering
  sseClient.connect({
    types: [EventTypes.CRASH_DISCOVERED, EventTypes.SYSTEM_ALERT]
  });

  // Cleanup after 1 minute
  setTimeout(() => {
    sseClient.disconnect();
    console.log('Disconnected from SSE stream');
    process.exit(0);
  }, 60000);
}

/**
 * Simulated alert function
 */
function triggerAlert(crashData: any): void {
  // This would typically integrate with your alerting system
  console.log('📧 Sending alert notification...');
  console.log('🔔 Alert: High severity crash detected in production');
  
  // Example integrations:
  // - Send Slack notification
  // - Create PagerDuty incident  
  // - Send email to security team
  // - Create JIRA ticket
  // - Update monitoring dashboard
}

/**
 * Example: Comprehensive monitoring with statistics
 */
async function comprehensiveMonitoring() {
  console.log('📊 Comprehensive Monitoring with Statistics\n');
  
  const client = new PandaFuzzClient({
    baseUrl: 'http://localhost:8080/api/v1'
  });

  // Statistics tracking
  const stats = {
    crashes: 0,
    completedJobs: 0,
    failedJobs: 0,
    totalEvents: 0
  };

  // Set up event handlers with statistics
  client.onCrashDiscovered((data) => {
    stats.crashes++;
    stats.totalEvents++;
    console.log(`📈 Stats update: ${stats.crashes} crashes discovered`);
  });

  client.onJobCompleted((data) => {
    stats.completedJobs++;
    stats.totalEvents++;
    console.log(`📈 Stats update: ${stats.completedJobs} jobs completed`);
  });

  client.sse.on(EventTypes.JOB_FAILED, (event) => {
    stats.failedJobs++;
    stats.totalEvents++;
    console.log(`📈 Stats update: ${stats.failedJobs} jobs failed`);
  });

  client.connectEvents();

  // Print statistics every 30 seconds
  const statsInterval = setInterval(() => {
    console.log('\n📊 Current Statistics:');
    console.log(`  Total Events: ${stats.totalEvents}`);
    console.log(`  Crashes: ${stats.crashes}`);
    console.log(`  Completed Jobs: ${stats.completedJobs}`);
    console.log(`  Failed Jobs: ${stats.failedJobs}`);
    console.log(`  Success Rate: ${((stats.completedJobs / (stats.completedJobs + stats.failedJobs)) * 100 || 0).toFixed(1)}%`);
  }, 30000);

  // Cleanup after 3 minutes
  setTimeout(() => {
    clearInterval(statsInterval);
    client.close();
    console.log('\nFinal Statistics:', stats);
    process.exit(0);
  }, 180000);
}

// CLI interface
const command = process.argv[2];

switch (command) {
  case 'basic':
    basicEventMonitoring();
    break;
  case 'filtered':
    filteredEventMonitoring();
    break;
  case 'custom':
    customSSEClientExample();
    break;
  case 'comprehensive':
    comprehensiveMonitoring();
    break;
  default:
    console.log('Available commands:');
    console.log('  basic - Basic event monitoring');
    console.log('  filtered - Filtered event monitoring');
    console.log('  custom - Custom SSE client usage');
    console.log('  comprehensive - Comprehensive monitoring with statistics');
    console.log('\nUsage: node event-monitoring.js <command>');
}

export { 
  EventMonitoringDashboard, 
  basicEventMonitoring, 
  filteredEventMonitoring, 
  customSSEClientExample,
  comprehensiveMonitoring 
};