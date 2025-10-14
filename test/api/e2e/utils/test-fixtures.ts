/**
 * Test fixtures for realistic testing scenarios
 */

export interface TestBinary {
  name: string;
  path: string;
  args: string[];
  environment?: Record<string, string>;
  expectedCrashTypes?: string[];
}

export interface TestCampaignConfig {
  name: string;
  description: string;
  fuzzerType: 'libfuzzer' | 'afl++' | 'honggfuzz';
  duration: number;
  memoryLimit: number;
  timeout: number;
  expectedCrashes?: number;
  tags?: string[];
}

export interface TestJobConfig {
  name: string;
  description: string;
  fuzzerType: 'libfuzzer' | 'afl++' | 'honggfuzz';
  duration: number;
  memoryLimit: number;
  timeout: number;
  args: string[];
  tags?: string[];
}

/**
 * Common test binaries for fuzzing
 */
export const TEST_BINARIES: TestBinary[] = [
  {
    name: 'vulnerable-buffer',
    path: '/test-resources/test-targets/vulnerable/vulnerable-app.c',
    args: ['@@'],
    expectedCrashTypes: ['buffer_overflow', 'segmentation_fault']
  },
  {
    name: 'format-string',
    path: '/test-resources/test-targets/crashers/test-crasher.c',
    args: ['@@'],
    expectedCrashTypes: ['format_string', 'segmentation_fault']
  },
  {
    name: 'simple-crash',
    path: '/test-resources/test-targets/crashers/segfault.c',
    args: ['@@'],
    expectedCrashTypes: ['segmentation_fault']
  }
];

/**
 * Campaign configurations for different testing scenarios
 */
export const CAMPAIGN_CONFIGS: Record<string, TestCampaignConfig> = {
  'quick-afl': {
    name: 'Quick AFL++ Test',
    description: 'Fast AFL++ campaign for basic functionality testing',
    fuzzerType: 'afl++',
    duration: 180, // 3 minutes
    memoryLimit: 256 * 1024 * 1024, // 256MB
    timeout: 1000,
    expectedCrashes: 1,
    tags: ['afl++', 'quick', 'test']
  },
  
  'quick-libfuzzer': {
    name: 'Quick LibFuzzer Test',
    description: 'Fast LibFuzzer campaign for basic functionality testing',
    fuzzerType: 'libfuzzer',
    duration: 180, // 3 minutes
    memoryLimit: 256 * 1024 * 1024, // 256MB
    timeout: 1000,
    expectedCrashes: 1,
    tags: ['libfuzzer', 'quick', 'test']
  },
  
  'intensive-testing': {
    name: 'Intensive Testing Campaign',
    description: 'Longer campaign for stress testing and performance validation',
    fuzzerType: 'afl++',
    duration: 600, // 10 minutes
    memoryLimit: 512 * 1024 * 1024, // 512MB
    timeout: 2000,
    expectedCrashes: 5,
    tags: ['intensive', 'performance', 'stress-test']
  },
  
  'multi-fuzzer': {
    name: 'Multi-Fuzzer Campaign',
    description: 'Campaign using multiple fuzzer types',
    fuzzerType: 'afl++', // Primary fuzzer
    duration: 300, // 5 minutes
    memoryLimit: 384 * 1024 * 1024, // 384MB
    timeout: 1500,
    expectedCrashes: 2,
    tags: ['multi-fuzzer', 'comprehensive']
  }
};

/**
 * Job configurations for individual job testing
 */
export const JOB_CONFIGS: Record<string, TestJobConfig> = {
  'quick-job': {
    name: 'Quick Test Job',
    description: 'Fast job for basic functionality testing',
    fuzzerType: 'libfuzzer',
    duration: 60, // 1 minute
    memoryLimit: 128 * 1024 * 1024, // 128MB
    timeout: 1000,
    args: ['@@'],
    tags: ['quick', 'basic-test']
  },
  
  'crash-discovery': {
    name: 'Crash Discovery Job',
    description: 'Job specifically designed to discover crashes',
    fuzzerType: 'afl++',
    duration: 300, // 5 minutes
    memoryLimit: 256 * 1024 * 1024, // 256MB
    timeout: 1000,
    args: ['@@'],
    tags: ['crash-discovery', 'security-testing']
  },
  
  'performance-test': {
    name: 'Performance Test Job',
    description: 'Long-running job for performance validation',
    fuzzerType: 'libfuzzer',
    duration: 900, // 15 minutes
    memoryLimit: 512 * 1024 * 1024, // 512MB
    timeout: 2000,
    args: ['@@'],
    tags: ['performance', 'load-test']
  }
};

/**
 * Test corpus data for different scenarios
 */
