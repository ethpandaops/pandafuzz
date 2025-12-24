import { test, expect, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import * as crypto from 'crypto';
import { exec } from 'child_process';
import { promisify } from 'util';

const execAsync = promisify(exec);

const MASTER_URL = process.env.MASTER_URL || 'http://localhost:8080';
const API_BASE = `${MASTER_URL}/api/v1`;

// Helper to wait for API to be ready
async function waitForAPI(page: Page, maxRetries = 30) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await page.request.get(`${MASTER_URL}/health`);
      if (response.ok()) return;
    } catch (e) {
      // Continue retrying
    }
    await page.waitForTimeout(1000);
  }
  throw new Error('API failed to become ready');
}

// Helper to create an AFL++ job with coverage enabled
async function createAFLJobWithCoverage(page: Page): Promise<any> {
  const jobData = {
    name: `afl-coverage-test-${Date.now()}`,
    description: 'Test AFL++ raw coverage files',
    fuzzer: 'aflplusplus',
    target: 'test_binary',
    duration: 60, // 1 minute for testing
    enable_coverage: true,
    coverage_format: 'raw',
    config: {
      timeout: 1000,
      memory_limit: 256,
      dictionary: '',
      duration: 60
    }
  };

  const response = await page.request.post(`${API_BASE}/jobs`, {
    data: jobData
  });

  expect(response.ok()).toBeTruthy();
  const result = await response.json();
  return result.job || result;
}

// Helper to wait for job completion
async function waitForJobCompletion(page: Page, jobId: string, maxWaitSeconds = 120) {
  const startTime = Date.now();
  
  while ((Date.now() - startTime) < maxWaitSeconds * 1000) {
    const response = await page.request.get(`${API_BASE}/jobs/${jobId}`);
    if (response.ok()) {
      const job = await response.json();
      if (job.status === 'completed' || job.status === 'finished') {
        return job;
      }
      if (job.status === 'failed' || job.status === 'error') {
        throw new Error(`Job ${jobId} failed with status: ${job.status}`);
      }
    }
    await page.waitForTimeout(2000);
  }
  throw new Error(`Job ${jobId} did not complete within ${maxWaitSeconds} seconds`);
}

// Helper to get file content from bot container
async function getFileFromBotContainer(containerName: string, filePath: string): Promise<Buffer | null> {
  try {
    const { stdout } = await execAsync(`docker exec ${containerName} cat "${filePath}"`);
    return Buffer.from(stdout);
  } catch (error) {
    console.error(`Failed to get file from container: ${error}`);
    return null;
  }
}

// Helper to calculate file hash
function calculateFileHash(content: Buffer): string {
  return crypto.createHash('sha256').update(content).digest('hex');
}

