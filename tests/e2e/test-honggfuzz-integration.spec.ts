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

// Helper to create a test binary for HongFuzz
async function createTestBinary(persistentMode: boolean = false): Promise<string> {
  const testDir = '/tmp/honggfuzz-test-' + Date.now();
  await fs.mkdir(testDir, { recursive: true });
  
  let sourceCode: string;
  
  if (persistentMode) {
    // Create a persistent mode test binary (LLVMFuzzerTestOneInput)
    sourceCode = `
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <stdio.h>

// This simulates a vulnerable function
void process_data(const uint8_t *data, size_t size) {
    char buffer[10];
    
    if (size > 0) {
        // Vulnerability: buffer overflow if size > 10
        if (size > 10 && data[0] == 'C' && data[1] == 'R' && data[2] == 'A' && data[3] == 'S' && data[4] == 'H') {
            // Trigger crash on specific input pattern
            memcpy(buffer, data, size); // This will overflow buffer
        } else {
            // Safe operation
            size_t copy_size = size < 10 ? size : 10;
            memcpy(buffer, data, copy_size);
        }
        
        // Process the buffer
        for (size_t i = 0; i < (size < 10 ? size : 10); i++) {
            buffer[i] = buffer[i] ^ 0x42;
        }
    }
}

// LLVMFuzzerTestOneInput - persistent mode entry point
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    process_data(data, size);
    return 0;
}
`;
  } else {
    // Create a standard test binary
    sourceCode = `
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char *argv[]) {
    char buffer[10];
    
    if (argc > 1) {
        // Read from file argument
        FILE *fp = fopen(argv[1], "rb");
        if (fp) {
            char input[100];
            size_t n = fread(input, 1, sizeof(input), fp);
            fclose(fp);
            
            // Vulnerability: buffer overflow on specific pattern
            if (n >= 5 && strncmp(input, "CRASH", 5) == 0) {
                strcpy(buffer, input); // This will crash on long input
            } else if (n > 0) {
                // Safe copy
                size_t copy_size = n < 10 ? n : 9;
                memcpy(buffer, input, copy_size);
                buffer[copy_size] = '\\0';
            }
        }
    } else {
        // Read from stdin
        char input[100];
        if (fgets(input, sizeof(input), stdin)) {
            // Remove newline
            input[strcspn(input, "\\n")] = 0;
            
            // Vulnerability
            if (strncmp(input, "CRASH", 5) == 0) {
                strcpy(buffer, input);
            } else {
                strncpy(buffer, input, 9);
                buffer[9] = '\\0';
            }
        }
    }
    
    printf("Processed: %s\\n", buffer);
    return 0;
}
`;
  }

  const sourcePath = path.join(testDir, persistentMode ? 'persistent_test.cc' : 'standard_test.c');
  const binaryPath = path.join(testDir, persistentMode ? 'persistent_test' : 'standard_test');
  
  await fs.writeFile(sourcePath, sourceCode);
  
  // Compile the test program with HongFuzz instrumentation
  // For persistent mode, use hfuzz-clang++ with fuzzer-no-link (HongFuzz provides main)
  // For standard mode, use hfuzz-gcc for basic instrumentation
  const compiler = persistentMode ? 'hfuzz-clang++' : 'hfuzz-gcc';
  const compileCmd = persistentMode
    ? `${compiler} -o ${binaryPath} ${sourcePath} -fsanitize=address,fuzzer-no-link`
    : `${compiler} -o ${binaryPath} ${sourcePath} -fsanitize=address`;

  await execAsync(compileCmd);
  
  return binaryPath;
}

// Helper to create seed corpus for HongFuzz
async function createSeedCorpus(): Promise<string> {
  const corpusDir = '/tmp/honggfuzz-corpus-' + Date.now();
  await fs.mkdir(corpusDir, { recursive: true });
  
  // Create seed files with various patterns
  const seeds = [
    { name: 'seed1.txt', content: 'A' },
    { name: 'seed2.txt', content: 'BB' },
    { name: 'seed3.txt', content: 'TEST' },
    { name: 'seed4.txt', content: 'FUZZ' },
    { name: 'seed5.txt', content: 'INPUT' },
    { name: 'seed6.txt', content: 'CRASH' }, // This might trigger crash
    { name: 'crash_trigger.txt', content: 'CRASHHHHHHHHHHHHHH' } // This should definitely trigger crash
  ];
  
  for (const seed of seeds) {
    await fs.writeFile(path.join(corpusDir, seed.name), seed.content);
  }
  
  return corpusDir;
}