export const TEST_CORPUS_DATA = {
  minimal: [
    {
      filename: 'empty.txt',
      content: Buffer.alloc(0),
      description: 'Empty file'
    },
    {
      filename: 'single_byte.txt',
      content: Buffer.from([0x41]),
      description: 'Single byte'
    }
  ],
  
  normal: [
    {
      filename: 'text.txt',
      content: Buffer.from('Hello World'),
      description: 'Plain text'
    },
    {
      filename: 'numbers.txt',
      content: Buffer.from('1234567890'),
      description: 'Numeric data'
    },
    {
      filename: 'json.txt',
      content: Buffer.from('{"key": "value", "number": 42}'),
      description: 'JSON data'
    }
  ],
  
  binary: [
    {
      filename: 'png_header.dat',
      content: Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
      description: 'PNG file header'
    },
    {
      filename: 'elf_header.dat',
      content: Buffer.from([0x7f, 0x45, 0x4c, 0x46]),
      description: 'ELF file header'
    },
    {
      filename: 'random.dat',
      content: Buffer.from(Array.from({length: 256}, () => Math.floor(Math.random() * 256))),
      description: 'Random binary data'
    }
  ],
  
  vulnerability_triggers: [
    {
      filename: 'buffer_overflow.txt',
      content: Buffer.from('A'.repeat(10000)),
      description: 'Large buffer for overflow testing'
    },
    {
      filename: 'format_strings.txt',
      content: Buffer.from('%s%s%s%s%s%n%x%x%x%x'),
      description: 'Format string attacks'
    },
    {
      filename: 'null_bytes.txt',
      content: Buffer.from('\x00'.repeat(100)),
      description: 'Null byte injection'
    },
    {
      filename: 'unicode.txt',
      content: Buffer.from('💀🔥🚨\uFEFF\u202E\u200B', 'utf8'),
      description: 'Unicode edge cases'
    },
    {
      filename: 'control_chars.txt',
      content: Buffer.from('\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f'),
      description: 'Control characters'
    }
  ],
  
  structured_data: [
    {
      filename: 'xml.txt',
      content: Buffer.from('<?xml version="1.0"?><root><item>value</item></root>'),
      description: 'XML document'
    },
    {
      filename: 'base64.txt',
      content: Buffer.from('SGVsbG8gV29ybGQ='),
      description: 'Base64 encoded data'
    },
    {
      filename: 'url_encoded.txt',
      content: Buffer.from('param1=value1&param2=value%20with%20spaces'),
      description: 'URL encoded data'
    }
  ]
};

/**
 * Expected performance thresholds for different operations
 */
export const PERFORMANCE_THRESHOLDS = {
  api_response_time: 1000, // 1 second max for API calls
  job_creation_time: 5000, // 5 seconds max to create a job
  bot_registration_time: 3000, // 3 seconds max for bot registration
  campaign_start_time: 10000, // 10 seconds max to start campaign
  crash_detection_time: 30000, // 30 seconds max to detect first crash
  corpus_upload_time: 15000, // 15 seconds max for corpus upload
  memory_usage_mb: 512, // 512MB max memory usage per test
  concurrent_jobs: 10, // Max concurrent jobs for load testing
  sse_event_delay: 5000 // 5 seconds max delay for SSE events
};

/**
 * Error scenarios for failure testing
 */
export const ERROR_SCENARIOS = {
  invalid_binary_path: {
    description: 'Job with non-existent binary path',
    binary_path: '/non/existent/binary',
    expected_error: 'Binary not found'
  },
  
  memory_limit_exceeded: {
    description: 'Job with excessive memory limit',
    memory_limit: 32 * 1024 * 1024 * 1024, // 32GB
    expected_error: 'Memory limit exceeded'
  },
  
  invalid_fuzzer_type: {
    description: 'Job with invalid fuzzer type',
    fuzzer_type: 'invalid-fuzzer',
    expected_error: 'Invalid fuzzer type'
  },
  
  malformed_corpus: {
    description: 'Corpus with malformed files',
    files: [
      {
        filename: '',
        content: Buffer.alloc(0),
        description: 'Empty filename'
      }
    ],
    expected_error: 'Invalid filename'
  }
};

/**
 * Network interruption scenarios
 */
export const NETWORK_SCENARIOS = {
  temporary_disconnect: {
    description: 'Temporary network disconnection',
    duration: 10000, // 10 seconds
    expected_recovery: true
  },
  
  extended_disconnect: {
    description: 'Extended network outage',
    duration: 60000, // 1 minute
    expected_recovery: true
  },
  
  intermittent_issues: {
    description: 'Intermittent connectivity issues',
    pattern: [5000, 2000, 8000, 1000], // Connection/disconnection pattern
    expected_recovery: true
  }
};

/**
 * Multi-tenant test scenarios
 */
export const MULTI_TENANT_SCENARIOS = {
  concurrent_campaigns: {
    description: 'Multiple campaigns running simultaneously',
    campaign_count: 3,
    jobs_per_campaign: 2,
    expected_isolation: true
  },
  
  resource_competition: {
    description: 'Multiple campaigns competing for resources',
    high_priority_jobs: 2,
    low_priority_jobs: 5,
    expected_prioritization: true
  },
  
  corpus_sharing: {
    description: 'Cross-campaign corpus sharing scenarios',
    shared_collections: 2,
    private_collections: 3,
    expected_sharing_behavior: 'selective'
  }
};

/**
 * Generate test data based on scenario
 */
export function generateTestData(scenario: keyof typeof TEST_CORPUS_DATA): Array<{filename: string; content: Buffer; description: string}> {
  return [...TEST_CORPUS_DATA[scenario]];
}

/**
 * Create configuration for specific test type
 */
export function createTestConfig(type: 'campaign' | 'job', variant: string): TestCampaignConfig | TestJobConfig {
  if (type === 'campaign') {
    return { ...CAMPAIGN_CONFIGS[variant] };
  } else {
    return { ...JOB_CONFIGS[variant] };
  }
}

/**
 * Get expected thresholds for performance testing
 */
export function getPerformanceThresholds(): typeof PERFORMANCE_THRESHOLDS {
  return { ...PERFORMANCE_THRESHOLDS };
}

/**
 * Create realistic bot registration data
 */
export function createBotRegistrationData(botId: string): any {
  return {
    id: botId,
    name: `Test Bot ${botId}`,
    capabilities: {
      fuzzers: ['afl++', 'libfuzzer', 'honggfuzz'],
      max_concurrent_jobs: 2,
      supported_platforms: ['linux'],
      memory_mb: 2048,
      cpu_cores: 4
    },
    metadata: {
      version: '1.0.0',
      environment: 'test',
      created_at: new Date().toISOString()
    }
  };
}