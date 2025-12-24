import axios, { AxiosInstance } from 'axios';
import {
  Campaign,
  CampaignStats,
  CoverageTrend,
  CrashRateMetrics,
  FuzzerPerformance,
  RealtimeMetrics,
  Job,
  Bot
} from '../types';

class ApiService {
  private client: AxiosInstance;

  constructor(baseURL: string = '/api/v1', apiKey?: string) {
    this.client = axios.create({
      baseURL,
      headers: {
        'Content-Type': 'application/json',
        ...(apiKey && { 'X-API-Key': apiKey })
      }
    });

    // Add request/response interceptors for error handling
    this.client.interceptors.response.use(
      response => response,
      error => {
        console.error('API Error:', error);
        return Promise.reject(error);
      }
    );
  }

  // Campaign endpoints
  async getCampaigns(): Promise<Campaign[]> {
    const response = await this.client.get<Campaign[]>('/campaigns');
    return response.data;
  }

  async getCampaign(id: string): Promise<Campaign> {
    const response = await this.client.get<Campaign>(`/campaigns/${id}`);
    return response.data;
  }

  async getCampaignStats(id: string): Promise<CampaignStats> {
    const response = await this.client.get<CampaignStats>(`/campaigns/${id}/stats`);
    return response.data;
  }

  // Analytics endpoints
  async getCoverageTrend(campaignId: string, period: string = '24h'): Promise<CoverageTrend> {
    const response = await this.client.get<CoverageTrend>('/analytics/coverage-trend', {
      params: { campaign_id: campaignId, period }
    });
    return response.data;
  }

  async getCrashRate(campaignId: string, window: string = '24h'): Promise<CrashRateMetrics> {
    const response = await this.client.get<CrashRateMetrics>('/analytics/crash-rate', {
      params: { campaign_id: campaignId, window }
    });
    return response.data;
  }

  async getFuzzerPerformance(fuzzerType: string, window: string = '24h'): Promise<FuzzerPerformance> {
    const response = await this.client.get<FuzzerPerformance>('/analytics/fuzzer-performance', {
      params: { fuzzer_type: fuzzerType, window }
    });
    return response.data;
  }

  async getRealtimeMetrics(campaignId: string): Promise<RealtimeMetrics> {
    const response = await this.client.get<RealtimeMetrics>('/analytics/realtime', {
      params: { campaign_id: campaignId }
    });
    return response.data;
  }

  // Job endpoints
  async getJobs(campaignId?: string, status?: string): Promise<Job[]> {
    const response = await this.client.get<Job[]>('/jobs', {
      params: { campaign_id: campaignId, status }
    });
    return response.data;
  }

  async getJob(id: string): Promise<Job> {
    const response = await this.client.get<Job>(`/jobs/${id}`);
    return response.data;
  }

  // Bot endpoints
  async getBots(status?: string): Promise<Bot[]> {
    const response = await this.client.get<Bot[]>('/bots', {
      params: { status }
    });
    return response.data;
  }

  async getBot(id: string): Promise<Bot> {
    const response = await this.client.get<Bot>(`/bots/${id}`);
    return response.data;
  }

  // WebSocket connection for real-time updates
  connectWebSocket(onMessage: (data: any) => void): WebSocket {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        onMessage(data);
      } catch (error) {
        console.error('WebSocket message error:', error);
      }
    };

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    return ws;
  }
}

export default ApiService;