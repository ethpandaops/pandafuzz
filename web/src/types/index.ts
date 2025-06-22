// API Types matching Go structs

export interface Bot {
  id: string;
  name: string;
  hostname: string;
  status: BotStatus;
  capabilities: string[];
  current_job?: string;
  last_seen: string;
  registered_at: string;
  ip: string;
  failure_count: number;
}

export enum BotStatus {
  Idle = 'idle',
  Busy = 'busy',
  Offline = 'offline',
  Failed = 'failed',
}

export interface Job {
  id: string;
  name: string;
  status: JobStatus;
  priority: JobPriority;
  fuzzer: string;
  target: string;
  target_args: string[];
  corpus?: string[];
  dictionary?: string;
  timeout_sec: number;
  memory_limit: number;
  assigned_bot?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  timeout_at: string;
  message?: string;
  config?: Record<string, any>;
}

export enum JobStatus {
  Pending = 'pending',
  Assigned = 'assigned',
  Running = 'running',
  Completed = 'completed',
  Failed = 'failed',
  Cancelled = 'cancelled',
}

export enum JobPriority {
  Low = 'low',
  Normal = 'normal',
  High = 'high',
}

export interface CrashResult {
  id: string;
  job_id: string;
  bot_id: string;
  timestamp: string;
  input: string; // Base64 encoded
  size: number;
  hash: string;
  type: string;
  output?: string;
  stack_trace?: string;
}

export interface CoverageResult {
  id: string;
  job_id: string;
  bot_id: string;
  timestamp: string;
  edges: number;
  covered_edges: number;
  new_edges: number;
  coverage_percent: number;
}

export interface SystemStatus {
  status: string;
  timestamp: string;
  version: string;
  uptime: number;
  database: string;
  bots: {
    total: number;
    idle: number;
    busy: number;
    offline: number;
  };
  jobs: {
    total: number;
    pending: number;
    running: number;
    completed: number;
    failed: number;
  };
}

export interface HealthCheck {
  status: string;
  timestamp: string;
  database: string;
  version?: string;
}

export interface ApiError {
  error: string;
  code?: string;
  details?: Record<string, any>;
}

// Dashboard specific types
export interface DashboardStats {
  totalBots: number;
  activeBots: number;
  totalJobs: number;
  runningJobs: number;
  totalCrashes: number;
  uniqueCrashes: number;
  averageCoverage: number;
  jobsPerHour: number;
}

export interface TimeSeriesData {
  timestamp: string;
  value: number;
}

export interface CoverageHistory {
  job_id: string;
  data: TimeSeriesData[];
}

export interface CrashTrend {
  date: string;
  crashes: number;
  unique_crashes: number;
}