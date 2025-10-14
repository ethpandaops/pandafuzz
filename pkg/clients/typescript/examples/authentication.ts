/**
 * Authentication Flows Example
 * 
 * This example demonstrates various authentication patterns and
 * secure token management with the PandaFuzz client SDK.
 */

import { PandaFuzzClient, Configuration } from '@pandafuzz/client';

/**
 * Token storage interface for different storage strategies
 */
interface TokenStorage {
  getToken(): Promise<string | null>;
  setToken(token: string): Promise<void>;
  removeToken(): Promise<void>;
}

/**
 * Memory-based token storage (for development/testing)
 */
class MemoryTokenStorage implements TokenStorage {
  private token: string | null = null;

  async getToken(): Promise<string | null> {
    return this.token;
  }

  async setToken(token: string): Promise<void> {
    this.token = token;
  }

  async removeToken(): Promise<void> {
    this.token = null;
  }
}

/**
 * File-based token storage (for server environments)
 */
class FileTokenStorage implements TokenStorage {
  constructor(private filePath: string) {}

  async getToken(): Promise<string | null> {
    try {
      const fs = await import('fs/promises');
      const data = await fs.readFile(this.filePath, 'utf-8');
      return data.trim();
    } catch (error) {
      if ((error as any).code === 'ENOENT') {
        return null;
      }
      throw error;
    }
  }

  async setToken(token: string): Promise<void> {
    const fs = await import('fs/promises');
    await fs.writeFile(this.filePath, token, 'utf-8');
  }

  async removeToken(): Promise<void> {
    try {
      const fs = await import('fs/promises');
      await fs.unlink(this.filePath);
    } catch (error) {
      if ((error as any).code !== 'ENOENT') {
        throw error;
      }
    }
  }
}

/**
 * Environment variable token storage (for containerized environments)
 */
class EnvironmentTokenStorage implements TokenStorage {
  constructor(private envVarName: string = 'PANDAFUZZ_TOKEN') {}

  async getToken(): Promise<string | null> {
    return process.env[this.envVarName] || null;
  }

  async setToken(token: string): Promise<void> {
    process.env[this.envVarName] = token;
  }

  async removeToken(): Promise<void> {
    delete process.env[this.envVarName];
  }
}

/**
 * JWT token manager with automatic refresh
 */
class JWTTokenManager {
  private tokenStorage: TokenStorage;
  private refreshTimer?: NodeJS.Timeout;
  private onTokenRefresh?: (token: string) => void;

  constructor(
    tokenStorage: TokenStorage,
    private refreshEndpoint: string,
    private refreshThresholdSeconds: number = 300 // Refresh 5 minutes before expiry
  ) {
    this.tokenStorage = tokenStorage;
  }

  /**
   * Get current token, refreshing if necessary
   */
  async getValidToken(): Promise<string | null> {
    const token = await this.tokenStorage.getToken();
    
    if (!token) {
      return null;
    }

    // Check if token is about to expire
    if (this.isTokenNearExpiry(token)) {
      console.log('🔄 Token is near expiry, attempting refresh...');
      return await this.refreshToken();
    }

    return token;
  }

  /**
   * Set new token and schedule automatic refresh
   */
  async setToken(token: string): Promise<void> {
    await this.tokenStorage.setToken(token);
    this.scheduleTokenRefresh(token);
    
    if (this.onTokenRefresh) {
      this.onTokenRefresh(token);
    }
  }

