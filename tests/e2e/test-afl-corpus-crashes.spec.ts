import { test, expect, Page } from '@playwright/test';
import { exec } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs/promises';
import * as path from 'path';

const execAsync = promisify(exec);

const MASTER_URL = process.env.MASTER_URL || 'http://localhost:8088';
const API_BASE = `${MASTER_URL}/api/v1`;

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

// Helper to create a vulnerable test binary
async function createVulnerableTestBinary(): Promise<string> {
  const testDir = '/tmp/afl-test-' + Date.now();
  await fs.mkdir(testDir, { recursive: true });
  
  // Create a simple vulnerable C program
  const vulnerableCode = `
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char *argv[]) {
    char buffer[10];
    
    if (argc > 1) {
        // Vulnerable strcpy - will crash on long input
        strcpy(buffer, argv[1]);
    } else {
        // Read from stdin
        char input[100];
        if (fgets(input, sizeof(input), stdin)) {
            // Remove newline
            input[strcspn(input, "\\n")] = 0;
            // Vulnerable strcpy - will crash on long input
            strcpy(buffer, input);
        }
    }
    
    printf("Input: %s\\n", buffer);
    return 0;
}
`;

  const sourcePath = path.join(testDir, 'vulnerable.c');
  const binaryPath = path.join(testDir, 'vulnerable');
  
  await fs.writeFile(sourcePath, vulnerableCode);
  
  // Compile the vulnerable program
  await execAsync(`gcc -o ${binaryPath} ${sourcePath}`);
  
  return binaryPath;
}

// Helper to create seed corpus files
async function createSeedCorpus(): Promise<string> {
  const corpusDir = '/tmp/afl-corpus-' + Date.now();
  await fs.mkdir(corpusDir, { recursive: true });
  
  // Create initial seed files
  const seeds = [
    { name: 'seed1.txt', content: 'A' },
    { name: 'seed2.txt', content: 'BB' },
    { name: 'seed3.txt', content: 'CCC' },
    { name: 'seed4.txt', content: 'DDDD' },
    { name: 'seed5.txt', content: 'EEEEE' },
    { name: 'crash_seed.txt', content: 'AAAAAAAAAAAAAAAAAAAA' } // This should trigger crash
  ];
  
  for (const seed of seeds) {
    await fs.writeFile(path.join(corpusDir, seed.name), seed.content);
  }
  
  return corpusDir;
}

