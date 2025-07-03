// Dashboard Manager
class Dashboard {
    constructor() {
        this.api = new PandaFuzzAPI();
        this.ws = new PandaFuzzWebSocket();
        this.refreshInterval = 30000; // 30 seconds
        this.refreshTimer = null;
        this.activityFilter = 'all';
        this.activityFeed = [];
        this.maxActivityItems = 50;
    }

    async init() {
        console.log('Initializing dashboard...');
        
        // Set up WebSocket
        this.setupWebSocket();
        
        // Set up event handlers
        this.setupEventHandlers();
        
        // Load initial data
        await this.loadDashboardData();
        
        // Start refresh timer
        this.startRefreshTimer();
        
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
            this.addActivityItem('campaign', 'New campaign created', data);
            this.loadCampaignStats();
        });

        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_UPDATED, (data) => {
            this.addActivityItem('campaign', 'Campaign updated', data);
            this.loadCampaignStats();
        });

        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_COMPLETED, (data) => {
            this.addActivityItem('campaign', 'Campaign completed', data);
            this.loadCampaignStats();
        });

        // Crash events
        this.ws.on(PandaFuzzWebSocket.Events.CRASH_FOUND, (data) => {
            this.addActivityItem('crash', 'New crash found', data);
            this.updateCrashCount();
            this.loadRecentCrashes();
        });

        // Corpus events
        this.ws.on(PandaFuzzWebSocket.Events.CORPUS_UPDATE, (data) => {
            this.addActivityItem('corpus', 'Corpus updated', data);
            this.updateCoverageStats();
        });

        // Bot events
        this.ws.on(PandaFuzzWebSocket.Events.BOT_STATUS, (data) => {
            this.updateBotStatus(data);
        });

        // Metrics updates
        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_METRICS_UPDATE, (data) => {
            this.updateCampaignMetrics(data);
        });

        // System alerts
        this.ws.on(PandaFuzzWebSocket.Events.SYSTEM_ALERT, (data) => {
            this.showAlert(data.level, data.message);
        });
    }

    setupEventHandlers() {
        // Refresh button
        document.getElementById('refresh-btn')?.addEventListener('click', () => {
            this.loadDashboardData();
        });

        // Activity filter
        document.getElementById('activity-filter')?.addEventListener('change', (e) => {
            this.activityFilter = e.target.value;
            this.renderActivityFeed();
        });

        // API docs link
        document.getElementById('api-docs-link')?.addEventListener('click', (e) => {
            e.preventDefault();
            window.open('/api/docs', '_blank');
        });
    }

    async loadDashboardData() {
        try {
            this.showLoading(true);
            
            // Load all data in parallel
            const [
                systemStatus,
                campaigns,
                bots,
                crashes,
                systemStats
            ] = await Promise.all([
                this.api.getSystemStatus(),
                this.api.listCampaigns({ limit: 100 }),
                this.api.listBots(),
                this.loadRecentCrashes(),
                this.api.getSystemStats()
            ]);

            // Update UI with loaded data
            this.updateSystemStatus(systemStatus);
            this.updateCampaignStats(campaigns);
            this.updateBotStats(bots);
            this.updateCrashStats(crashes);
            this.updatePerformanceMetrics(systemStats);

        } catch (error) {
            console.error('Failed to load dashboard data:', error);
            this.showError('Failed to load dashboard data');
        } finally {
            this.showLoading(false);
        }
    }

    updateSystemStatus(status) {
        // Update server time and uptime
        document.getElementById('server-time').textContent = 
            new Date(status.timestamp).toLocaleTimeString();
        document.getElementById('server-uptime').textContent = 
            this.formatDuration(status.server.uptime);
    }

    updateCampaignStats(campaigns) {
        const activeCampaigns = campaigns.campaigns.filter(c => c.status === 'running').length;
        const pendingCampaigns = campaigns.campaigns.filter(c => c.status === 'pending').length;
        const completedCampaigns = campaigns.campaigns.filter(c => c.status === 'completed').length;

        // Update metrics
        document.getElementById('active-campaigns').textContent = activeCampaigns;

        // Draw campaign chart
        this.drawCampaignChart({
            running: activeCampaigns,
            pending: pendingCampaigns,
            completed: completedCampaigns,
            failed: campaigns.campaigns.filter(c => c.status === 'failed').length
        });
    }

    updateBotStats(bots) {
        const onlineBots = bots.bots.filter(b => b.bot.is_online).length;
        document.getElementById('active-bots').textContent = onlineBots;

        // Update bot grid
        this.renderBotGrid(bots.bots);
    }

    async updateCrashStats(crashes) {
        if (!crashes) return;

        // Get unique crashes count from deduplicated groups
        try {
            // This would need to aggregate across all campaigns
            let uniqueCrashes = 0;
            const campaigns = await this.api.listCampaigns({ limit: 100 });
            
            for (const campaign of campaigns.campaigns) {
                try {
                    const groups = await this.api.getCrashGroups(campaign.id);
                    uniqueCrashes += groups.unique_crashes || 0;
                } catch (error) {
                    console.debug('No crashes for campaign', campaign.id);
                }
            }

            document.getElementById('unique-crashes').textContent = uniqueCrashes;
        } catch (error) {
            console.error('Failed to get crash stats:', error);
        }
    }

    updatePerformanceMetrics(stats) {
        // Update coverage
        const totalCoverage = stats.state?.total_coverage || 0;
        document.getElementById('total-coverage').textContent = 
            this.formatNumber(totalCoverage);

        // Update other metrics
        document.getElementById('exec-per-sec').textContent = 
            this.formatNumber(stats.state?.exec_per_second || 0);
        
        // Calculate corpus size in MB
        const corpusSizeMB = (stats.state?.corpus_size || 0) / (1024 * 1024);
        document.getElementById('corpus-size').textContent = 
            corpusSizeMB.toFixed(1);
    }

    async loadRecentCrashes() {
        // In a real implementation, this would load recent crashes
        // For now, return empty array
        return { crashes: [] };
    }

    async loadCampaignStats() {
        const campaigns = await this.api.listCampaigns({ limit: 100 });
        this.updateCampaignStats(campaigns);
    }

    async updateCrashCount() {
        const crashes = await this.loadRecentCrashes();
        this.updateCrashStats(crashes);
    }

    async updateCoverageStats() {
        const stats = await this.api.getSystemStats();
        this.updatePerformanceMetrics(stats);
    }

    drawCampaignChart(data) {
        const canvas = document.getElementById('campaign-chart');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Simple pie chart
        const total = Object.values(data).reduce((a, b) => a + b, 0);
        if (total === 0) {
            ctx.fillStyle = '#e5e7eb';
            ctx.fillRect(0, 0, width, height);
            ctx.fillStyle = '#6b7280';
            ctx.font = '14px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No campaigns', width / 2, height / 2);
            return;
        }

        const colors = {
            running: '#10b981',
            pending: '#f59e0b',
            completed: '#3b82f6',
            failed: '#ef4444'
        };

        let currentAngle = -Math.PI / 2;
        const centerX = width / 2;
        const centerY = height / 2;
        const radius = Math.min(width, height) / 2 - 10;

        // Draw pie slices
        Object.entries(data).forEach(([status, count]) => {
            if (count === 0) return;

            const sliceAngle = (count / total) * 2 * Math.PI;

            // Draw slice
            ctx.beginPath();
            ctx.moveTo(centerX, centerY);
            ctx.arc(centerX, centerY, radius, currentAngle, currentAngle + sliceAngle);
            ctx.closePath();
            ctx.fillStyle = colors[status];
            ctx.fill();

            currentAngle += sliceAngle;
        });

        // Draw legend
        const legendEl = document.getElementById('campaign-legend');
        legendEl.innerHTML = '';
        
        Object.entries(data).forEach(([status, count]) => {
            if (count === 0) return;
            
            const item = document.createElement('div');
            item.className = 'legend-item';
            item.innerHTML = `
                <span class="legend-color" style="background-color: ${colors[status]}"></span>
                <span class="legend-label">${status}: ${count}</span>
            `;
            legendEl.appendChild(item);
        });
    }

    renderBotGrid(bots) {
        const grid = document.getElementById('bot-status-grid');
        if (!grid) return;

        if (bots.length === 0) {
            grid.innerHTML = '<p class="empty-state">No bots connected</p>';
            return;
        }

        grid.innerHTML = bots.map(bot => {
            const b = bot.bot;
            const statusClass = b.is_online ? 'online' : 'offline';
            const healthClass = bot.health_status || 'unknown';
            
            return `
                <div class="bot-item ${statusClass}" title="${b.hostname}">
                    <div class="bot-icon">🤖</div>
                    <div class="bot-info">
                        <div class="bot-name">${b.name || b.id.substring(0, 8)}</div>
                        <div class="bot-status">
                            <span class="status-indicator ${healthClass}"></span>
                            ${b.current_job ? 'Working' : 'Idle'}
                        </div>
                    </div>
                </div>
            `;
        }).join('');
    }

    addActivityItem(type, message, data) {
        const item = {
            id: Date.now(),
            type: type,
            message: message,
            data: data,
            timestamp: new Date()
        };

        this.activityFeed.unshift(item);
        
        // Limit feed size
        if (this.activityFeed.length > this.maxActivityItems) {
            this.activityFeed = this.activityFeed.slice(0, this.maxActivityItems);
        }

        this.renderActivityFeed();
    }

    renderActivityFeed() {
        const feed = document.getElementById('activity-feed');
        if (!feed) return;

        const filteredItems = this.activityFilter === 'all' 
            ? this.activityFeed 
            : this.activityFeed.filter(item => item.type === this.activityFilter);

        if (filteredItems.length === 0) {
            feed.innerHTML = '<p class="empty-state">No recent activity</p>';
            return;
        }

        feed.innerHTML = filteredItems.map(item => {
            const icon = this.getActivityIcon(item.type);
            const timeAgo = this.formatTimeAgo(item.timestamp);
            
            return `
                <div class="activity-item">
                    <span class="activity-icon">${icon}</span>
                    <div class="activity-content">
                        <div class="activity-message">${item.message}</div>
                        <div class="activity-time">${timeAgo}</div>
                    </div>
                </div>
            `;
        }).join('');
    }

    getActivityIcon(type) {
        const icons = {
            campaign: '📊',
            crash: '🐛',
            corpus: '📁',
            bot: '🤖',
            system: '⚙️'
        };
        return icons[type] || '📌';
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

    updateBotStatus(data) {
        // Update bot grid with new status
        this.loadDashboardData();
    }

    updateCampaignMetrics(data) {
        // Update specific campaign metrics
        // This would update charts or specific metric displays
    }

    showAlert(level, message) {
        this.showToast(message, level);
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
        // In a real implementation, show/hide loading indicator
        if (show) {
            document.body.classList.add('loading');
        } else {
            document.body.classList.remove('loading');
        }
    }

    startRefreshTimer() {
        this.refreshTimer = setInterval(() => {
            this.loadDashboardData();
        }, this.refreshInterval);
    }

    stopRefreshTimer() {
        if (this.refreshTimer) {
            clearInterval(this.refreshTimer);
            this.refreshTimer = null;
        }
    }

    // Utility methods
    formatNumber(num) {
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        } else if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toString();
    }

    formatDuration(duration) {
        // Duration is expected to be a string like "72h34m12s"
        // Parse and format nicely
        const match = duration.match(/(\d+)h(\d+)m/);
        if (match) {
            const hours = parseInt(match[1]);
            const days = Math.floor(hours / 24);
            const remainingHours = hours % 24;
            
            if (days > 0) {
                return `${days}d ${remainingHours}h`;
            }
            return `${hours}h ${match[2]}m`;
        }
        return duration;
    }

    formatTimeAgo(date) {
        const seconds = Math.floor((new Date() - date) / 1000);
        
        if (seconds < 60) return 'just now';
        if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
        if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
        return Math.floor(seconds / 86400) + 'd ago';
    }

    // Cleanup
    destroy() {
        this.stopRefreshTimer();
        this.ws.disconnect();
    }
}

// Export for use in other scripts
window.Dashboard = Dashboard;