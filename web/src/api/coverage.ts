import axios, { AxiosInstance } from 'axios';
import {
  CoverageReport,
  CoverageMetadata,
  CoverageReportListResponse,
  CoverageReportFilter,
} from '../types/coverage';
import { ApiError } from '../types';

export class CoverageAPIClient {
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

    // Request interceptor for auth
    this.client.interceptors.request.use(
      (config) => {
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

  /**
   * Fetch coverage reports for a specific job
   * @param jobId - The job ID to fetch coverage reports for
   * @param filter - Optional filter parameters
   * @returns Promise with array of coverage reports
   */
  async fetchCoverageReports(
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

      const response = await this.client.get<CoverageReportListResponse>(
        `/jobs/${jobId}/coverage`,
        { params }
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

  /**
   * Fetch coverage metadata for a specific report
   * @param jobId - The job ID
   * @param reportId - The coverage report ID
   * @returns Promise with coverage metadata
   */
  async fetchCoverageMetadata(
    jobId: string,
    reportId: string
  ): Promise<CoverageMetadata> {
    const response = await this.client.get<CoverageMetadata>(
      `/jobs/${jobId}/coverage/${reportId}/metadata`
    );
    return response.data;
  }

  /**
   * Download a coverage report as a blob
   * @param jobId - The job ID
   * @param reportId - The coverage report ID
   * @returns Promise with the report file as a Blob
   */
  async downloadCoverageReport(
    jobId: string,
    reportId: string
  ): Promise<Blob> {
    const response = await this.client.get(
      `/jobs/${jobId}/coverage/${reportId}`,
      {
        responseType: 'blob',
        headers: {
          'Accept': 'application/json, text/html, text/plain, application/octet-stream',
        },
      }
    );
    return response.data;
  }

  /**
   * Get coverage statistics for a job (aggregated from all reports)
   * @param jobId - The job ID
   * @returns Promise with coverage statistics
   */
  async getCoverageStats(jobId: string): Promise<any> {
    try {
      const reports = await this.fetchCoverageReports(jobId);
      
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
        const metadata = await this.fetchCoverageMetadata(jobId, latestReport.id);
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

  /**
   * Get coverage trends over time for a job
   * @param jobId - The job ID
   * @param limit - Maximum number of data points to return
   * @returns Promise with coverage trend data
   */
  async getCoverageTrends(
    jobId: string,
    limit: number = 20
  ): Promise<any[]> {
    try {
      const reports = await this.fetchCoverageReports(jobId, { 
        limit,
        sort_by: 'created_at',
        sort_order: 'desc'
      });

      const trends = await Promise.all(
        reports.map(async (report) => {
          try {
            const metadata = await this.fetchCoverageMetadata(jobId, report.id);
            return {
              timestamp: report.created_at,
              report_id: report.id,
              line_coverage: metadata.line_coverage || 0,
              function_coverage: metadata.function_coverage || 0,
              branch_coverage: metadata.branch_coverage || 0,
            };
          } catch (error) {
            console.warn(`Failed to fetch metadata for report ${report.id}:`, error);
            return {
              timestamp: report.created_at,
              report_id: report.id,
              line_coverage: 0,
              function_coverage: 0,
              branch_coverage: 0,
            };
          }
        })
      );

      // Sort by timestamp ascending for proper trend visualization
      return trends.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    } catch (error) {
      console.warn('Failed to fetch coverage trends:', error);
      return [];
    }
  }
}

// Create singleton instance
const coverageAPI = new CoverageAPIClient();
export default coverageAPI;