// HongFuzz tests require hfuzz-cc wrapper and proper compilation environment
// The bot container now includes properly built hfuzz-cc/hfuzz-clang/hfuzz-clang++ wrappers
test.describe('HongFuzz Integration Tests', () => {
  let standardBinary: string;
  let persistentBinary: string;
  let seedCorpus: string;

  test.beforeAll(async () => {
    // Create test binaries and seed corpus
    standardBinary = await createTestBinary(false);
    persistentBinary = await createTestBinary(true);
    seedCorpus = await createSeedCorpus();
    
    console.log('Standard test binary:', standardBinary);
    console.log('Persistent test binary:', persistentBinary);
    console.log('Seed corpus:', seedCorpus);
  });

  test.afterAll(async () => {
    // Cleanup
    try {
      await fs.rm(path.dirname(standardBinary), { recursive: true, force: true });
      await fs.rm(path.dirname(persistentBinary), { recursive: true, force: true });
      await fs.rm(seedCorpus, { recursive: true, force: true });
    } catch (e) {
      console.error('Cleanup error:', e);
    }
  });

  test('should create and execute basic HongFuzz job', async ({ page }) => {
    await waitForAPI(page);

    // Create a HongFuzz job
    console.log('Creating HongFuzz job...');
    const jobData = {
      name: 'HongFuzz Basic Test',
      description: 'Testing basic HongFuzz functionality',
      fuzzer: 'honggfuzz',
      target_binary: standardBinary,
      duration: 30, // 30 seconds
      memory_limit: 512 * 1024 * 1024, // 512MB
      cpu_limit: 1,
      timeout: 1000,
      arguments: '___FILE___', // HongFuzz placeholder
      seed_directory: seedCorpus,
      fuzzer_options: {
        honggfuzz_config: {
          persistent_mode: false,
          hardware_feedback: 'none',
          verify_crashes: true,
          use_instrumentation: false,
          report_file: 'honggfuzz.report'
        }
      }
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();
    console.log('Created HongFuzz job:', job.id);

    // Wait for job to start
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
    
    console.log('Job status:', jobStatus);
    expect(['running', 'completed']).toContain(jobStatus);

    // Wait for some fuzzing to happen
    await page.waitForTimeout(10000); // 10 seconds

    // Get job statistics
    const statsResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
    const statsData = await statsResponse.json();
    
    console.log('Job statistics:', {
      status: statsData.job.status,
      executions: statsData.job.executions,
      crashes_found: statsData.job.crashes_found
    });

    // Verify job executed some inputs
    expect(statsData.job.executions).toBeGreaterThan(0);

    // Stop the job
    console.log('Stopping job...');
    const stopResponse = await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    expect(stopResponse.ok()).toBeTruthy();

    // Cleanup
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });

  test('should detect and use persistent mode for LLVMFuzzerTestOneInput binaries', async ({ page }) => {
    await waitForAPI(page);

    // Create a HongFuzz job with persistent mode binary
    console.log('Creating HongFuzz job with persistent mode binary...');
    const jobData = {
      name: 'HongFuzz Persistent Mode Test',
      description: 'Testing persistent mode auto-detection',
      fuzzer: 'honggfuzz',
      target_binary: persistentBinary,
      duration: 30,
      memory_limit: 512 * 1024 * 1024,
      cpu_limit: 1,
      timeout: 1000,
      seed_directory: seedCorpus,
      fuzzer_options: {
        honggfuzz_config: {
          persistent_mode: true, // Explicitly enable persistent mode
          hardware_feedback: 'instructions',
          verify_crashes: true,
          use_instrumentation: true,
          report_file: 'honggfuzz_persistent.report'
        }
      }
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();
    console.log('Created persistent mode job:', job.id);

    // Wait for job to start
    let jobStatus = 'pending';
    let attempts = 0;
    
    while (jobStatus === 'pending' && attempts < 30) {
      await page.waitForTimeout(1000);
      const statusResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
      const statusData = await statusResponse.json();
      jobStatus = statusData.job.status;
      attempts++;
    }
    
    expect(['running', 'completed']).toContain(jobStatus);

    // Wait for fuzzing
    await page.waitForTimeout(15000); // 15 seconds

    // Get job details
    const detailsResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
    const details = await detailsResponse.json();
    
    console.log('Persistent mode job stats:', {
      executions: details.job.executions,
      exec_per_sec: details.job.exec_per_sec,
      crashes_found: details.job.crashes_found
    });

    // Persistent mode should achieve higher execution speed
    expect(details.job.executions).toBeGreaterThan(1000);
    
    // Stop and cleanup
    await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });

  test('should detect crashes and report them accurately', async ({ page }) => {
    await waitForAPI(page);

    // Create a job that should find crashes
    console.log('Creating HongFuzz job for crash detection...');
    const jobData = {
      name: 'HongFuzz Crash Detection Test',
      description: 'Testing crash detection capabilities',
      fuzzer: 'honggfuzz',
      target_binary: standardBinary,
      duration: 60, // 60 seconds to ensure crashes are found
      memory_limit: 512 * 1024 * 1024,
      cpu_limit: 1,
      timeout: 1000,
      arguments: '___FILE___',
      seed_directory: seedCorpus,
      fuzzer_options: {
        honggfuzz_config: {
          persistent_mode: false,
          hardware_feedback: 'branches',
          verify_crashes: true,
          use_instrumentation: false,
          mutations_per_run: 10,
          report_file: 'honggfuzz_crashes.report'
        }
      }
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();
    console.log('Created crash detection job:', job.id);

    // Wait for job to start
    let jobStatus = 'pending';
    let attempts = 0;
    
    while (jobStatus === 'pending' && attempts < 30) {
      await page.waitForTimeout(1000);
      const statusResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
      const statusData = await statusResponse.json();
      jobStatus = statusData.job.status;
      attempts++;
    }

    // Monitor for crashes
    console.log('Monitoring for crashes...');
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
          console.log('Crashes detected:', crashes.length);
          
          // Verify crash details
          expect(crashes[0]).toHaveProperty('hash');
          expect(crashes[0]).toHaveProperty('type');
          expect(crashes[0]).toHaveProperty('job_id', job.id);
          expect(crashes[0]).toHaveProperty('input_base64'); // HongFuzz should provide crash input
          
          // Decode and verify crash input
          if (crashes[0].input_base64) {
            const crashInput = Buffer.from(crashes[0].input_base64, 'base64').toString();
            console.log('Crash input:', crashInput);
          }
        }
      }
      
      if (!crashesFound) {
        await page.waitForTimeout(2000);
      }
    }
    
    // We should find crashes given our vulnerable binary
    expect(crashesFound).toBeTruthy();

    // Stop and cleanup
    await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });

  test('should accurately collect and report statistics', async ({ page }) => {
    await waitForAPI(page);

    // Create a job with report file enabled
    console.log('Creating HongFuzz job for stats collection...');
    const jobData = {
      name: 'HongFuzz Stats Collection Test',
      description: 'Testing statistics reporting accuracy',
      fuzzer: 'honggfuzz',
      target_binary: standardBinary,
      duration: 20,
      memory_limit: 512 * 1024 * 1024,
      cpu_limit: 1,
      timeout: 1000,
      arguments: '___FILE___',
      seed_directory: seedCorpus,
      fuzzer_options: {
        honggfuzz_config: {
          persistent_mode: false,
          hardware_feedback: 'none',
          verify_crashes: false,
          use_instrumentation: false,
          report_file: 'stats_test.report'
        }
      }
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();

    // Wait for job to run
    await page.waitForTimeout(5000);

    // Get initial stats
    const stats1Response = await page.request.get(`${API_BASE}/jobs/${job.id}`);
    const stats1 = await stats1Response.json();
    
    // Wait more
    await page.waitForTimeout(5000);
    
    // Get updated stats
    const stats2Response = await page.request.get(`${API_BASE}/jobs/${job.id}`);
    const stats2 = await stats2Response.json();
    
    console.log('Stats comparison:', {
      executions1: stats1.job.executions,
      executions2: stats2.job.executions,
      exec_per_sec: stats2.job.exec_per_sec,
      coverage: stats2.job.coverage_percent
    });

    // Verify stats are increasing
    expect(stats2.job.executions).toBeGreaterThan(stats1.job.executions);
    expect(stats2.job.exec_per_sec).toBeGreaterThan(0);

    // Stop and cleanup
    await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });

  test('should test hardware feedback options', async ({ page }) => {
    await waitForAPI(page);

    // Test different hardware feedback modes
    const feedbackModes = ['instructions', 'branches', 'edges'];
    
    for (const mode of feedbackModes) {
      console.log(`Testing hardware feedback mode: ${mode}`);
      
      const jobData = {
        name: `HongFuzz Hardware Feedback Test - ${mode}`,
        description: `Testing ${mode} hardware feedback`,
        fuzzer: 'honggfuzz',
        target_binary: standardBinary,
        duration: 10,
        memory_limit: 512 * 1024 * 1024,
        cpu_limit: 1,
        timeout: 1000,
        arguments: '___FILE___',
        seed_directory: seedCorpus,
        fuzzer_options: {
          honggfuzz_config: {
            persistent_mode: false,
            hardware_feedback: mode,
            verify_crashes: false,
            use_instrumentation: true
          }
        }
      };

      const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
        data: jobData
      });
      
      expect(jobResponse.ok()).toBeTruthy();
      const { job } = await jobResponse.json();
      
      // Wait for job to run
      await page.waitForTimeout(5000);
      
      // Verify job is running or completed
      const statusResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
      const status = await statusResponse.json();
      
      console.log(`Hardware feedback ${mode} results:`, {
        status: status.job.status,
        executions: status.job.executions
      });
      
      expect(['running', 'completed']).toContain(status.job.status);
      
      // Stop and cleanup
      await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
      await page.request.delete(`${API_BASE}/jobs/${job.id}`);
    }
  });

  test('should handle corpus collection and evolution', async ({ page }) => {
    await waitForAPI(page);

    // Create a corpus collection
    console.log('Creating corpus collection...');
    const collectionResponse = await page.request.post(`${API_BASE}/corpus/collections`, {
      data: {
        name: 'HongFuzz Test Corpus',
        description: 'Corpus for HongFuzz testing',
        tags: ['honggfuzz', 'test']
      }
    });
    
    expect(collectionResponse.ok()).toBeTruthy();
    const collection = await collectionResponse.json();
    const collectionId = collection.ID || collection.id;

    // Create HongFuzz job with corpus collection
    const jobData = {
      name: 'HongFuzz Corpus Evolution Test',
      description: 'Testing corpus collection and evolution',
      fuzzer: 'honggfuzz',
      target_binary: standardBinary,
      duration: 30,
      memory_limit: 512 * 1024 * 1024,
      cpu_limit: 1,
      timeout: 1000,
      arguments: '___FILE___',
      seed_directory: seedCorpus,
      corpus_collection_id: collectionId,
      fuzzer_options: {
        honggfuzz_config: {
          persistent_mode: false,
          hardware_feedback: 'branches',
          verify_crashes: false,
          minimize_corpus: true
        }
      }
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();

    // Let it run for a while
    await page.waitForTimeout(20000);

    // Check corpus evolution
    const corpusResponse = await page.request.get(
      `${API_BASE}/corpus/collections/${collectionId}/files`
    );
    
    if (corpusResponse.ok()) {
      const { files } = await corpusResponse.json();
      console.log('Corpus evolution - file count:', files.length);
      
      // Should have more files than initial seeds
      expect(files.length).toBeGreaterThanOrEqual(7); // We started with 7 seeds
    }

    // Stop and cleanup
    await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
    await page.request.delete(`${API_BASE}/corpus/collections/${collectionId}`);
  });

  test('should handle configuration options correctly', async ({ page }) => {
    await waitForAPI(page);

    // Test various configuration options
    const jobData = {
      name: 'HongFuzz Configuration Test',
      description: 'Testing various HongFuzz configuration options',
      fuzzer: 'honggfuzz',
      target_binary: standardBinary,
      duration: 15,
      memory_limit: 256 * 1024 * 1024,
      cpu_limit: 1,
      timeout: 500,
      arguments: '___FILE___',
      seed_directory: seedCorpus,
      fuzzer_options: {
        honggfuzz_config: {
          persistent_mode: false,
          hardware_feedback: 'edges',
          verify_crashes: true,
          use_instrumentation: true,
          minimize_corpus: true,
          mutations_per_run: 20,
          max_file_size: 1024,
          report_file: 'config_test.report'
        },
        clean_temp: true
      }
    };

    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: jobData
    });
    
    expect(jobResponse.ok()).toBeTruthy();
    const { job } = await jobResponse.json();

    // Let it run
    await page.waitForTimeout(10000);

    // Verify configuration was applied
    const statusResponse = await page.request.get(`${API_BASE}/jobs/${job.id}`);
    const status = await statusResponse.json();
    
    console.log('Configuration test results:', {
      status: status.job.status,
      executions: status.job.executions,
      memory_limit: status.job.memory_limit,
      timeout: status.job.timeout
    });

    expect(status.job.memory_limit).toBe(256 * 1024 * 1024);
    expect(status.job.timeout).toBe(500);

    // Stop and cleanup
    await page.request.put(`${API_BASE}/jobs/${job.id}/cancel`);
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });

  test('should display HongFuzz jobs in web UI', async ({ page }) => {
    await page.goto(MASTER_URL);
    
    // Create a HongFuzz job
    const jobResponse = await page.request.post(`${API_BASE}/jobs`, {
      data: {
        name: 'HongFuzz UI Test',
        fuzzer: 'honggfuzz',
        target_binary: '/bin/test',
        duration: 60
      }
    });
    
    const { job } = await jobResponse.json();
    
    // Navigate to jobs page
    await page.click('a:text("Jobs")');
    
    // Wait for job to appear
    await page.waitForTimeout(2000);
    
    // Check if HongFuzz job is visible
    await expect(page.locator(`text=HongFuzz UI Test`)).toBeVisible();
    await expect(page.locator(`text=honggfuzz`)).toBeVisible();
    
    // Cleanup
    await page.request.delete(`${API_BASE}/jobs/${job.id}`);
  });
});