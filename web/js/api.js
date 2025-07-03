// PandaFuzz API Client
class PandaFuzzAPI {
    constructor(baseURL = '/api/v2') {
        this.baseURL = baseURL;
        this.headers = {
            'Content-Type': 'application/json',
        };
    }

    // Helper method for API requests
    async request(method, endpoint, body = null) {
        const options = {
            method: method,
            headers: this.headers,
        };

        if (body && method !== 'GET') {
            options.body = JSON.stringify(body);
        }

        try {
            const response = await fetch(`${this.baseURL}${endpoint}`, options);
            
            if (!response.ok) {
                const error = await response.json().catch(() => ({ error: response.statusText }));
                throw new Error(error.error || `HTTP ${response.status}: ${response.statusText}`);
            }

            const contentType = response.headers.get('content-type');
            if (contentType && contentType.includes('application/json')) {
                return await response.json();
            }
            
            return response;
        } catch (error) {
            console.error('API request failed:', error);
            throw error;
        }
    }

    // Campaign endpoints
    async createCampaign(campaign) {
        return this.request('POST', '/campaigns', campaign);
    }

    async listCampaigns(filters = {}) {
        const params = new URLSearchParams(filters).toString();
        return this.request('GET', `/campaigns${params ? '?' + params : ''}`);
    }

    async getCampaign(id) {
        return this.request('GET', `/campaigns/${id}`);
    }

    async updateCampaign(id, updates) {
        return this.request('PATCH', `/campaigns/${id}`, updates);
    }

    async deleteCampaign(id) {
        return this.request('DELETE', `/campaigns/${id}`);
    }

    async restartCampaign(id) {
        return this.request('POST', `/campaigns/${id}/restart`);
    }

    async getCampaignStats(id) {
        return this.request('GET', `/campaigns/${id}/stats`);
    }

    async getCampaignTimeline(id) {
        return this.request('GET', `/campaigns/${id}/timeline`);
    }

    async uploadCampaignBinary(id, file) {
        const formData = new FormData();
        formData.append('binary', file);

        const response = await fetch(`${this.baseURL}/campaigns/${id}/binary`, {
            method: 'POST',
            body: formData,
        });

        if (!response.ok) {
            throw new Error(`Upload failed: ${response.statusText}`);
        }

        return response.json();
    }

    async uploadCampaignCorpus(id, files) {
        const formData = new FormData();
        for (const file of files) {
            formData.append('corpus', file);
        }

        const response = await fetch(`${this.baseURL}/campaigns/${id}/corpus`, {
            method: 'POST',
            body: formData,
        });

        if (!response.ok) {
            throw new Error(`Upload failed: ${response.statusText}`);
        }

        return response.json();
    }

    // Crash analysis endpoints
    async getCrashGroups(campaignId, filters = {}) {
        const params = new URLSearchParams(filters).toString();
        return this.request('GET', `/campaigns/${campaignId}/crashes${params ? '?' + params : ''}`);
    }

    async getStackTrace(crashId) {
        return this.request('GET', `/crashes/${crashId}/stacktrace`);
    }

    async getCrashInput(crashId) {
        const response = await this.request('GET', `/crashes/${crashId}/input`);
        return response.blob();
    }

    // Corpus endpoints
    async getCorpusEvolution(campaignId) {
        return this.request('GET', `/campaigns/${campaignId}/corpus/evolution`);
    }

    async syncCorpus(campaignId, botId) {
        return this.request('POST', `/campaigns/${campaignId}/corpus/sync`, { bot_id: botId });
    }

    async shareCorpus(fromCampaignId, toCampaignId) {
        return this.request('POST', `/campaigns/${fromCampaignId}/corpus/share`, { 
            to_campaign_id: toCampaignId 
        });
    }

    async listCorpusFiles(campaignId, filters = {}) {
        const params = new URLSearchParams(filters).toString();
        return this.request('GET', `/campaigns/${campaignId}/corpus/files${params ? '?' + params : ''}`);
    }

    async downloadCorpusFile(campaignId, hash) {
        const response = await this.request('GET', `/campaigns/${campaignId}/corpus/files/${hash}`);
        return response.blob();
    }

    // Bot endpoints
    async listBots(filters = {}) {
        const params = new URLSearchParams(filters).toString();
        return this.request('GET', `/bots${params ? '?' + params : ''}`);
    }

    async getBotMetrics(botId) {
        return this.request('GET', `/bots/${botId}/metrics`);
    }

    async getBotMetricsHistory(botId, duration = '1h') {
        return this.request('GET', `/bots/${botId}/metrics/history?duration=${duration}`);
    }

    // Job endpoints
    async createJob(job, campaignId = null) {
        const params = campaignId ? `?campaign_id=${campaignId}` : '';
        return this.request('POST', `/jobs${params}`, job);
    }

    async listJobs(filters = {}) {
        const params = new URLSearchParams(filters).toString();
        return this.request('GET', `/jobs${params ? '?' + params : ''}`);
    }

    async getJob(id) {
        return this.request('GET', `/jobs/${id}`);
    }

    async cancelJob(id) {
        return this.request('POST', `/jobs/${id}/cancel`);
    }

    async streamJobProgress(id, onProgress) {
        const eventSource = new EventSource(`${this.baseURL}/jobs/${id}/progress`);
        
        eventSource.addEventListener('progress', (event) => {
            const data = JSON.parse(event.data);
            onProgress(data);
        });

        eventSource.addEventListener('completed', (event) => {
            const data = JSON.parse(event.data);
            onProgress(data);
            eventSource.close();
        });

        eventSource.addEventListener('error', (event) => {
            console.error('Job progress stream error:', event);
            eventSource.close();
        });

        return eventSource;
    }

    // System endpoints
    async getSystemStatus() {
        return this.request('GET', '/status');
    }

    async getSystemStats() {
        return this.request('GET', '/system/stats');
    }

    async triggerMaintenance(type, target = null, force = false) {
        return this.request('POST', '/system/maintenance', {
            type: type,
            target: target,
            force: force,
        });
    }

    // Batch operations
    async submitBatchResults(botId, jobId, results) {
        return this.request('POST', '/batch/results', {
            bot_id: botId,
            job_id: jobId,
            crashes: results.crashes || [],
            coverage: results.coverage || [],
            corpus: results.corpus || [],
        });
    }

    // Health check
    async health() {
        return this.request('GET', '/health');
    }
}

// Export for use in other scripts
window.PandaFuzzAPI = PandaFuzzAPI;