  /**
   * Remove token and cancel refresh timer
   */
  async removeToken(): Promise<void> {
    await this.tokenStorage.removeToken();
    
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
      this.refreshTimer = undefined;
    }
  }

  /**
   * Set callback for token refresh events
   */
  onRefresh(callback: (token: string) => void): void {
    this.onTokenRefresh = callback;
  }

  /**
   * Check if token is near expiry
   */
  private isTokenNearExpiry(token: string): boolean {
    try {
      const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64').toString());
      const expiryTime = payload.exp * 1000; // Convert to milliseconds
      const currentTime = Date.now();
      const timeUntilExpiry = expiryTime - currentTime;
      
      return timeUntilExpiry <= this.refreshThresholdSeconds * 1000;
    } catch (error) {
      console.error('❌ Failed to parse JWT token:', error);
      return true; // Assume expired if we can't parse
    }
  }

  /**
   * Refresh the token
   */
  private async refreshToken(): Promise<string> {
    try {
      const currentToken = await this.tokenStorage.getToken();
      
      if (!currentToken) {
        throw new Error('No token to refresh');
      }

      const response = await fetch(this.refreshEndpoint, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${currentToken}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        throw new Error(`Token refresh failed: ${response.statusText}`);
      }

      const data = await response.json();
      const newToken = data.access_token || data.token;

      if (!newToken) {
        throw new Error('No token in refresh response');
      }

      await this.setToken(newToken);
      console.log('✅ Token refreshed successfully');
      
      return newToken;
    } catch (error) {
      console.error('❌ Token refresh failed:', error);
      await this.removeToken(); // Remove invalid token
      throw error;
    }
  }

  /**
   * Schedule automatic token refresh
   */
  private scheduleTokenRefresh(token: string): void {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer);
    }

    try {
      const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64').toString());
      const expiryTime = payload.exp * 1000;
      const currentTime = Date.now();
      const refreshTime = expiryTime - (this.refreshThresholdSeconds * 1000);
      const timeUntilRefresh = refreshTime - currentTime;

      if (timeUntilRefresh > 0) {
        console.log(`⏰ Token refresh scheduled in ${Math.round(timeUntilRefresh / 1000)}s`);
        
        this.refreshTimer = setTimeout(async () => {
          try {
            await this.refreshToken();
          } catch (error) {
            console.error('❌ Scheduled token refresh failed:', error);
          }
        }, timeUntilRefresh);
      }
    } catch (error) {
      console.error('❌ Failed to schedule token refresh:', error);
    }
  }
}

/**
 * Authenticated PandaFuzz client with advanced token management
 */
class AuthenticatedPandaFuzzClient {
  private client: PandaFuzzClient;
  private tokenManager?: JWTTokenManager;

  constructor(
    private baseUrl: string = 'http://localhost:8080/api/v1',
    tokenStorage?: TokenStorage
  ) {
    this.client = new PandaFuzzClient({ baseUrl });

    if (tokenStorage) {
      this.tokenManager = new JWTTokenManager(
        tokenStorage,
        `${baseUrl}/auth/refresh`
      );

      // Set up token refresh callback
      this.tokenManager.onRefresh((newToken) => {
        this.updateClientToken(newToken);
      });
    }
  }

  /**
   * Initialize client with stored token
   */
  async initialize(): Promise<void> {
    if (this.tokenManager) {
      const token = await this.tokenManager.getValidToken();
      if (token) {
        this.updateClientToken(token);
        console.log('✅ Initialized with stored token');
      }
    }
  }

