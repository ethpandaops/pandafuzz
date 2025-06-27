import axios, { AxiosInstance } from 'axios';
import {
  Bot,
  Job,
  CrashResult,
  CoverageResult,
  SystemStatus,
  HealthCheck,
  ApiError,
} from '../types';

export class PandaFuzzAPI {
  private client: AxiosInstance;
  private baseURL: string;

  constructor(baseURL: string = '') {
    this.baseURL = baseURL || '/api/v1';
    this.client = axios.create({
      baseURL: this.baseURL,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Request interceptor for auth (if needed)
    this.client.interceptors.request.use(
      (config) => {
        // Add auth token if available
        const token = localStorage.getItem('auth_token');
        if (token) {
          config.headers.Authorization = `Bearer ${token}`;
        }
        return config;
      },
      (error) => Promise.reject(error)
    );

    // Response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      (error) => {
        if (error.response?.data) {
          const apiError: ApiError = error.response.data;
          return Promise.reject(new Error(apiError.error || 'Unknown error'));
        }
        return Promise.reject(error);
      }
    );
  }

  // System endpoints
  async getHealth(): Promise<HealthCheck> {
    // Health endpoint is at root level, not under /api/v1
    const response = await this.client.get<HealthCheck>('/../../health');
    return response.data;
  }

  async getSystemStats(): Promise<any> {
    const response = await this.client.get('/system/stats');
    return response.data;
  }

  // Bot endpoints
  async getBots(): Promise<Bot[]> {
    const response = await this.client.get<{bots: Bot[], count: number}>('/bots');
    // Handle both array response and object with bots array
    if (Array.isArray(response.data)) {
      return response.data;
    }
    return response.data.bots || [];
  }

  async getBot(id: string): Promise<Bot> {
    const response = await this.client.get<Bot>(`/bots/${id}`);
    return response.data;
  }

  async deleteBot(id: string): Promise<void> {
    await this.client.delete(`/bots/${id}`);
  }

  // Job endpoints
  async getJobs(params?: {
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<Job[]> {
    const response = await this.client.get<{jobs: Job[], count: number, total: number}>('/jobs', { params });
    // Handle both array response and object with jobs array
    if (Array.isArray(response.data)) {
      return response.data;
    }
    return response.data.jobs || [];
  }

  async getJob(id: string): Promise<Job> {
    const response = await this.client.get<Job>(`/jobs/${id}`);
    return response.data;
  }

  async createJob(job: Partial<Job>): Promise<Job> {
    const response = await this.client.post<Job>('/jobs', job);
    return response.data;
  }

  async createJobWithUpload(jobData: Partial<Job>, targetBinary: File, seedCorpus?: File[]): Promise<Job> {
    const formData = new FormData();
    
    // Add job metadata as JSON
    formData.append('job_metadata', JSON.stringify(jobData));
    
    // Add target binary
    formData.append('target_binary', targetBinary);
    
    // Add seed corpus files if provided
    if (seedCorpus && seedCorpus.length > 0) {
      seedCorpus.forEach((file) => {
        formData.append('seed_corpus', file);
      });
    }
    
    const response = await this.client.post<Job>('/jobs/upload', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  }

  async cancelJob(id: string): Promise<void> {
    await this.client.put(`/jobs/${id}/cancel`);
  }

  async deleteJob(id: string): Promise<void> {
    await this.client.delete(`/jobs/${id}`);
  }

  // Result endpoints - Note: These are likely not implemented yet
  async getCrashes(params?: {
    job_id?: string;
    limit?: number;
    offset?: number;
  }): Promise<CrashResult[]> {
    try {
      const response = await this.client.get<{
        crashes: CrashResult[];
        count: number;
        limit: number;
        offset: number;
      }>('/results/crashes', {
        params,
      });
      return response.data.crashes || [];
    } catch (error) {
      // Return empty array if endpoint doesn't exist
      console.warn('Crashes endpoint error:', error);
      return [];
    }
  }

  async getCoverageResults(params?: {
    job_id?: string;
    limit?: number;
    offset?: number;
  }): Promise<CoverageResult[]> {
    try {
      const response = await this.client.get<CoverageResult[]>(
        '/results/coverage',
        { params }
      );
      return response.data;
    } catch (error) {
      // Return empty array if endpoint doesn't exist
      console.warn('Coverage endpoint not implemented, returning empty array');
      return [];
    }
  }

  // Dashboard specific endpoints
  async getDashboardStats(): Promise<any> {
    try {
      // Get system stats from the actual endpoint
      const stats = await this.getSystemStats();
      
      // Get bots and jobs
      const [bots, jobs] = await Promise.all([
        this.getBots(),
        this.getJobs({ limit: 100 }),
      ]);

      // Get crashes if endpoint exists
      let crashes: CrashResult[] = [];
      try {
        crashes = await this.getCrashes({ limit: 100 });
      } catch (e) {
        // Ignore if crashes endpoint doesn't exist
      }

      // Calculate stats
      const activeBots = bots.filter((b) => b.status !== 'offline').length;
      const runningJobs = jobs.filter((j) => j.status === 'running').length;
      const uniqueCrashes = new Set(crashes.map((c) => c.hash)).size;

      return {
        totalBots: bots.length,
        activeBots,
        totalJobs: jobs.length,
        runningJobs,
        totalCrashes: crashes.length,
        uniqueCrashes,
        averageCoverage: 0, // Calculate from coverage results if available
        jobsPerHour: 0, // Calculate from job timestamps
      };
    } catch (error) {
      console.error('Error fetching dashboard stats:', error);
      throw error;
    }
  }

  // Corpus management endpoints
  async getJobCorpus(jobId: string): Promise<{job_id: string, files: any[]}> {
    const response = await this.client.get(`/jobs/${jobId}/corpus`);
    return response.data;
  }

  async uploadJobCorpus(jobId: string, formData: FormData): Promise<any> {
    const response = await this.client.post(`/jobs/${jobId}/corpus`, formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  }

  async getJobCorpusStats(jobId: string): Promise<any> {
    const response = await this.client.get(`/jobs/${jobId}/corpus/stats`);
    return response.data;
  }

  async downloadCorpusFile(jobId: string, filename: string): Promise<Blob> {
    const response = await this.client.get(`/jobs/${jobId}/corpus/${filename}`, {
      responseType: 'blob',
    });
    return response.data;
  }

  async deleteCorpusFile(jobId: string, filename: string): Promise<void> {
    await this.client.delete(`/jobs/${jobId}/corpus/${filename}`);
  }

  // Job logs endpoints
  async getJobLogs(jobId: string, limit?: number, offset?: number): Promise<any> {
    const params = new URLSearchParams();
    if (limit) params.append('limit', limit.toString());
    if (offset) params.append('offset', offset.toString());
    
    const response = await this.client.get(`/jobs/${jobId}/logs?${params.toString()}`);
    return response.data;
  }

  getJobLogStreamUrl(jobId: string): string {
    return `${this.baseURL}/jobs/${jobId}/logs/stream`;
  }
}

// Create singleton instance
const api = new PandaFuzzAPI();
export default api;