import { FullConfig } from '@playwright/test';

async function globalTeardown(config: FullConfig) {
  console.log('🧹 Cleaning up PandaFuzz e2e test environment...');
  
  try {
    // Clean up any test resources if needed
    // For now, we just log the teardown
    console.log('✅ Global teardown completed');
  } catch (error) {
    console.error('❌ Error during global teardown:', error);
  }
}

export default globalTeardown;