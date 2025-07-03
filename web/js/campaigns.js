// Campaigns Manager
class CampaignsManager {
    constructor() {
        this.api = new PandaFuzzAPI();
        this.ws = new PandaFuzzWebSocket();
        this.campaigns = [];
        this.currentPage = 1;
        this.itemsPerPage = 20;
        this.totalPages = 1;
        this.filters = {
            status: '',
            search: ''
        };
        this.currentCampaign = null;
    }

    async init() {
        console.log('Initializing campaigns page...');
        
        // Set up WebSocket
        this.setupWebSocket();
        
        // Set up event handlers
        this.setupEventHandlers();
        
        // Load initial data
        await this.loadCampaigns();
        
        // Connect WebSocket
        this.ws.connect();
    }

    setupWebSocket() {
        // Connection status handler
        this.ws.onConnectionStatusChange((connected) => {
            this.updateConnectionStatus(connected);
        });

        // Campaign events
        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_CREATED, (data) => {
            this.handleCampaignCreated(data);
        });

        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_UPDATED, (data) => {
            this.handleCampaignUpdated(data);
        });

        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_COMPLETED, (data) => {
            this.handleCampaignCompleted(data);
        });

        // Metrics updates
        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_METRICS_UPDATE, (data) => {
            this.updateCampaignMetrics(data);
        });
    }

    setupEventHandlers() {
        // Create campaign button
        document.getElementById('create-campaign-btn')?.addEventListener('click', () => {
            this.showCreateModal();
        });

        // Modal controls
        document.getElementById('close-modal')?.addEventListener('click', () => {
            this.hideCreateModal();
        });

        document.getElementById('cancel-create')?.addEventListener('click', () => {
            this.hideCreateModal();
        });

        document.getElementById('close-details-modal')?.addEventListener('click', () => {
            this.hideDetailsModal();
        });

        // Form submission
        document.getElementById('create-campaign-form')?.addEventListener('submit', (e) => {
            e.preventDefault();
            this.createCampaign();
        });

        // Filters
        document.getElementById('status-filter')?.addEventListener('change', (e) => {
            this.filters.status = e.target.value;
            this.currentPage = 1;
            this.filterCampaigns();
        });

        document.getElementById('search-campaigns')?.addEventListener('input', (e) => {
            this.filters.search = e.target.value.toLowerCase();
            this.currentPage = 1;
            this.filterCampaigns();
        });

        // Refresh button
        document.getElementById('refresh-campaigns-btn')?.addEventListener('click', () => {
            this.loadCampaigns();
        });

        // Pagination
        document.getElementById('prev-page')?.addEventListener('click', () => {
            if (this.currentPage > 1) {
                this.currentPage--;
                this.renderCampaigns();
            }
        });

        document.getElementById('next-page')?.addEventListener('click', () => {
            if (this.currentPage < this.totalPages) {
                this.currentPage++;
                this.renderCampaigns();
            }
        });

        // API docs link
        document.getElementById('api-docs-link')?.addEventListener('click', (e) => {
            e.preventDefault();
            window.open('/api/docs', '_blank');
        });
    }

    async loadCampaigns() {
        try {
            this.showLoading(true);
            const result = await this.api.listCampaigns({ 
                limit: 1000, // Get all for client-side filtering
                status: this.filters.status || undefined
            });
            
            this.campaigns = result.campaigns || [];
            this.updateStatistics();
            this.filterCampaigns();
            
        } catch (error) {
            console.error('Failed to load campaigns:', error);
            this.showError('Failed to load campaigns');
        } finally {
            this.showLoading(false);
        }
    }

    filterCampaigns() {
        let filtered = this.campaigns;
        
        // Apply status filter
        if (this.filters.status) {
            filtered = filtered.filter(c => c.status === this.filters.status);
        }
        
        // Apply search filter
        if (this.filters.search) {
            filtered = filtered.filter(c => 
                c.name.toLowerCase().includes(this.filters.search) ||
                c.description?.toLowerCase().includes(this.filters.search) ||
                c.target_binary.toLowerCase().includes(this.filters.search) ||
                c.tags?.some(tag => tag.toLowerCase().includes(this.filters.search))
            );
        }
        
        // Calculate pagination
        this.totalPages = Math.ceil(filtered.length / this.itemsPerPage);
        if (this.currentPage > this.totalPages) {
            this.currentPage = Math.max(1, this.totalPages);
        }
        
        // Get current page items
        const start = (this.currentPage - 1) * this.itemsPerPage;
        const pageItems = filtered.slice(start, start + this.itemsPerPage);
        
        this.renderCampaigns(pageItems);
        this.updatePagination();
    }

    renderCampaigns(campaigns = null) {
        const tbody = document.getElementById('campaigns-tbody');
        if (!tbody) return;
        
        const toRender = campaigns || this.campaigns;
        
        if (toRender.length === 0) {
            tbody.innerHTML = '<tr><td colspan="8" class="empty-state">No campaigns found</td></tr>';
            return;
        }
        
        tbody.innerHTML = toRender.map(campaign => {
            const createdDate = new Date(campaign.created_at).toLocaleDateString();
            const statusClass = `status-${campaign.status}`;
            
            // Get stats (would be populated from real data)
            const stats = {
                jobs: campaign.jobs_count || 0,
                coverage: campaign.total_coverage || 0,
                crashes: campaign.crashes_count || 0
            };
            
            return `
                <tr>
                    <td>
                        <div class="campaign-name">${this.escapeHtml(campaign.name)}</div>
                        ${campaign.description ? `<div class="campaign-desc">${this.escapeHtml(campaign.description)}</div>` : ''}
                    </td>
                    <td><span class="badge ${statusClass}">${campaign.status}</span></td>
                    <td class="monospace">${this.escapeHtml(campaign.target_binary)}</td>
                    <td>${stats.jobs}</td>
                    <td>${this.formatNumber(stats.coverage)}</td>
                    <td>${stats.crashes}</td>
                    <td>${createdDate}</td>
                    <td>
                        <div class="action-buttons">
                            <button class="btn btn-sm" onclick="window.campaignsManager.viewCampaign('${campaign.id}')">View</button>
                            ${this.getActionButtons(campaign)}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }

    getActionButtons(campaign) {
        const buttons = [];
        
        switch (campaign.status) {
            case 'running':
                buttons.push(`<button class="btn btn-sm" onclick="window.campaignsManager.pauseCampaign('${campaign.id}')">Pause</button>`);
                break;
            case 'paused':
                buttons.push(`<button class="btn btn-sm" onclick="window.campaignsManager.resumeCampaign('${campaign.id}')">Resume</button>`);
                break;
            case 'completed':
            case 'failed':
                if (campaign.auto_restart) {
                    buttons.push(`<button class="btn btn-sm" onclick="window.campaignsManager.restartCampaign('${campaign.id}')">Restart</button>`);
                }
                break;
            case 'pending':
                buttons.push(`<button class="btn btn-sm" onclick="window.campaignsManager.startCampaign('${campaign.id}')">Start</button>`);
                break;
        }
        
        if (campaign.status !== 'running') {
            buttons.push(`<button class="btn btn-sm btn-danger" onclick="window.campaignsManager.deleteCampaign('${campaign.id}')">Delete</button>`);
        }
        
        return buttons.join(' ');
    }

    updateStatistics() {
        const totalCampaigns = this.campaigns.length;
        const activeCampaigns = this.campaigns.filter(c => c.status === 'running').length;
        
        document.getElementById('total-campaigns').textContent = totalCampaigns;
        document.getElementById('active-campaigns-count').textContent = activeCampaigns;
    }

    updatePagination() {
        document.getElementById('current-page').textContent = this.currentPage;
        document.getElementById('total-pages').textContent = this.totalPages;
        
        const prevBtn = document.getElementById('prev-page');
        const nextBtn = document.getElementById('next-page');
        
        prevBtn.disabled = this.currentPage <= 1;
        nextBtn.disabled = this.currentPage >= this.totalPages;
    }

    showCreateModal() {
        document.getElementById('create-campaign-modal').style.display = 'block';
        document.getElementById('create-campaign-form').reset();
    }

    hideCreateModal() {
        document.getElementById('create-campaign-modal').style.display = 'none';
    }

    async createCampaign() {
        const form = document.getElementById('create-campaign-form');
        const formData = new FormData(form);
        
        const campaign = {
            name: document.getElementById('campaign-name').value,
            description: document.getElementById('campaign-description').value,
            target_binary: document.getElementById('target-binary').value,
            max_jobs: parseInt(document.getElementById('max-jobs').value),
            auto_restart: document.getElementById('auto-restart').checked,
            shared_corpus: document.getElementById('shared-corpus').checked,
            job_template: {
                duration: parseInt(document.getElementById('job-duration').value) * 3600, // Convert to seconds
                memory_limit: parseInt(document.getElementById('memory-limit').value) * 1024 * 1024, // Convert to bytes
                timeout: parseInt(document.getElementById('timeout').value),
                fuzzer_type: document.getElementById('fuzzer-type').value
            },
            tags: document.getElementById('campaign-tags').value
                .split(',')
                .map(tag => tag.trim())
                .filter(tag => tag.length > 0)
        };
        
        try {
            this.showLoading(true);
            const result = await this.api.createCampaign(campaign);
            
            this.hideCreateModal();
            this.showToast('Campaign created successfully');
            
            // Start immediately if requested
            if (document.getElementById('start-immediately').checked) {
                await this.startCampaign(result.campaign.id);
            }
            
            // Reload campaigns
            await this.loadCampaigns();
            
        } catch (error) {
            console.error('Failed to create campaign:', error);
            this.showError('Failed to create campaign: ' + error.message);
        } finally {
            this.showLoading(false);
        }
    }

    async viewCampaign(campaignId) {
        try {
            const result = await this.api.getCampaign(campaignId);
            const campaign = result.campaign;
            this.currentCampaign = campaign;
            
            const stats = await this.api.getCampaignStats(campaignId);
            this.showCampaignDetails(campaign, stats);
            
        } catch (error) {
            console.error('Failed to load campaign details:', error);
            this.showError('Failed to load campaign details');
        }
    }

    showCampaignDetails(campaign, stats) {
        const content = document.getElementById('campaign-details-content');
        const statusClass = `status-${campaign.status}`;
        
        content.innerHTML = `
            <div class="campaign-details">
                <div class="details-section">
                    <h3>${this.escapeHtml(campaign.name)}</h3>
                    <p class="description">${this.escapeHtml(campaign.description || 'No description')}</p>
                    <div class="tags">
                        ${campaign.tags?.map(tag => `<span class="tag">${this.escapeHtml(tag)}</span>`).join('') || ''}
                    </div>
                </div>
                
                <div class="details-grid">
                    <div class="detail-item">
                        <span class="detail-label">Status:</span>
                        <span class="badge ${statusClass}">${campaign.status}</span>
                    </div>
                    <div class="detail-item">
                        <span class="detail-label">Target Binary:</span>
                        <span class="monospace">${this.escapeHtml(campaign.target_binary)}</span>
                    </div>
                    <div class="detail-item">
                        <span class="detail-label">Binary Hash:</span>
                        <span class="monospace">${campaign.binary_hash?.substring(0, 8) || 'N/A'}</span>
                    </div>
                    <div class="detail-item">
                        <span class="detail-label">Created:</span>
                        <span>${new Date(campaign.created_at).toLocaleString()}</span>
                    </div>
                    <div class="detail-item">
                        <span class="detail-label">Auto Restart:</span>
                        <span>${campaign.auto_restart ? 'Yes' : 'No'}</span>
                    </div>
                    <div class="detail-item">
                        <span class="detail-label">Shared Corpus:</span>
                        <span>${campaign.shared_corpus ? 'Yes' : 'No'}</span>
                    </div>
                </div>
                
                <div class="stats-section">
                    <h4>Statistics</h4>
                    <div class="stats-grid">
                        <div class="stat-card">
                            <div class="stat-value">${stats.total_jobs || 0}</div>
                            <div class="stat-label">Total Jobs</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${stats.active_jobs || 0}</div>
                            <div class="stat-label">Active Jobs</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${this.formatNumber(stats.total_coverage || 0)}</div>
                            <div class="stat-label">Total Coverage</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${stats.unique_crashes || 0}</div>
                            <div class="stat-label">Unique Crashes</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${this.formatSize(stats.corpus_size || 0)}</div>
                            <div class="stat-label">Corpus Size</div>
                        </div>
                        <div class="stat-card">
                            <div class="stat-value">${this.formatNumber(stats.exec_per_second || 0)}/s</div>
                            <div class="stat-label">Exec/Second</div>
                        </div>
                    </div>
                </div>
                
                <div class="modal-actions">
                    ${this.getDetailActionButtons(campaign)}
                </div>
            </div>
        `;
        
        document.getElementById('campaign-details-modal').style.display = 'block';
    }

    getDetailActionButtons(campaign) {
        const buttons = [];
        
        buttons.push(`<button class="btn btn-secondary" onclick="window.campaignsManager.viewCorpusEvolution('${campaign.id}')">View Corpus Evolution</button>`);
        buttons.push(`<button class="btn btn-secondary" onclick="window.campaignsManager.downloadCorpus('${campaign.id}')">Download Corpus</button>`);
        
        switch (campaign.status) {
            case 'running':
                buttons.push(`<button class="btn btn-primary" onclick="window.campaignsManager.pauseCampaign('${campaign.id}')">Pause Campaign</button>`);
                break;
            case 'paused':
                buttons.push(`<button class="btn btn-primary" onclick="window.campaignsManager.resumeCampaign('${campaign.id}')">Resume Campaign</button>`);
                break;
            case 'completed':
            case 'failed':
                buttons.push(`<button class="btn btn-primary" onclick="window.campaignsManager.restartCampaign('${campaign.id}')">Restart Campaign</button>`);
                break;
        }
        
        return buttons.join(' ');
    }

    hideDetailsModal() {
        document.getElementById('campaign-details-modal').style.display = 'none';
        this.currentCampaign = null;
    }

    async startCampaign(campaignId) {
        try {
            await this.api.updateCampaign(campaignId, { status: 'running' });
            this.showToast('Campaign started');
            await this.loadCampaigns();
        } catch (error) {
            console.error('Failed to start campaign:', error);
            this.showError('Failed to start campaign');
        }
    }

    async pauseCampaign(campaignId) {
        try {
            await this.api.updateCampaign(campaignId, { status: 'paused' });
            this.showToast('Campaign paused');
            await this.loadCampaigns();
            if (this.currentCampaign?.id === campaignId) {
                this.hideDetailsModal();
            }
        } catch (error) {
            console.error('Failed to pause campaign:', error);
            this.showError('Failed to pause campaign');
        }
    }

    async resumeCampaign(campaignId) {
        try {
            await this.api.updateCampaign(campaignId, { status: 'running' });
            this.showToast('Campaign resumed');
            await this.loadCampaigns();
        } catch (error) {
            console.error('Failed to resume campaign:', error);
            this.showError('Failed to resume campaign');
        }
    }

    async restartCampaign(campaignId) {
        if (!confirm('Are you sure you want to restart this campaign?')) {
            return;
        }
        
        try {
            await this.api.restartCampaign(campaignId);
            this.showToast('Campaign restarted');
            await this.loadCampaigns();
            if (this.currentCampaign?.id === campaignId) {
                this.hideDetailsModal();
            }
        } catch (error) {
            console.error('Failed to restart campaign:', error);
            this.showError('Failed to restart campaign');
        }
    }

    async deleteCampaign(campaignId) {
        if (!confirm('Are you sure you want to delete this campaign? This action cannot be undone.')) {
            return;
        }
        
        try {
            await this.api.deleteCampaign(campaignId);
            this.showToast('Campaign deleted');
            await this.loadCampaigns();
        } catch (error) {
            console.error('Failed to delete campaign:', error);
            this.showError('Failed to delete campaign');
        }
    }

    async viewCorpusEvolution(campaignId) {
        try {
            const evolution = await this.api.getCorpusEvolution(campaignId);
            // In a real implementation, show this in a chart
            console.log('Corpus evolution:', evolution);
            this.showToast('Corpus evolution data loaded (check console)');
        } catch (error) {
            console.error('Failed to load corpus evolution:', error);
            this.showError('Failed to load corpus evolution');
        }
    }

    async downloadCorpus(campaignId) {
        // In a real implementation, this would download a zip of corpus files
        this.showToast('Corpus download started...');
        
        try {
            // Get corpus files list
            const files = await this.api.listCorpusFiles(campaignId, { limit: 1000 });
            console.log('Corpus files:', files);
            this.showToast(`${files.files?.length || 0} corpus files available`);
        } catch (error) {
            console.error('Failed to download corpus:', error);
            this.showError('Failed to download corpus');
        }
    }

    // WebSocket event handlers
    handleCampaignCreated(data) {
        this.showToast(`New campaign created: ${data.name}`, 'success');
        this.loadCampaigns();
    }

    handleCampaignUpdated(data) {
        // Update campaign in list if visible
        const index = this.campaigns.findIndex(c => c.id === data.id);
        if (index >= 0) {
            this.campaigns[index] = { ...this.campaigns[index], ...data };
            this.filterCampaigns();
        }
        
        // Update details if viewing this campaign
        if (this.currentCampaign?.id === data.id) {
            this.viewCampaign(data.id);
        }
    }

    handleCampaignCompleted(data) {
        this.showToast(`Campaign completed: ${data.name}`, 'info');
        this.loadCampaigns();
    }

    updateCampaignMetrics(data) {
        // Update metrics if viewing this campaign
        if (this.currentCampaign?.id === data.campaign_id) {
            // In a real implementation, update the stats display
            console.log('Campaign metrics update:', data);
        }
    }

    updateConnectionStatus(connected) {
        const statusEl = document.getElementById('ws-status');
        if (!statusEl) return;

        const dot = statusEl.querySelector('.status-dot');
        const text = statusEl.querySelector('.status-text');

        if (connected) {
            dot.className = 'status-dot connected';
            text.textContent = 'Connected';
        } else {
            dot.className = 'status-dot disconnected';
            text.textContent = 'Disconnected';
        }
    }

    showToast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        if (!container) return;

        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;

        container.appendChild(toast);

        // Auto-remove after 5 seconds
        setTimeout(() => {
            toast.classList.add('fade-out');
            setTimeout(() => toast.remove(), 300);
        }, 5000);
    }

    showError(message) {
        this.showToast(message, 'error');
    }

    showLoading(show) {
        if (show) {
            document.body.classList.add('loading');
        } else {
            document.body.classList.remove('loading');
        }
    }

    formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        } else if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }

    formatSize(bytes) {
        if (bytes >= 1024 * 1024 * 1024) {
            return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
        } else if (bytes >= 1024 * 1024) {
            return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
        } else if (bytes >= 1024) {
            return (bytes / 1024).toFixed(1) + ' KB';
        }
        return bytes + ' B';
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Cleanup
    destroy() {
        this.ws.disconnect();
    }
}

// Export for use in other scripts
window.CampaignsManager = CampaignsManager;