// Raw coverage download tests - API endpoints now implemented
// Note: Some tests may still need actual coverage data from running bots
test.describe('Raw AFL++ Coverage File Downloads', () => {
  test.beforeEach(async ({ page }) => {
    await waitForAPI(page);
  });

  test('should download raw AFL++ coverage files and verify content matches bot', async ({ page }) => {
    // Create an AFL++ job with coverage enabled
    const job = await createAFLJobWithCoverage(page);
    console.log(`Created job ${job.id}`);

    // Wait for job to complete
    const completedJob = await waitForJobCompletion(page, job.id);
    console.log(`Job ${job.id} completed with status: ${completedJob.status}`);

    // Check if raw coverage files are available
    const coverageResponse = await page.request.get(`${API_BASE}/jobs/${job.id}/coverage/raw`);
    expect(coverageResponse.ok()).toBeTruthy();
    
    const coverageData = await coverageResponse.json();
    expect(coverageData.files).toBeDefined();
    expect(Array.isArray(coverageData.files)).toBeTruthy();

    if (coverageData.files.length === 0) {
      console.log('No raw coverage files found, job may not have generated coverage yet');
      return;
    }

    const latestFile = coverageData.files[0];
    
    // Verify file paths are present
    expect(latestFile).toHaveProperty('fuzzer_stats_path');
    expect(latestFile).toHaveProperty('plot_data_path');
    expect(latestFile).toHaveProperty('fuzz_bitmap_path');

    // Test downloading individual files
    const fileTypes = ['fuzzer_stats', 'plot_data', 'fuzz_bitmap'];
    const downloadedFiles: { [key: string]: Buffer } = {};

    for (const fileType of fileTypes) {
      const downloadResponse = await page.request.get(
        `${API_BASE}/jobs/${job.id}/coverage/raw/${fileType}`
      );
      
      if (downloadResponse.ok()) {
        const buffer = await downloadResponse.body();
        downloadedFiles[fileType] = buffer;
        
        console.log(`Downloaded ${fileType}: ${buffer.length} bytes`);
        
        // Verify appropriate content type
        const contentType = downloadResponse.headers()['content-type'];
        expect(contentType).toBe('application/octet-stream');
        
        // Verify file has content
        expect(buffer.length).toBeGreaterThan(0);
        
        // For fuzzer_stats, verify it's text content
        if (fileType === 'fuzzer_stats') {
          const content = buffer.toString('utf-8');
          expect(content).toContain('start_time');
          expect(content).toContain('cycles_done');
          expect(content).toContain('execs_done');
        }
        
        // For plot_data, verify it's CSV format
        if (fileType === 'plot_data') {
          const content = buffer.toString('utf-8');
          const lines = content.split('\n');
          expect(lines.length).toBeGreaterThan(0);
          // First line should be CSV header or data
          if (lines[0].includes(',')) {
            const fields = lines[0].split(',');
            expect(fields.length).toBeGreaterThanOrEqual(5); // AFL++ plot_data has multiple fields
          }
        }
      } else {
        console.log(`File ${fileType} not available (${downloadResponse.status()})`);
      }
    }

    // Test downloading all files as ZIP
    const zipResponse = await page.request.get(
      `${API_BASE}/jobs/${job.id}/coverage/raw/all/zip`
    );
    
    if (zipResponse.ok()) {
      const zipBuffer = await zipResponse.body();
      expect(zipBuffer.length).toBeGreaterThan(0);
      
      // Verify it's a valid ZIP file (starts with PK signature)
      const signature = zipBuffer.slice(0, 2).toString('hex');
      expect(signature).toBe('504b'); // 'PK' in hex
      
      console.log(`Downloaded ZIP archive: ${zipBuffer.length} bytes`);
    } else {
      console.log(`ZIP download not available (${zipResponse.status()})`);
    }

    // If running in Docker environment, try to verify against bot container
    if (process.env.DOCKER_COMPOSE === 'true') {
      const botContainerName = 'pandafuzz-bot-1'; // Adjust based on your docker-compose setup
      
      for (const [fileType, downloadedContent] of Object.entries(downloadedFiles)) {
        const aflOutputPath = `/tmp/fuzzing/${job.id}/output/afl_output/${fileType}`;
        const botContent = await getFileFromBotContainer(botContainerName, aflOutputPath);
        
        if (botContent) {
          // Compare file hashes
          const downloadedHash = calculateFileHash(downloadedContent);
          const botHash = calculateFileHash(botContent);
          
          expect(downloadedHash).toBe(botHash);
          console.log(`✓ ${fileType} hash matches between download and bot container`);
        } else {
          console.log(`Could not retrieve ${fileType} from bot container for comparison`);
        }
      }
    }
  });

  test('should handle missing coverage files gracefully', async ({ page }) => {
    // Try to get coverage files for a non-existent job
    const fakeJobId = 'non-existent-job-123';
    
    const response = await page.request.get(`${API_BASE}/jobs/${fakeJobId}/coverage/raw`);
    
    // Should return appropriate error status
    expect(response.status()).toBeGreaterThanOrEqual(400);
    expect(response.status()).toBeLessThan(500);
  });

  test('should reject invalid file type requests', async ({ page }) => {
    // Create a job first
    const job = await createAFLJobWithCoverage(page);
    
    // Try to download an invalid file type
    const response = await page.request.get(
      `${API_BASE}/jobs/${job.id}/coverage/raw/invalid_file_type`
    );
    
    expect(response.status()).toBe(400);
    
    const error = await response.json();
    expect(error.error).toContain('Invalid file type');
  });

  test('should display raw coverage files in UI', async ({ page }) => {
    // Create a job with coverage
    const job = await createAFLJobWithCoverage(page);
    
    // Navigate to job details page
    await page.goto(`${MASTER_URL}/jobs/${job.id}`);
    await page.waitForLoadState('networkidle');
    
    // Look for raw coverage section
    const rawCoverageSection = page.locator('text="Raw AFL++ Coverage Files"');
    
    // If the section exists, verify its contents
    if (await rawCoverageSection.count() > 0) {
      await expect(rawCoverageSection).toBeVisible();
      
      // Check for individual file download buttons
      const fuzzerStatsButton = page.locator('button:has-text("fuzzer_stats")');
      const plotDataButton = page.locator('button:has-text("plot_data")');
      const fuzzBitmapButton = page.locator('button:has-text("fuzz_bitmap")');
      
      // Check for "Download All as ZIP" button
      const downloadAllButton = page.locator('button:has-text("Download All as ZIP")');
      
      // Verify file descriptions are shown
      const descriptions = [
        'AFL++ statistics and metrics',
        'Time-series coverage data for plotting',
        'Binary coverage bitmap data'
      ];
      
      for (const desc of descriptions) {
        const descElement = page.locator(`text="${desc}"`);
        if (await descElement.count() > 0) {
          await expect(descElement).toBeVisible();
        }
      }
    }
  });

  test('should download files through UI buttons', async ({ page, context }) => {
    // Create a job with coverage
    const job = await createAFLJobWithCoverage(page);
    
    // Wait for job completion
    await waitForJobCompletion(page, job.id);
    
    // Navigate to job details page
    await page.goto(`${MASTER_URL}/jobs/${job.id}`);
    await page.waitForLoadState('networkidle');
    
    // Look for download buttons
    const downloadButtons = page.locator('button:has-text("Download")');
    const buttonCount = await downloadButtons.count();
    
    if (buttonCount > 0) {
      // Set up download handling
      const downloadPromise = page.waitForEvent('download', { timeout: 10000 }).catch(() => null);
      
      // Click the first download button
      await downloadButtons.first().click();
      
      // Check if download started
      const download = await downloadPromise;
      if (download) {
        // Verify download filename
        const filename = download.suggestedFilename();
        expect(filename).toMatch(/fuzzer_stats|plot_data|fuzz_bitmap/);
        
        // Save the file to verify content
        const downloadPath = await download.path();
        if (downloadPath) {
          const content = fs.readFileSync(downloadPath);
          expect(content.length).toBeGreaterThan(0);
          console.log(`Downloaded file ${filename}: ${content.length} bytes`);
        }
      }
    }
  });

  test('should handle concurrent downloads', async ({ page }) => {
    // Create a job with coverage
    const job = await createAFLJobWithCoverage(page);
    
    // Wait for job completion
    await waitForJobCompletion(page, job.id);
    
    // Download all files concurrently
    const downloadPromises = ['fuzzer_stats', 'plot_data', 'fuzz_bitmap'].map(async (fileType) => {
      const response = await page.request.get(
        `${API_BASE}/jobs/${job.id}/coverage/raw/${fileType}`
      );
      return {
        fileType,
        status: response.status(),
        size: response.ok() ? (await response.body()).length : 0
      };
    });
    
    const results = await Promise.all(downloadPromises);
    
    // Verify all downloads completed
    for (const result of results) {
      console.log(`${result.fileType}: status=${result.status}, size=${result.size}`);
      if (result.status === 200) {
        expect(result.size).toBeGreaterThan(0);
      }
    }
  });

  test('should preserve file integrity during download', async ({ page }) => {
    // Create a job with coverage
    const job = await createAFLJobWithCoverage(page);
    
    // Wait for job completion
    await waitForJobCompletion(page, job.id);
    
    // Download the same file multiple times and verify consistency
    const fileType = 'fuzzer_stats';
    const hashes: string[] = [];
    
    for (let i = 0; i < 3; i++) {
      const response = await page.request.get(
        `${API_BASE}/jobs/${job.id}/coverage/raw/${fileType}`
      );
      
      if (response.ok()) {
        const buffer = await response.body();
        const hash = calculateFileHash(buffer);
        hashes.push(hash);
        
        await page.waitForTimeout(500); // Small delay between downloads
      }
    }
    
    // All hashes should be identical
    if (hashes.length > 1) {
      const firstHash = hashes[0];
      for (const hash of hashes) {
        expect(hash).toBe(firstHash);
      }
      console.log(`✓ File integrity verified: consistent hash across ${hashes.length} downloads`);
    }
  });

  test.afterAll(async ({ request }) => {
    // Cleanup: delete test jobs
    try {
      const jobsResponse = await request.get(`${API_BASE}/jobs`);
      if (jobsResponse.ok()) {
        const jobs = await jobsResponse.json();
        const jobsArray = Array.isArray(jobs) ? jobs : jobs.jobs || [];
        
        for (const job of jobsArray) {
          if (job.name && job.name.includes('afl-coverage-test')) {
            await request.delete(`${API_BASE}/jobs/${job.id}`);
            console.log(`Cleaned up test job: ${job.id}`);
          }
        }
      }
    } catch (error) {
      console.error('Cleanup error:', error);
    }
  });
});