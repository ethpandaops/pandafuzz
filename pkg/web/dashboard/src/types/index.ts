// API Types matching the backend structures

export interface Campaign {
  id: string;
  name: string;
  target_binary: string;
  fuzzer: string;
  status: 'active' | 'paused' | 'completed';
  created_at: string;
  updated_at: string;
}

export interface CampaignStats {
  campaign_id: string;
  total_jobs: number;
  active_jobs: number;
  completed_jobs: number;
  total_crashes: number;
  unique_crashes: number;
  total_coverage: number;
  corpus_size: number;
  execution_count: number;
  exec_per_second: number;
}

export interface CoveragePoint {
  timestamp: string;
  total_edges: number;
  new_edges: number;
  exec_count: number;
  exec_per_sec: number;
  corpus_size: number;
  corpus_bytes: number;
}

export interface CoverageTrend {
  campaign_id: string;
  interval: string;
  start_time: string;
  end_time: string;
  data_points: CoveragePoint[];
  total_growth: number;
  growth_rate: number;
}

export interface CrashRateMetrics {
  campaign_id: string;
  window: string;
  total_crashes: number;
  unique_crashes: number;
  crash_rate: number;
  unique_crash_rate: number;
  trend: 'increasing' | 'decreasing' | 'stable';
  trend_confidence: number;
}

export interface FuzzerPerformance {
  fuzzer_type: string;
  window: string;
  total_jobs: number;
  successful_jobs: number;
  failed_jobs: number;
  average_runtime: string;
  total_exec_count: number;
  average_exec_speed: number;
  coverage_gain: number;
  crashes_found: number;
  efficiency_score: number;
}

export interface RealtimeMetrics {
  campaign_id: string;
  timestamp: string;
  exec_per_second: number;
  current_coverage: number;
  recent_crashes: number;
  active_bots: number;
  queue_length: number;
  memory_usage: number;
  cpu_usage: number;
}

export interface Job {
  id: string;
  name: string;
  campaign_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  fuzzer: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  bot_id?: string;
}

export interface Bot {
  id: string;
  name: string;
  hostname: string;
  status: 'idle' | 'busy' | 'offline';
  current_job_id?: string;
  capabilities: string[];
  last_heartbeat: string;
}