  /**
   * Authenticate with username/password
   */
  async loginWithPassword(username: string, password: string): Promise<void> {
    console.log(`🔐 Authenticating user: ${username}`);

    try {
      const response = await fetch(`${this.baseUrl}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      });

      if (!response.ok) {
        throw new Error(`Authentication failed: ${response.statusText}`);
      }

      const data = await response.json();
      const token = data.access_token || data.token;

      if (!token) {
        throw new Error('No token in authentication response');
      }

      await this.setToken(token);
      console.log('✅ Authentication successful');
      
    } catch (error) {
      console.error('❌ Authentication failed:', error);
      throw error;
    }
  }

  /**
   * Authenticate with API key
   */
  async loginWithApiKey(apiKey: string): Promise<void> {
    console.log('🗝️ Authenticating with API key');

    // Update client configuration with API key
    this.client.updateConfiguration({ apiKey });

    // Test the API key by making a simple request
    try {
      await this.client.health.getHealth();
      console.log('✅ API key authentication successful');
    } catch (error) {
      console.error('❌ API key authentication failed:', error);
      throw error;
    }
  }

  /**
   * Authenticate with OAuth2 flow
   */
  async loginWithOAuth2(
    clientId: string,
    clientSecret: string,
    scope: string = 'read write'
  ): Promise<void> {
    console.log('🔗 Authenticating with OAuth2');

    try {
      const response = await fetch(`${this.baseUrl}/auth/oauth2/token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({
          grant_type: 'client_credentials',
          client_id: clientId,
          client_secret: clientSecret,
          scope
        })
      });

      if (!response.ok) {
        throw new Error(`OAuth2 authentication failed: ${response.statusText}`);
      }

      const data = await response.json();
      const token = data.access_token;

      if (!token) {
        throw new Error('No access token in OAuth2 response');
      }

      await this.setToken(token);
      console.log('✅ OAuth2 authentication successful');
      
    } catch (error) {
      console.error('❌ OAuth2 authentication failed:', error);
      throw error;
    }
  }

  /**
   * Set token and update client
   */
  private async setToken(token: string): Promise<void> {
    if (this.tokenManager) {
      await this.tokenManager.setToken(token);
    }
    this.updateClientToken(token);
  }

  /**
   * Update client configuration with new token
   */
  private updateClientToken(token: string): void {
    this.client.updateConfiguration({ accessToken: token });
  }

  /**
   * Logout and clean up tokens
   */
  async logout(): Promise<void> {
    console.log('👋 Logging out...');

    try {
      // Attempt to revoke token on server
      await fetch(`${this.baseUrl}/auth/logout`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${await this.tokenManager?.getValidToken()}`
        }
      });
    } catch (error) {
      console.warn('⚠️ Server logout failed:', error);
    }

    // Clean up local token storage
    if (this.tokenManager) {
      await this.tokenManager.removeToken();
    }

    // Clear client authentication
    this.client.updateConfiguration({ accessToken: undefined, apiKey: undefined });
    
    console.log('✅ Logged out successfully');
  }

  /**
   * Check if currently authenticated
   */
  async isAuthenticated(): Promise<boolean> {
    try {
      await this.client.health.getHealth();
      return true;
    } catch (error: any) {
      return error.status !== 401;
    }
  }

  /**
   * Get user information (if token contains user data)
   */
  async getUserInfo(): Promise<any> {
    if (!this.tokenManager) {
      throw new Error('No token manager configured');
    }

    const token = await this.tokenManager.getValidToken();
    
    if (!token) {
      throw new Error('No token available');
    }

    try {
      const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64').toString());
      return {
        userId: payload.sub,
        username: payload.username || payload.preferred_username,
        email: payload.email,
        roles: payload.roles || [],
        permissions: payload.permissions || [],
        expiresAt: new Date(payload.exp * 1000)
      };
    } catch (error) {
      console.error('❌ Failed to parse user info from token:', error);
      throw error;
    }
  }

  /**
   * Access the underlying PandaFuzz client
   */
  get pandafuzz(): PandaFuzzClient {
    return this.client;
  }
}

/**
 * Example: Password-based authentication flow
 */
async function passwordAuthenticationExample() {
  console.log('🔐 Password Authentication Example\n');

  const tokenStorage = new MemoryTokenStorage();
  const client = new AuthenticatedPandaFuzzClient('http://localhost:8080/api/v1', tokenStorage);

  try {
    // Initialize with stored token (if any)
    await client.initialize();

    // Check if already authenticated
    if (await client.isAuthenticated()) {
      console.log('✅ Already authenticated');
      
      // Get user info
      const userInfo = await client.getUserInfo();
      console.log('User info:', {
        username: userInfo.username,
        roles: userInfo.roles,
        expiresAt: userInfo.expiresAt
      });
    } else {
      // Authenticate with username/password
      await client.loginWithPassword('admin', 'password123');
      
      // Test authenticated request
      const bots = await client.pandafuzz.bots.listBots();
      console.log(`Retrieved ${bots.data?.length || 0} bots`);
    }

    // Logout
    await client.logout();

  } catch (error) {
    console.error('Password authentication example failed:', error);
  }
}

/**
 * Example: API key authentication
 */
async function apiKeyAuthenticationExample() {
  console.log('🗝️ API Key Authentication Example\n');

  const client = new AuthenticatedPandaFuzzClient();

  try {
    // Authenticate with API key
    await client.loginWithApiKey('your-api-key-here');

    // Test authenticated requests
    const [health, bots] = await Promise.all([
      client.pandafuzz.health.getHealth(),
      client.pandafuzz.bots.listBots()
    ]);

    console.log('Health status:', health.status);
    console.log(`Found ${bots.data?.length || 0} bots`);

  } catch (error) {
    console.error('API key authentication example failed:', error);
  }
}

/**
 * Example: OAuth2 authentication
 */
async function oauth2AuthenticationExample() {
  console.log('🔗 OAuth2 Authentication Example\n');

  const tokenStorage = new FileTokenStorage('/tmp/pandafuzz-token');
  const client = new AuthenticatedPandaFuzzClient('http://localhost:8080/api/v1', tokenStorage);

  try {
    await client.initialize();

    if (!(await client.isAuthenticated())) {
      // Authenticate with OAuth2
      await client.loginWithOAuth2(
        'your-client-id',
        'your-client-secret',
        'read write admin'
      );
    }

    // Use the authenticated client
    const campaigns = await client.pandafuzz.campaigns.listCampaigns();
    console.log(`Found ${campaigns.data?.length || 0} campaigns`);

    // Token will be automatically refreshed when needed

  } catch (error) {
    console.error('OAuth2 authentication example failed:', error);
  }
}

/**
 * Example: Multi-tenant authentication
 */
async function multiTenantAuthenticationExample() {
  console.log('🏢 Multi-tenant Authentication Example\n');

  // Different clients for different tenants
  const tenantClients = new Map<string, AuthenticatedPandaFuzzClient>();

  const tenants = ['tenant-a', 'tenant-b', 'tenant-c'];

  for (const tenantId of tenants) {
    console.log(`Setting up client for tenant: ${tenantId}`);
    
    const tokenStorage = new FileTokenStorage(`/tmp/pandafuzz-token-${tenantId}`);
    const client = new AuthenticatedPandaFuzzClient(
      `http://localhost:8080/api/v1`,
      tokenStorage
    );

    // Initialize and authenticate each tenant
    await client.initialize();
    
    if (!(await client.isAuthenticated())) {
      await client.loginWithPassword(`admin-${tenantId}`, `password-${tenantId}`);
    }

    tenantClients.set(tenantId, client);
  }

  // Use tenant-specific clients
  for (const [tenantId, client] of tenantClients) {
    try {
      const bots = await client.pandafuzz.bots.listBots();
      console.log(`Tenant ${tenantId}: ${bots.data?.length || 0} bots`);
    } catch (error) {
      console.error(`Failed to get bots for tenant ${tenantId}:`, error);
    }
  }

  // Cleanup
  for (const client of tenantClients.values()) {
    await client.logout();
  }
}

