import axios, { AxiosInstance } from 'axios';
import {
  Bot,
  Job,
  CrashResult,
  CoverageResult,
  SystemStatus,
  HealthCheck,
  ApiError,
  CoverageReport,
  CoverageMetadata,
  CoverageReportFilter,
} from '../types';

export class PandaFuzzAPI {
  private client: AxiosInstance;
  private v1Client: AxiosInstance; // For endpoints not yet in v3
  private baseURL: string;

  constructor(baseURL: string = '') {
    this.baseURL = baseURL || '/api/v1';
    this.client = axios.create({
      baseURL: this.baseURL,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // v1Client is now the same as client (unified API)
    this.v1Client = this.client;

    // Request interceptor for auth (if needed) - apply to both clients
    const authInterceptor = (config: any) => {
      // Add auth token if available
      const token = localStorage.getItem('auth_token');
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
      return config;
    };

    this.client.interceptors.request.use(authInterceptor, (error) => Promise.reject(error));
    this.v1Client.interceptors.request.use(authInterceptor, (error) => Promise.reject(error));

    // Response interceptor for error handling - apply to both clients
    const errorInterceptor = (error: any) => {
      if (error.response?.data) {
        const apiError: ApiError = error.response.data;
        return Promise.reject(new Error(apiError.error || 'Unknown error'));
      }
      return Promise.reject(error);
    };

    this.client.interceptors.response.use((response) => response, errorInterceptor);
    this.v1Client.interceptors.response.use((response) => response, errorInterceptor);
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
    const response = await this.client.get<{data: Bot[], pagination: any}>('/bots');
    // Handle both array response and object with data array
    if (Array.isArray(response.data)) {
      return response.data;
    }
    return response.data.data || [];
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
    sort_by?: string;
    sort_order?: 'asc' | 'desc';
  }): Promise<Job[]> {
    const response = await this.client.get<{data: Job[], pagination: any}>('/jobs', { params });
    // Handle both array response and object with data array
    if (Array.isArray(response.data)) {
      return response.data;
    }
    return response.data.data || [];
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

  // Admin endpoints
  async recoverOrphanedJobs(): Promise<{ recovered_count: number; message: string }> {
    const response = await this.client.post<{ recovered_count: number; message: string }>('/admin/recover-jobs');
    return response.data;
  }

  // Result endpoints - Note: These are likely not implemented yet
  async getCrashes(params?: {
    job_id?: string;
    limit?: number;
    offset?: number;
    sort_by?: string;
    sort_order?: 'asc' | 'desc';
  }): Promise<CrashResult[]> {
    try {
      // Use unified API for crashes
      const response = await this.client.get<{
        data: CrashResult[];
        pagination: {
          total: number;
          limit: number;
          offset: number;
        };
      }>('/crashes', {
        params,
      });
      return response.data.data || [];
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
      const activeBots = bots.filter((b) => b.is_online).length;
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
    
    // Use v1 client for logs endpoint as it's not implemented in v3 yet
    const response = await this.v1Client.get(`/jobs/${jobId}/logs?${params.toString()}`);
    return response.data;
  }

  getJobLogStreamUrl(jobId: string): string {
    return `${this.baseURL}/jobs/${jobId}/logs/stream`;
  }

  // Coverage endpoints (API v3)
  async getCoverageReports(
    jobId: string,
    filter?: CoverageReportFilter
  ): Promise<CoverageReport[]> {
    try {
      const params = {
        ...(filter?.format && { format: filter.format }),
        ...(filter?.limit && { limit: filter.limit }),
        ...(filter?.offset && { offset: filter.offset }),
        ...(filter?.sort_by && { sort_by: filter.sort_by }),
        ...(filter?.sort_order && { sort_order: filter.sort_order }),
      };

      // Use unified API endpoint for coverage
      const response = await axios.get<{reports: CoverageReport[], total: number}>(
        `/api/v1/jobs/${jobId}/coverage`,
        { 
          params,
          headers: {
            'Content-Type': 'application/json',
            ...(localStorage.getItem('auth_token') && {
              Authorization: `Bearer ${localStorage.getItem('auth_token')}`
            })
          }
        }
      );

      // Handle both array response and object with reports array
      if (Array.isArray(response.data)) {
        return response.data;
      }
      return response.data.reports || [];
    } catch (error) {
      console.warn('Coverage reports endpoint error:', error);
      return [];
    }
  }

  async getCoverageMetadata(
    jobId: string,
    reportId: string
  ): Promise<CoverageMetadata> {
    const response = await axios.get<CoverageMetadata>(
      `/api/v1/jobs/${jobId}/coverage/${reportId}/metadata`,
      {
        headers: {
          'Content-Type': 'application/json',
          ...(localStorage.getItem('auth_token') && {
            Authorization: `Bearer ${localStorage.getItem('auth_token')}`
          })
        }
      }
    );
    return response.data;
  }

  async downloadCoverageReport(
    jobId: string,
    reportId: string
  ): Promise<Blob> {
    const response = await axios.get(
      `/api/v1/jobs/${jobId}/coverage/${reportId}`,
      {
        responseType: 'blob',
        headers: {
          'Accept': 'application/json, text/html, text/plain, application/octet-stream',
          ...(localStorage.getItem('auth_token') && {
            Authorization: `Bearer ${localStorage.getItem('auth_token')}`
          })
        },
      }
    );
    return response.data;
  }

  // Helper method for coverage statistics
  async getCoverageStats(jobId: string): Promise<any> {
    try {
      const reports = await this.getCoverageReports(jobId);
      
      if (reports.length === 0) {
        return {
          line_coverage: 0,
          function_coverage: 0,
          branch_coverage: 0,
          total_reports: 0,
        };
      }

      // Get metadata for the most recent report
      const latestReport = reports[0]; // Assuming sorted by created_at DESC
      try {
        const metadata = await this.getCoverageMetadata(jobId, latestReport.id);
        return {
          line_coverage: metadata.line_coverage || 0,
          function_coverage: metadata.function_coverage || 0,
          branch_coverage: metadata.branch_coverage || 0,
          total_lines: metadata.total_lines || 0,
          covered_lines: metadata.covered_lines || 0,
          total_functions: metadata.total_functions || 0,
          covered_functions: metadata.covered_functions || 0,
          total_reports: reports.length,
          latest_report_id: latestReport.id,
          latest_report_date: latestReport.created_at,
        };
      } catch (metadataError) {
        console.warn('Failed to fetch coverage metadata:', metadataError);
        return {
          line_coverage: 0,
          function_coverage: 0,
          branch_coverage: 0,
          total_reports: reports.length,
        };
      }
    } catch (error) {
      console.warn('Failed to fetch coverage stats:', error);
      return {
        line_coverage: 0,
        function_coverage: 0,
        branch_coverage: 0,
        total_reports: 0,
      };
    }
  }
}

// Create singleton instance
const api = new PandaFuzzAPI();
export default api;