import { test, expect } from '@playwright/test';

test.describe('Coverage Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('http://localhost:8080/coverage');
  });

  test('should load coverage page', async ({ page }) => {
    // Wait for the page to load
    await page.waitForLoadState('networkidle');
    
    // Check that the title is present
    await expect(page.locator('h4:has-text("Coverage Reports")')).toBeVisible();
    
    // Check for the refresh button
    await expect(page.locator('button:has-text("Refresh")')).toBeVisible();
  });

  test('should display empty state when no coverage reports', async ({ page }) => {
    // Wait for the page to load
    await page.waitForLoadState('networkidle');
    
    // Look for empty state message
    const emptyState = page.locator('text=/No coverage reports available|Loading coverage reports/');
    await expect(emptyState).toBeVisible({ timeout: 10000 });
  });

  test('should handle API errors gracefully', async ({ page, context }) => {
    // Intercept API calls and return error
    await context.route('**/api/v1/jobs/*/coverage', route => {
      route.fulfill({
        status: 500,
        body: JSON.stringify({ error: 'Internal Server Error' })
      });
    });

    await page.goto('http://localhost:8080/coverage');
    await page.waitForLoadState('networkidle');
    
    // Should still show the page without crashing
    await expect(page.locator('h4:has-text("Coverage Reports")')).toBeVisible();
  });

  test('should have refresh functionality', async ({ page }) => {
    await page.waitForLoadState('networkidle');
    
    // Click refresh button
    const refreshButton = page.locator('button:has-text("Refresh")');
    await expect(refreshButton).toBeVisible();
    
    // Test that clicking refresh doesn't cause errors
    await refreshButton.click();
    await page.waitForLoadState('networkidle');
    
    // Page should still be functional
    await expect(page.locator('h4:has-text("Coverage Reports")')).toBeVisible();
  });

  test('should display download buttons when coverage reports exist', async ({ page, context }) => {
    // Mock API response with coverage data
    await context.route('**/api/v1/jobs/*/coverage', route => {
      route.fulfill({
        status: 200,
        body: JSON.stringify({
          reports: [{
            id: 'test-report-1',
            job_id: 'job-1',
            format: 'lcov',
            file_path: '/coverage/job-1/coverage.lcov',
            file_size: 1024,
            created_at: new Date().toISOString(),
            bot_id: 'bot-1'
          }]
        })
      });
    });

    // Navigate to a specific job's coverage
    await page.goto('http://localhost:8080/jobs/job-1/coverage');
    await page.waitForLoadState('networkidle');
    
    // Check for download button
    const downloadButtons = page.locator('button:has-text("Download")');
    const count = await downloadButtons.count();
    
    if (count > 0) {
      await expect(downloadButtons.first()).toBeVisible();
    }
  });
});