/**
 * Example: Custom authentication middleware
 */
async function customAuthenticationExample() {
  console.log('⚙️ Custom Authentication Middleware Example\n');

  // Custom configuration with authentication middleware
  const config = new Configuration({
    basePath: 'http://localhost:8080/api/v1',
    middleware: [
      {
        pre: async (context) => {
          // Add custom authentication headers
          context.init.headers = {
            ...context.init.headers,
            'X-API-Version': '1.0',
            'X-Client-Version': '1.0.0',
            'X-Request-ID': `req-${Date.now()}-${Math.random()}`
          };

          // Add timestamp-based signature (example)
          const timestamp = Math.floor(Date.now() / 1000);
          const signature = generateSignature(context.url, timestamp);
          
          context.init.headers['X-Timestamp'] = timestamp.toString();
          context.init.headers['X-Signature'] = signature;

          console.log(`🔐 Added custom auth headers to ${context.url}`);
          return context;
        },
        
        post: async (context) => {
          // Log response for debugging
          console.log(`📝 Response: ${context.response.status} for ${context.url}`);
          
          // Handle token refresh on 401 responses
          if (context.response.status === 401) {
            console.log('🔄 Received 401, attempting token refresh...');
            // Custom token refresh logic could go here
          }
          
          return context;
        }
      }
    ]
  });

  const client = new PandaFuzzClient({ baseUrl: undefined }, config);

  try {
    const health = await client.health.getHealth();
    console.log('Custom auth successful:', health.status);
  } catch (error) {
    console.error('Custom auth failed:', error);
  }
}

/**
 * Generate example signature for custom authentication
 */
function generateSignature(url: string, timestamp: number): string {
  // This is a simplified example - in real implementations,
  // use proper cryptographic signing
  const crypto = require('crypto');
  const secret = 'your-secret-key';
  const payload = `${url}:${timestamp}`;
  
  return crypto
    .createHmac('sha256', secret)
    .update(payload)
    .digest('hex');
}

// CLI interface
const command = process.argv[2];

switch (command) {
  case 'password':
    passwordAuthenticationExample();
    break;
  case 'apikey':
    apiKeyAuthenticationExample();
    break;
  case 'oauth2':
    oauth2AuthenticationExample();
    break;
  case 'multi-tenant':
    multiTenantAuthenticationExample();
    break;
  case 'custom':
    customAuthenticationExample();
    break;
  default:
    console.log('Available commands:');
    console.log('  password - Password-based authentication');
    console.log('  apikey - API key authentication');
    console.log('  oauth2 - OAuth2 authentication flow');
    console.log('  multi-tenant - Multi-tenant authentication');
    console.log('  custom - Custom authentication middleware');
    console.log('\nUsage: node authentication.js <command>');
}

export {
  TokenStorage,
  MemoryTokenStorage,
  FileTokenStorage,
  EnvironmentTokenStorage,
  JWTTokenManager,
  AuthenticatedPandaFuzzClient,
  passwordAuthenticationExample,
  apiKeyAuthenticationExample,
  oauth2AuthenticationExample,
  multiTenantAuthenticationExample,
  customAuthenticationExample
};