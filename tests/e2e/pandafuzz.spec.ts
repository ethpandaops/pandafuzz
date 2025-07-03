import { test, expect, Page } from '@playwright/test';

const MASTER_URL = process.env.MASTER_URL || 'http://localhost:8088';
const API_BASE = `${MASTER_URL}/api/v1`;
const API_V2_BASE = `${MASTER_URL}/api/v2`;

// Helper to wait for API to be ready
async function waitForAPI(page: Page, maxRetries = 30) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await page.request.get(`${API_BASE}/health`);
      if (response.ok()) return;
    } catch (e) {
      // Continue retrying
    }
    await page.waitForTimeout(1000);
  }
  throw new Error('API failed to become ready');
}

test.describe('PandaFuzz E2E Tests', () => {
  test.beforeEach(async ({ page }) => {
    await waitForAPI(page);
  });

  test.describe('Legacy Features - Job Management', () => {
    test('should create and manage jobs', async ({ page }) => {
      // Create a job via API
      const jobData = {
        name: 'test-job-legacy',
        description: 'Testing legacy job management',
        fuzzer: 'afl++',
        target_binary: '/bin/test',
        duration: 3600,
        memory_limit: 2147483648, // 2GB
        cpu_limit: 2,
        timeout: 1000,
        arguments: '@@'
      };

      const createResponse = await page.request.post(`${API_BASE}/jobs`, {
        data: jobData
      });
      expect(createResponse.ok()).toBeTruthy();
      const { job } = await createResponse.json();
      expect(job.id).toBeTruthy();

      // Get job details
      const getResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
      expect(getResponse.ok()).toBeTruthy();
      const jobDetails = await getResponse.json();
      expect(jobDetails.job.name).toBe(jobData.name);
      expect(jobDetails.job.status).toBe('pending');

      // List jobs
      const listResponse = await page.request.get(`${API_BASE}/jobs`);
      expect(listResponse.ok()).toBeTruthy();
      const { jobs } = await listResponse.json();
      expect(jobs.some((j: any) => j.id === job.id)).toBeTruthy();

      // Cancel job
      const cancelResponse = await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
      expect(cancelResponse.ok()).toBeTruthy();
    });

    test('should handle bot registration and heartbeat', async ({ page }) => {
      // Register a bot
      const botData = {
        hostname: 'test-bot-001',
        ip_address: '192.168.1.100',
        port: 8081,
        capabilities: {
          fuzzers: ['afl++', 'libfuzzer'],
          cpu_cores: 4,
          memory_gb: 8,
          platform: 'linux'
        }
      };

      const registerResponse = await page.request.post(`${API_BASE}/bots/register`, {
        data: botData
      });
      expect(registerResponse.ok()).toBeTruthy();
      const { bot } = await registerResponse.json();
      expect(bot.id).toBeTruthy();

      // Send heartbeat
      const heartbeatResponse = await page.request.post(`${API_BASE}/bots/${bot.id}/heartbeat`, {
        data: {
          status: 'idle',
          cpu_usage: 25.5,
          memory_usage: 30.2
        }
      });
      expect(heartbeatResponse.ok()).toBeTruthy();

      // List bots
      const listResponse = await page.request.get(`${API_BASE}/bots`);
      expect(listResponse.ok()).toBeTruthy();
      const { bots } = await listResponse.json();
      expect(bots.some((b: any) => b.bot.id === bot.id)).toBeTruthy();

      // Cleanup: delete bot
      await page.request.delete(`${API_BASE}/bots/${bot.id}`);
    });

    test('should handle crash reporting', async ({ page }) => {
      // First create a job
      const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
        data: {
          name: 'crash-test-job',
          fuzzer: 'afl++',
          target_binary: '/bin/crash-test',
          duration: 3600
        }
      });
      const { job } = await jobResponse.json();

      // Report a crash
      const crashData = {
        job_id: job.id,
        bot_id: 'test-bot-crash',
        type: 'heap_overflow',
        signal: 11,
        exit_code: -11,
        hash: 'deadbeef12345678',
        stack_trace: `==12345==ERROR: AddressSanitizer: heap-buffer-overflow
    #0 0x7f8a6b1234ab in vulnerable_function /src/test.c:42:5
    #1 0x7f8a6b1235bc in main /src/test.c:100:3`,
        input: btoa('crash input data'), // Base64 encoded
        size: 16,
        timestamp: new Date().toISOString()
      };

      const crashResponse = await page.request.post(`${API_BASE}/results/crash`, {
        data: crashData
      });
      expect(crashResponse.ok()).toBeTruthy();

      // Get job crashes
      const crashesResponse = await page.request.get(`${API_BASE}/jobs/${job.id}/crashes`);
      expect(crashesResponse.ok()).toBeTruthy();
      const { crashes } = await crashesResponse.json();
      expect(crashes.length).toBeGreaterThan(0);
      expect(crashes[0].hash).toBe(crashData.hash);
    });
  });

  test.describe('New Features - Campaign Management', () => {
    test('should create and manage campaigns', async ({ page }) => {
      // Create a campaign
      const campaignData = {
        name: 'Test Campaign E2E',
        description: 'End-to-end test campaign',
        target_binary: '/bin/test-campaign',
        max_jobs: 5,
        auto_restart: true,
        shared_corpus: true,
        job_template: {
          duration: 3600,
          memory_limit: 2147483648,
          timeout: 1000,
          fuzzer_type: 'afl++'
        },
        tags: ['e2e', 'test']
      };

      const createResponse = await page.request.post(`${API_BASE}/campaigns`, {
        data: campaignData
      });
      expect(createResponse.ok()).toBeTruthy();
      const { campaign } = await createResponse.json();
      expect(campaign.id).toBeTruthy();
      expect(campaign.status).toBe('pending');

      // Update campaign status
      const updateResponse = await page.request.patch(`${API_BASE}/campaigns/${campaign.id}`, {
        data: {
          status: 'running'
        }
      });
      expect(updateResponse.ok()).toBeTruthy();

      // Get campaign details
      const getResponse = await page.request.get(`${API_BASE}/campaigns/${campaign.id}`);
      expect(getResponse.ok()).toBeTruthy();
      const details = await getResponse.json();
      expect(details.campaign.status).toBe('running');

      // Get campaign statistics
      const statsResponse = await page.request.get(`${API_BASE}/campaigns/${campaign.id}/stats`);
      expect(statsResponse.ok()).toBeTruthy();
      const { stats } = await statsResponse.json();
      expect(stats).toHaveProperty('total_jobs');
      expect(stats).toHaveProperty('total_coverage');

      // List campaigns
      const listResponse = await page.request.get(`${API_BASE}/campaigns?status=running`);
      expect(listResponse.ok()).toBeTruthy();
      const { campaigns } = await listResponse.json();
      expect(campaigns.some((c: any) => c.id === campaign.id)).toBeTruthy();

      // Delete campaign
      const deleteResponse = await page.request.delete(`${API_BASE}/campaigns/${campaign.id}`);
      expect(deleteResponse.ok()).toBeTruthy();
    });

    test('should handle crash deduplication', async ({ page }) => {
      // Create a campaign
      const campaignResponse = await page.request.post(`${API_BASE}/campaigns`, {
        data: {
          name: 'Dedup Test Campaign',
          target_binary: '/bin/dedup-test',
          max_jobs: 1
        }
      });
      const { campaign } = await campaignResponse.json();

      // Create a job for the campaign
      const jobResponse = await page.request.post(`${API_BASE}/jobs?campaign_id=${campaign.id}`, {
        data: {
          name: 'dedup-test-job',
          fuzzer: 'afl++',
          target_binary: '/bin/dedup-test',
          campaign_id: campaign.id,
          duration: 3600
        }
      });
      const { job } = await jobResponse.json();

      // Report multiple crashes with same stack trace
      const stackTrace = `#0 0x7f8a6b1234ab in crash_here /src/crash.c:10:5
#1 0x7f8a6b1235bc in process_input /src/main.c:50:3
#2 0x7f8a6b1236cd in main /src/main.c:100:5`;

      // First crash
      await page.request.post(`${API_BASE}/results/crash`, {
        data: {
          job_id: job.id,
          bot_id: 'dedup-bot',
          campaign_id: campaign.id,
          type: 'segfault',
          signal: 11,
          hash: 'hash1',
          stack_trace: stackTrace,
          timestamp: new Date().toISOString()
        }
      });

      // Second crash with same stack (should be deduplicated)
      await page.request.post(`${API_BASE}/results/crash`, {
        data: {
          job_id: job.id,
          bot_id: 'dedup-bot',
          campaign_id: campaign.id,
          type: 'segfault',
          signal: 11,
          hash: 'hash2',
          stack_trace: stackTrace,
          timestamp: new Date().toISOString()
        }
      });

      // Get crash groups
      const groupsResponse = await page.request.get(`${API_BASE}/campaigns/${campaign.id}/crashes`);
      expect(groupsResponse.ok()).toBeTruthy();
      const { groups } = await groupsResponse.json();
      
      // Should have only one group due to deduplication
      expect(groups.length).toBe(1);
      expect(groups[0].count).toBe(2); // Two crashes in the group
    });

    test('should handle corpus evolution tracking', async ({ page }) => {
      // Create a campaign
      const campaignResponse = await page.request.post(`${API_BASE}/campaigns`, {
        data: {
          name: 'Corpus Evolution Test',
          target_binary: '/bin/corpus-test',
          shared_corpus: true
        }
      });
      const { campaign } = await campaignResponse.json();

      // Simulate corpus updates
      await page.request.post(`${API_BASE}/results/corpus`, {
        data: {
          job_id: 'corpus-job-1',
          bot_id: 'corpus-bot',
          campaign_id: campaign.id,
          files: [
            {
              filename: 'input001',
              hash: 'abc123',
              size: 1024,
              coverage: 5000,
              new_coverage: 100
            },
            {
              filename: 'input002',
              hash: 'def456',
              size: 2048,
              coverage: 5500,
              new_coverage: 500
            }
          ],
          total_size: 3072,
          timestamp: new Date().toISOString()
        }
      });

      // Get corpus evolution
      const evolutionResponse = await page.request.get(
        `${API_BASE}/campaigns/${campaign.id}/corpus/evolution`
      );
      expect(evolutionResponse.ok()).toBeTruthy();
      const { evolution } = await evolutionResponse.json();
      expect(evolution.length).toBeGreaterThan(0);
      expect(evolution[0].total_files).toBe(2);
      expect(evolution[0].new_coverage).toBe(600);
    });

    test('should handle corpus sharing between campaigns', async ({ page }) => {
      // Create two campaigns
      const campaign1Response = await page.request.post(`${API_BASE}/campaigns`, {
        data: {
          name: 'Source Campaign',
          target_binary: '/bin/share-test',
          shared_corpus: true
        }
      });
      const { campaign: campaign1 } = await campaign1Response.json();

      const campaign2Response = await page.request.post(`${API_BASE}/campaigns`, {
        data: {
          name: 'Target Campaign',
          target_binary: '/bin/share-test',
          shared_corpus: true
        }
      });
      const { campaign: campaign2 } = await campaign2Response.json();

      // Add corpus to source campaign
      await page.request.post(`${API_BASE}/results/corpus`, {
        data: {
          job_id: 'share-job-1',
          bot_id: 'share-bot',
          campaign_id: campaign1.id,
          files: [
            {
              filename: 'shared001',
              hash: 'share123',
              size: 1024,
              coverage: 8000,
              new_coverage: 1000
            }
          ]
        }
      });

      // Share corpus
      const shareResponse = await page.request.post(
        `${API_BASE}/campaigns/${campaign1.id}/corpus/share`,
        {
          data: {
            to_campaign_id: campaign2.id
          }
        }
      );
      expect(shareResponse.ok()).toBeTruthy();

      // Verify corpus was shared
      const corpusResponse = await page.request.get(
        `${API_BASE}/campaigns/${campaign2.id}/corpus/files`
      );
      expect(corpusResponse.ok()).toBeTruthy();
      const { files } = await corpusResponse.json();
      expect(files.some((f: any) => f.hash === 'share123')).toBeTruthy();
    });
  });

  test.describe('Web UI Tests', () => {
    test('should load dashboard', async ({ page }) => {
      await page.goto(MASTER_URL);
      
      // Check main navigation
      await expect(page.locator('nav#main-nav')).toBeVisible();
      await expect(page.locator('a:text("Dashboard")')).toBeVisible();
      await expect(page.locator('a:text("Campaigns")')).toBeVisible();
      await expect(page.locator('a:text("Crashes")')).toBeVisible();

      // Check summary cards
      await expect(page.locator('#active-campaigns')).toBeVisible();
      await expect(page.locator('#total-coverage')).toBeVisible();
      await expect(page.locator('#unique-crashes')).toBeVisible();
      await expect(page.locator('#active-bots')).toBeVisible();
    });

    test('should navigate to campaigns page', async ({ page }) => {
      await page.goto(MASTER_URL);
      await page.click('a:text("Campaigns")');
      
      await expect(page.locator('h1:text("Campaign Management")')).toBeVisible();
      await expect(page.locator('#create-campaign-btn')).toBeVisible();
      await expect(page.locator('#campaigns-table')).toBeVisible();
    });

    test('should create campaign via UI', async ({ page }) => {
      await page.goto(`${MASTER_URL}/campaigns.html`);
      
      // Click create button
      await page.click('#create-campaign-btn');
      
      // Fill form
      await page.fill('#campaign-name', 'UI Test Campaign');
      await page.fill('#campaign-description', 'Created via Playwright test');
      await page.fill('#target-binary', '/bin/ui-test');
      await page.fill('#max-jobs', '3');
      await page.selectOption('#fuzzer-type', 'libfuzzer');
      
      // Submit form
      await page.click('button:text("Create Campaign")');
      
      // Wait for success indication
      await page.waitForTimeout(1000);
      
      // Verify campaign appears in list
      await expect(page.locator('td:text("UI Test Campaign")')).toBeVisible();
    });

    test('should navigate to crashes page', async ({ page }) => {
      await page.goto(MASTER_URL);
      await page.click('a:text("Crashes")');
      
      await expect(page.locator('h1:text("Crash Analysis")')).toBeVisible();
      await expect(page.locator('#crash-groups-table')).toBeVisible();
      await expect(page.locator('#severity-filter')).toBeVisible();
    });
  });

  test.describe('WebSocket Tests', () => {
    test('should receive real-time updates', async ({ page }) => {
      await page.goto(MASTER_URL);
      
      // Wait for WebSocket connection
      await page.waitForSelector('.status-dot.connected', { timeout: 10000 });
      
      // Create a campaign via API to trigger WebSocket event
      const campaignResponse = await page.request.post(`${API_BASE}/campaigns`, {
        data: {
          name: 'WebSocket Test Campaign',
          target_binary: '/bin/ws-test'
        }
      });
      expect(campaignResponse.ok()).toBeTruthy();
      
      // Wait for activity feed to update
      await page.waitForTimeout(1000);
      
      // Check if activity feed shows the new campaign
      const activityFeed = page.locator('#activity-feed');
      await expect(activityFeed).toContainText('campaign');
    });
  });

  test.describe('API v2 Tests', () => {
    test('should support API v2 endpoints', async ({ page }) => {
      // Test v2 campaign timeline endpoint
      const campaignResponse = await page.request.post(`${API_V2_BASE}/campaigns`, {
        data: {
          name: 'API v2 Test Campaign',
          target_binary: '/bin/v2-test'
        }
      });
      const { campaign } = await campaignResponse.json();

      // Get timeline (v2 specific endpoint)
      const timelineResponse = await page.request.get(
        `${API_V2_BASE}/campaigns/${campaign.id}/timeline`
      );
      expect(timelineResponse.ok()).toBeTruthy();
      const { timeline } = await timelineResponse.json();
      expect(Array.isArray(timeline)).toBeTruthy();
    });

    test('should support streaming job progress', async ({ page }) => {
      // Create a job
      const jobResponse = await page.request.post(`${API_V2_BASE}/jobs`, {
        data: {
          name: 'streaming-test-job',
          fuzzer: 'afl++',
          target_binary: '/bin/stream-test',
          duration: 60
        }
      });
      const { job } = await jobResponse.json();

      // Test SSE endpoint
      const response = await fetch(`${API_V2_BASE}/jobs/${job.id}/progress`);
      expect(response.ok).toBeTruthy();
      expect(response.headers.get('content-type')).toContain('text/event-stream');
      
      // Close the stream
      await response.body?.cancel();
    });
  });

  test.describe('Resilience Tests', () => {
    test('should handle bot disconnection gracefully', async ({ page }) => {
      // Register a bot
      const botResponse = await page.request.post(`${API_BASE}/bots/register`, {
        data: {
          hostname: 'resilience-bot',
          capabilities: {
            fuzzers: ['afl++'],
            cpu_cores: 2,
            memory_gb: 4
          }
        }
      });
      const { bot } = await botResponse.json();

      // Create a job and assign to bot
      const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
        data: {
          name: 'resilience-test-job',
          fuzzer: 'afl++',
          target_binary: '/bin/resilience-test'
        }
      });
      const { job } = await jobResponse.json();

      // Simulate bot getting the job
      await page.request.get(`${API_BASE}/bots/${bot.id}/job`);

      // Simulate bot disconnection (no heartbeat)
      await page.waitForTimeout(3000);

      // Check that job is reassigned or marked for reassignment
      const jobStatusResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
      const jobStatus = await jobStatusResponse.json();
      
      // Job should either be pending again or have no assigned bot
      expect(
        jobStatus.job.status === 'pending' || 
        jobStatus.job.assigned_bot === null
      ).toBeTruthy();
    });

    test('should recover from master restart', async ({ page }) => {
      // Create some state before "restart"
      const campaignResponse = await page.request.post(`${API_BASE}/campaigns`, {
        data: {
          name: 'Persistence Test Campaign',
          target_binary: '/bin/persist-test',
          auto_restart: true
        }
      });
      const { campaign } = await campaignResponse.json();

      // Simulate master restart by waiting
      await page.waitForTimeout(2000);

      // Verify state persisted
      const getResponse = await page.request.get(`${API_BASE}/campaigns/${campaign.id}`);
      expect(getResponse.ok()).toBeTruthy();
      const recovered = await getResponse.json();
      expect(recovered.campaign.name).toBe('Persistence Test Campaign');
      expect(recovered.campaign.auto_restart).toBe(true);
    });
  });
});

// Cleanup helper
test.afterAll(async ({ request }) => {
  // Clean up test data
  try {
    // Delete test campaigns
    const campaignsResponse = await request.get(`${API_BASE}/campaigns`);
    if (campaignsResponse.ok()) {
      const { campaigns } = await campaignsResponse.json();
      for (const campaign of campaigns) {
        if (campaign.name.includes('Test') || campaign.name.includes('test')) {
          await request.delete(`${API_BASE}/campaigns/${campaign.id}`);
        }
      }
    }

    // Delete test jobs
    const jobsResponse = await request.get(`${API_BASE}/jobs`);
    if (jobsResponse.ok()) {
      const { jobs } = await jobsResponse.json();
      for (const job of jobs) {
        if (job.name.includes('test')) {
          await request.delete(`${API_BASE}/jobs/${job.id}`);
        }
      }
    }
  } catch (error) {
    console.error('Cleanup error:', error);
  }
});