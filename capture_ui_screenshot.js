const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  
  try {
    // Navigate to the application
    await page.goto('http://localhost:3001');
    await page.waitForTimeout(2000);
    
    // Take screenshot of main page
    await page.screenshot({ path: 'ui_main.png', fullPage: true });
    console.log('Captured main page');
    
    // Navigate to Jobs page
    await page.click('text=Jobs');
    await page.waitForTimeout(1000);
    await page.screenshot({ path: 'ui_jobs.png', fullPage: true });
    console.log('Captured jobs page');
    
    // Click Create Job
    await page.click('button:has-text("Create Job")');
    await page.waitForTimeout(1000);
    await page.screenshot({ path: 'ui_create_job.png', fullPage: true });
    console.log('Captured create job dialog');
    
  } catch (error) {
    console.error('Error:', error);
  } finally {
    await browser.close();
  }
})();