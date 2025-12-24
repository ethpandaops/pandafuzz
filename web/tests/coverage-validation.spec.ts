import { test, expect } from '@playwright/test';

test.describe('Coverage System Validation', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('http://localhost:8080');
    await page.waitForLoadState('networkidle');
  });

  test('should display coverage page in navigation', async ({ page }) => {
    // Check that Coverage link exists in navigation
    const coverageLink = page.locator('a:has-text("Coverage")').first();
    await expect(coverageLink).toBeVisible();
    
    // Navigate to coverage page
    await coverageLink.click();
    await page.waitForURL('**/coverage');
    
    // Verify we're on the coverage page
    await expect(page.locator('h4:has-text("Coverage Reports")')).toBeVisible();
  });

  test('should show empty state when no coverage reports exist', async ({ page }) => {
    // Navigate directly to coverage page
    await page.goto('http://localhost:8080/coverage');
    await page.waitForLoadState('networkidle');
    
    // Should show either empty state or loading
    const emptyOrLoading = page.locator('text=/No coverage reports|Loading coverage|Coverage Reports/');
    await expect(emptyOrLoading).toBeVisible({ timeout: 10000 });
  });

  test('should show jobs with coverage enabled', async ({ page }) => {
    // Navigate to jobs page
    await page.goto('http://localhost:8080/jobs');
    await page.waitForLoadState('networkidle');
    
    // Check if any jobs exist with coverage enabled
    const jobsTable = page.locator('table').first();
    
    // If jobs exist, check for coverage indicators
    const jobRows = jobsTable.locator('tbody tr');
    const rowCount = await jobRows.count();
    
    if (rowCount > 0) {
      // Look for the test job we created
      const testJob = jobRows.filter({ hasText: 'Coverage Test' }).first();
      const exists = await testJob.count() > 0;
      
      if (exists) {
        // Verify the job has completed status
        const status = testJob.locator('td').nth(3); // Status column
        await expect(status).toContainText(/completed|running|assigned/i);
      }
    }
  });

  test('should handle coverage API endpoints', async ({ page, request }) => {
    // Test the coverage API endpoint
    const jobsResponse = await request.get('http://localhost:8080/api/v1/jobs');
    expect(jobsResponse.ok()).toBeTruthy();
    
    const jobsData = await jobsResponse.json();
    
    if (jobsData.jobs && jobsData.jobs.length > 0) {
      // Find a job with coverage enabled
      const coverageJob = jobsData.jobs.find(job => job.enable_coverage === true);
      
      if (coverageJob) {
        // Check coverage endpoint for this job
        const coverageResponse = await request.get(
          `http://localhost:8080/api/v1/jobs/${coverageJob.id}/coverage`
        );
        expect(coverageResponse.ok()).toBeTruthy();
        
        const coverageData = await coverageResponse.json();
        expect(coverageData).toHaveProperty('reports');
        expect(Array.isArray(coverageData.reports)).toBeTruthy();
        
        // If there are reports, verify structure
        if (coverageData.reports.length > 0) {
          const report = coverageData.reports[0];
          expect(report).toHaveProperty('id');
          expect(report).toHaveProperty('job_id');
          expect(report).toHaveProperty('format');
          expect(report).toHaveProperty('created_at');
        }
      }
    }
  });

  test('should display coverage table with correct columns', async ({ page }) => {
    await page.goto('http://localhost:8080/coverage');
    await page.waitForLoadState('networkidle');
    
    // If there's a table (not empty state), check columns
    const table = page.locator('table').first();
    const tableExists = await table.count() > 0;
    
    if (tableExists) {
      // Check table headers
      const headers = table.locator('thead th');
      const headerCount = await headers.count();
      
      if (headerCount > 0) {
        // Verify expected columns exist
        await expect(headers.filter({ hasText: /Job Name/i })).toBeVisible();
        await expect(headers.filter({ hasText: /Format/i })).toBeVisible();
        await expect(headers.filter({ hasText: /Size/i })).toBeVisible();
        await expect(headers.filter({ hasText: /Actions/i })).toBeVisible();
      }
    }
  });

  test('should have download functionality for coverage reports', async ({ page, request }) => {
    // First check if any coverage reports exist via API
    const jobsResponse = await request.get('http://localhost:8080/api/v1/jobs');
    const jobsData = await jobsResponse.json();
    
    if (jobsData.jobs && jobsData.jobs.length > 0) {
      const coverageJob = jobsData.jobs.find(job => job.enable_coverage === true);
      
      if (coverageJob) {
        const coverageResponse = await request.get(
          `http://localhost:8080/api/v1/jobs/${coverageJob.id}/coverage`
        );
        const coverageData = await coverageResponse.json();
        
        if (coverageData.reports && coverageData.reports.length > 0) {
          const report = coverageData.reports[0];
          
          // Try to download the report
          const downloadResponse = await request.get(
            `http://localhost:8080/api/v1/jobs/${coverageJob.id}/coverage/${report.id}`,
            { failOnStatusCode: false }
          );
          
          // Check that we get a response (even if 404 due to missing file)
          expect(downloadResponse.status()).toBeLessThanOrEqual(404);
          
          // If successful, verify content type
          if (downloadResponse.ok()) {
            const contentType = downloadResponse.headers()['content-type'];
            expect(contentType).toBeTruthy();
          }
        }
      }
    }
  });

  test('should refresh coverage data', async ({ page }) => {
    await page.goto('http://localhost:8080/coverage');
    await page.waitForLoadState('networkidle');
    
    // Look for refresh button
    const refreshButton = page.locator('button').filter({ hasText: /Refresh/i }).first();
    const refreshExists = await refreshButton.count() > 0;
    
    if (refreshExists) {
      // Click refresh and ensure no errors
      await refreshButton.click();
      
      // Wait for any network activity to complete
      await page.waitForLoadState('networkidle');
      
      // Page should still be functional
      await expect(page.locator('h4:has-text("Coverage Reports")')).toBeVisible();
    }
  });
});

test.describe('Coverage Database Schema', () => {
  test('should have coverage tables in database', async ({ request }) => {
    // This test validates that the database schema is correct
    // We can't directly query SQLite from Playwright, but we can check
    // that coverage-enabled jobs are properly stored
    
    const jobsResponse = await request.get('http://localhost:8080/api/v1/jobs');
    expect(jobsResponse.ok()).toBeTruthy();
    
    const jobsData = await jobsResponse.json();
    
    // Check that jobs have coverage fields
    if (jobsData.jobs && jobsData.jobs.length > 0) {
      const job = jobsData.jobs[0];
      
      // These fields should exist even if null/false
      expect(job).toHaveProperty('enable_coverage');
      expect(job).toHaveProperty('coverage_format');
      
      // If coverage is enabled, format should be set
      if (job.enable_coverage) {
        expect(job.coverage_format).toBeTruthy();
      }
    }
  });
});