// Note: AFL++ tests require real fuzzing which can be slow
// These tests validate the corpus collection API endpoints
test.describe('AFL++ Corpus Collection and Crash Detection', () => {
  let testBinary: string;
  let seedCorpus: string;

  test.beforeAll(async () => {
    // Create test binary and seed corpus
    testBinary = await createVulnerableTestBinary();
    seedCorpus = await createSeedCorpus();
    
    console.log('Test binary created:', testBinary);
    console.log('Seed corpus created:', seedCorpus);
  });

  test.afterAll(async () => {
    // Cleanup
    try {
      await fs.rm(path.dirname(testBinary), { recursive: true, force: true });
      await fs.rm(seedCorpus, { recursive: true, force: true });
    } catch (e) {
      console.error('Cleanup error:', e);
    }
  });

  test('should create corpus collection, run AFL++ job, and detect crashes', async ({ page }) => {
    await waitForAPI(page);

    // Step 1: Create a corpus collection
    console.log('Creating corpus collection...');
    const collectionResponse = await page.request.post(`${API_BASE}/corpus/collections`, {
      data: {
        name: 'AFL++ Test Corpus',
        description: 'Test corpus for AFL++ crash detection',
        tags: ['afl++', 'test', 'crash-detection']
      }
    });
    
    expect(collectionResponse.ok()).toBeTruthy();
    const collection = await collectionResponse.json();
    const collectionId = collection.ID || collection.id;
    console.log('Created corpus collection:', collectionId);

    // Step 2: Upload seed files to the corpus collection
    console.log('Uploading seed files to corpus collection...');
    const formData = new FormData();
    
    // Read and add seed files to form data
    const seedFiles = await fs.readdir(seedCorpus);
    for (const file of seedFiles) {
      const filePath = path.join(seedCorpus, file);
      const content = await fs.readFile(filePath);
      const blob = new Blob([content], { type: 'application/octet-stream' });
      formData.append('files', blob, file);
    }

    const uploadResponse = await fetch(`${API_BASE}/corpus/collections/${collectionId}/upload`, {
      method: 'POST',
      body: formData
    });
    
    expect(uploadResponse.ok).toBeTruthy();
    const uploadResult = await uploadResponse.json();
    console.log('Uploaded files:', uploadResult);

    // Step 3: Create an AFL++ job using the corpus collection
    console.log('Creating AFL++ job...');
    const jobData = {
      name: 'AFL++ Crash Detection Test',
      description: 'Testing AFL++ with vulnerable binary',
      fuzzer: 'afl++',
      target_binary: testBinary,
      duration: 30, // Run for 30 seconds
      memory_limit: 512 * 1024 * 1024, // 512MB
      cpu_limit: 1,
      timeout: 1000,
      arguments: '@@', // AFL++ will replace with input file
      seed_directory: seedCorpus,
      corpus_collection_id: collectionId
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();
    console.log('Created job:', job.id);

    // Step 4: Wait for job to start running
    console.log('Waiting for job to start...');
    let jobStatus = 'pending';
    let attempts = 0;
    
    while (jobStatus === 'pending' && attempts < 30) {
      await page.waitForTimeout(1000);
      const statusResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
      const statusData = await statusResponse.json();
      jobStatus = statusData.job.status;
      attempts++;
    }
    
    console.log('Job status after waiting:', jobStatus);
    expect(['running', 'completed']).toContain(jobStatus);

    // Step 5: Monitor for crashes using Playwright
    console.log('Monitoring for crashes...');
    
    // Navigate to the crashes page
    await page.goto(`${MASTER_URL}/crashes.html`);
    
    // Wait for the page to load
    await expect(page.locator('h1:text("Crash Analysis")')).toBeVisible();
    
    // Monitor for crashes to appear
    let crashesFound = false;
    const maxWaitTime = 60000; // 60 seconds
    const startTime = Date.now();
    
    while (!crashesFound && (Date.now() - startTime) < maxWaitTime) {
      // Check API for crashes
      const crashesResponse = await page.request.get(`${API_BASE}/jobs/${job.id}/crashes`);
      if (crashesResponse.ok()) {
        const { crashes } = await crashesResponse.json();
        if (crashes && crashes.length > 0) {
          crashesFound = true;
          console.log('Crashes detected via API:', crashes.length);
          
          // Verify crash details
          expect(crashes.length).toBeGreaterThan(0);
          expect(crashes[0]).toHaveProperty('hash');
          expect(crashes[0]).toHaveProperty('type');
          expect(crashes[0]).toHaveProperty('job_id', job.id);
        }
      }
      
      // Also check UI for crash updates
      await page.reload();
      const crashTableRows = await page.locator('#crash-groups-table tbody tr').count();
      if (crashTableRows > 0) {
        console.log('Crashes visible in UI:', crashTableRows);
        crashesFound = true;
      }
      
      if (!crashesFound) {
        await page.waitForTimeout(2000);
      }
    }
    
    // Assert that crashes were found
    expect(crashesFound).toBeTruthy();
    console.log('Successfully detected crashes!');

    // Step 6: Verify crash details in the UI
    if (await page.locator('#crash-groups-table tbody tr').count() > 0) {
      // Click on the first crash to see details
      await page.locator('#crash-groups-table tbody tr').first().click();
      
      // Wait for crash details to load
      await page.waitForTimeout(1000);
      
      // Verify crash details are displayed
      const crashDetails = page.locator('.crash-details, #crash-detail-modal, .modal-content');
      if (await crashDetails.count() > 0) {
        await expect(crashDetails.first()).toBeVisible();
        console.log('Crash details displayed in UI');
      }
    }

    // Step 7: Stop the job
    console.log('Stopping the job...');
    const stopResponse = await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    expect(stopResponse.ok()).toBeTruthy();

    // Step 8: Get final statistics
    const finalStatsResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
    const finalStats = await finalStatsResponse.json();
    
    console.log('Final job statistics:', {
      status: finalStats.job.status,
      duration: finalStats.job.duration,
      crashes_found: finalStats.job.crashes_found
    });

    // Step 9: Verify corpus evolution
    const corpusEvolutionResponse = await page.request.get(
      `${API_BASE}/corpus/collections/${collectionId}/files`
    );
    
    if (corpusEvolutionResponse.ok()) {
      const { files } = await corpusEvolutionResponse.json();
      console.log('Final corpus size:', files.length);
      expect(files.length).toBeGreaterThanOrEqual(seedFiles.length);
    }

    // Cleanup: Delete the job and collection
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
    await page.request.delete(`${API_BASE}/corpus/collections/${collectionId}`);
  });

  test('should display crash information in web UI', async ({ page }) => {
    await page.goto(MASTER_URL);
    
    // Navigate to crashes page
    await page.click('a:text("Crashes")');
    await expect(page.locator('h1:text("Crash Analysis")')).toBeVisible();
    
    // Check if crash groups table exists
    await expect(page.locator('#crash-groups-table')).toBeVisible();
    
    // If there are crashes, verify the UI elements
    const crashRows = await page.locator('#crash-groups-table tbody tr').count();
    if (crashRows > 0) {
      // Verify table headers
      await expect(page.locator('#crash-groups-table th:text("Hash")')).toBeVisible();
      await expect(page.locator('#crash-groups-table th:text("Type")')).toBeVisible();
      await expect(page.locator('#crash-groups-table th:text("Count")')).toBeVisible();
      
      // Verify severity filter
      await expect(page.locator('#severity-filter')).toBeVisible();
      
      console.log('Crash UI elements verified');
    }
  });

  test('should handle real-time crash updates via WebSocket', async ({ page }) => {
    await page.goto(MASTER_URL);
    
    // Wait for WebSocket connection
    await page.waitForSelector('.status-dot.connected', { timeout: 10000 });
    
    // Create a simple job that will crash quickly
    const quickJobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: {
        name: 'Quick Crash Test',
        fuzzer: 'afl++',
        target_binary: testBinary,
        duration: 10,
        memory_limit: 256 * 1024 * 1024,
        seed_directory: seedCorpus,
        arguments: '@@'
      }
    });
    
    const { job } = await quickJobResponse.json();
    
    // Monitor activity feed for crash notifications
    let crashNotificationFound = false;
    const maxWait = 30000; // 30 seconds
    const startTime = Date.now();
    
    while (!crashNotificationFound && (Date.now() - startTime) < maxWait) {
      const activityFeed = page.locator('#activity-feed');
      const feedText = await activityFeed.textContent();
      
      if (feedText && (feedText.includes('crash') || feedText.includes('Crash'))) {
        crashNotificationFound = true;
        console.log('Crash notification found in activity feed');
      }
      
      if (!crashNotificationFound) {
        await page.waitForTimeout(1000);
      }
    }
    
    // Cleanup
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });
});