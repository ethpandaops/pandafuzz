// Coverage types matching the OpenAPI specification and Go structs

export interface CoverageReport {
  id: string;
  job_id: string;
  format: CoverageFormat;
  size: number;
  created_at: string;
  storage_path?: string;
  checksum?: string;
  // Additional fields from v1 coverage table
  bot_id?: string;
  edges?: number;
  new_edges?: number;
  timestamp?: string;
  exec_count?: number;
  // Added for UI context
  jobName?: string;
  jobId?: string;
}

export interface CoverageMetadata {
  id?: number;
  report_id: string;
  job_id?: string;
  line_coverage?: number;
  function_coverage?: number;
  branch_coverage?: number;
  total_lines?: number;
  covered_lines?: number;
  total_functions?: number;
  covered_functions?: number;
  collected_at: string;
}

export type CoverageFormat = 'json' | 'html' | 'lcov' | 'cobertura' | 'raw';

// API Response types
export interface CoverageReportListResponse {
  reports: CoverageReport[];
  total: number;
  limit: number;
  offset: number;
}

export interface CoverageSubmitResponse {
  coverage_id: string;
  message?: string;
}

// Coverage statistics and analysis types
export interface CoverageStats {
  line_coverage_percent: number;
  function_coverage_percent: number;
  branch_coverage_percent: number;
  total_lines: number;
  covered_lines: number;
  total_functions: number;
  covered_functions: number;
}

export interface CoverageTrend {
  timestamp: string;
  line_coverage: number;
  function_coverage: number;
  branch_coverage: number;
}

// Filter and query types
export interface CoverageReportFilter {
  job_id?: string;
  format?: CoverageFormat;
  limit?: number;
  offset?: number;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
}