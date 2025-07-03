// Crashes Manager
class CrashesManager {
    constructor() {
        this.api = new PandaFuzzAPI();
        this.ws = new PandaFuzzWebSocket();
        this.currentCampaign = '';
        this.currentGroup = null;
        this.crashGroups = [];
        this.crashes = [];
        this.filters = {
            campaign: '',
            severity: '',
            type: '',
            minCount: 1
        };
    }

    async init() {
        console.log('Initializing crashes page...');
        
        // Set up WebSocket
        this.setupWebSocket();
        
        // Set up event handlers
        this.setupEventHandlers();
        
        // Load initial data
        await this.loadCampaigns();
        await this.loadCrashGroups();
        
        // Connect WebSocket
        this.ws.connect();
    }

    setupWebSocket() {
        // Connection status handler
        this.ws.onConnectionStatusChange((connected) => {
            this.updateConnectionStatus(connected);
        });

        // Crash events
        this.ws.on(PandaFuzzWebSocket.Events.CRASH_FOUND, (data) => {
            this.handleNewCrash(data);
        });

        // Campaign events
        this.ws.on(PandaFuzzWebSocket.Events.CAMPAIGN_UPDATED, (data) => {
            if (data.id === this.currentCampaign) {
                this.loadCrashGroups();
            }
        });
    }

    setupEventHandlers() {
        // Filter handlers
        document.getElementById('campaign-filter')?.addEventListener('change', (e) => {
            this.filters.campaign = e.target.value;
            this.loadCrashGroups();
        });

        document.getElementById('severity-filter')?.addEventListener('change', (e) => {
            this.filters.severity = e.target.value;
            this.filterCrashGroups();
        });

        document.getElementById('type-filter')?.addEventListener('change', (e) => {
            this.filters.type = e.target.value;
            this.filterCrashGroups();
        });

        document.getElementById('min-count')?.addEventListener('change', (e) => {
            this.filters.minCount = parseInt(e.target.value) || 1;
            this.filterCrashGroups();
        });

        // Refresh button
        document.getElementById('refresh-crashes-btn')?.addEventListener('click', () => {
            this.loadCrashGroups();
        });

        // Export button
        document.getElementById('export-crashes-btn')?.addEventListener('click', () => {
            this.exportCrashes();
        });

        // Back to groups button
        document.getElementById('back-to-groups')?.addEventListener('click', () => {
            this.showCrashGroups();
        });

        // Modal close
        document.getElementById('close-stack-modal')?.addEventListener('click', () => {
            this.closeStackModal();
        });

        // Modal actions
        document.getElementById('download-input-btn')?.addEventListener('click', () => {
            if (this.currentCrash) {
                this.downloadCrashInput(this.currentCrash.id);
            }
        });

        document.getElementById('copy-stack-btn')?.addEventListener('click', () => {
            this.copyStackTrace();
        });

        // API docs link
        document.getElementById('api-docs-link')?.addEventListener('click', (e) => {
            e.preventDefault();
            window.open('/api/docs', '_blank');
        });
    }

    async loadCampaigns() {
        try {
            const result = await this.api.listCampaigns({ limit: 100 });
            const select = document.getElementById('campaign-filter');
            
            // Clear existing options except "All"
            select.innerHTML = '<option value="">All Campaigns</option>';
            
            // Add campaign options
            result.campaigns.forEach(campaign => {
                const option = document.createElement('option');
                option.value = campaign.id;
                option.textContent = campaign.name;
                select.appendChild(option);
            });
        } catch (error) {
            console.error('Failed to load campaigns:', error);
        }
    }

    async loadCrashGroups() {
        try {
            this.showLoading(true);
            
            if (!this.filters.campaign) {
                // Load crashes from all campaigns
                const campaigns = await this.api.listCampaigns({ limit: 100 });
                this.crashGroups = [];
                
                for (const campaign of campaigns.campaigns) {
                    try {
                        const groups = await this.api.getCrashGroups(campaign.id);
                        groups.groups.forEach(group => {
                            group.campaign_name = campaign.name;
                            group.campaign_id = campaign.id;
                        });
                        this.crashGroups.push(...groups.groups);
                    } catch (error) {
                        console.debug('No crashes for campaign', campaign.id);
                    }
                }
            } else {
                // Load crashes from specific campaign
                const groups = await this.api.getCrashGroups(this.filters.campaign);
                this.crashGroups = groups.groups || [];
                
                // Get campaign name
                if (this.crashGroups.length > 0) {
                    const campaign = await this.api.getCampaign(this.filters.campaign);
                    this.crashGroups.forEach(group => {
                        group.campaign_name = campaign.campaign.name;
                        group.campaign_id = campaign.campaign.id;
                    });
                }
            }
            
            this.updateStatistics();
            this.renderCrashGroups();
            
        } catch (error) {
            console.error('Failed to load crash groups:', error);
            this.showError('Failed to load crash groups');
        } finally {
            this.showLoading(false);
        }
    }

    updateStatistics() {
        const totalCrashes = this.crashGroups.reduce((sum, group) => sum + group.count, 0);
        const uniqueStacks = this.crashGroups.length;
        const criticalCrashes = this.crashGroups.filter(g => g.severity === 'critical').length;
        
        // Today's crashes (approximation based on last_seen)
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        const todayCrashes = this.crashGroups.filter(g => {
            const lastSeen = new Date(g.last_seen);
            return lastSeen >= today;
        }).reduce((sum, group) => sum + group.count, 0);
        
        // Update UI
        document.getElementById('total-crashes').textContent = totalCrashes;
        document.getElementById('unique-stacks').textContent = uniqueStacks;
        document.getElementById('critical-crashes').textContent = criticalCrashes;
        document.getElementById('today-crashes').textContent = todayCrashes;
        
        // Update footer
        document.getElementById('footer-total-crashes').textContent = totalCrashes;
        document.getElementById('footer-unique-crashes').textContent = uniqueStacks;
    }

    filterCrashGroups() {
        // Apply client-side filters
        const filtered = this.crashGroups.filter(group => {
            if (this.filters.severity && group.severity !== this.filters.severity) {
                return false;
            }
            if (this.filters.type && !this.inferCrashType(group).includes(this.filters.type)) {
                return false;
            }
            if (group.count < this.filters.minCount) {
                return false;
            }
            return true;
        });
        
        this.renderCrashGroups(filtered);
    }

    renderCrashGroups(groups = this.crashGroups) {
        const tbody = document.getElementById('crash-groups-tbody');
        if (!tbody) return;
        
        if (groups.length === 0) {
            tbody.innerHTML = '<tr><td colspan="8" class="empty-state">No crash groups found</td></tr>';
            return;
        }
        
        tbody.innerHTML = groups.map(group => {
            const firstSeen = new Date(group.first_seen).toLocaleDateString();
            const lastSeen = new Date(group.last_seen).toLocaleDateString();
            const topFrame = group.stack_frames?.[0] || {};
            const frameText = topFrame.function || topFrame.file || 'Unknown';
            const severityClass = `severity-${group.severity || 'unknown'}`;
            
            return `
                <tr>
                    <td class="monospace">${group.stack_hash.substring(0, 8)}...</td>
                    <td>${group.campaign_name || 'Unknown'}</td>
                    <td><span class="badge">${group.count}</span></td>
                    <td><span class="badge ${severityClass}">${group.severity || 'unknown'}</span></td>
                    <td>${firstSeen}</td>
                    <td>${lastSeen}</td>
                    <td class="truncate" title="${this.escapeHtml(frameText)}">${this.escapeHtml(frameText)}</td>
                    <td>
                        <button class="btn btn-sm" onclick="window.crashesManager.viewGroup('${group.id}')">View</button>
                        <button class="btn btn-sm" onclick="window.crashesManager.viewStackTrace('${group.example_crash}')">Stack</button>
                    </td>
                </tr>
            `;
        }).join('');
    }

    async viewGroup(groupId) {
        const group = this.crashGroups.find(g => g.id === groupId);
        if (!group) return;
        
        this.currentGroup = group;
        
        // Load individual crashes for this group
        try {
            // For now, show a placeholder since we need to implement crash listing by group
            this.showIndividualCrashes([]);
            this.showToast('Viewing crashes for group ' + groupId.substring(0, 8));
        } catch (error) {
            console.error('Failed to load crashes:', error);
            this.showError('Failed to load crashes for this group');
        }
    }

    showIndividualCrashes(crashes) {
        // Hide groups, show crashes
        document.getElementById('crash-groups-table').style.display = 'none';
        document.getElementById('individual-crashes-section').style.display = 'block';
        
        const tbody = document.getElementById('crashes-tbody');
        if (!tbody) return;
        
        if (crashes.length === 0) {
            tbody.innerHTML = '<tr><td colspan="7" class="empty-state">No individual crashes in this group</td></tr>';
            return;
        }
        
        tbody.innerHTML = crashes.map(crash => {
            const timestamp = new Date(crash.timestamp).toLocaleString();
            
            return `
                <tr>
                    <td class="monospace">${crash.id.substring(0, 8)}...</td>
                    <td>${crash.job_id?.substring(0, 8) || '-'}</td>
                    <td>${crash.bot_id?.substring(0, 8) || '-'}</td>
                    <td>${crash.type || 'unknown'}</td>
                    <td>${crash.signal || '-'}</td>
                    <td>${timestamp}</td>
                    <td>
                        <button class="btn btn-sm" onclick="window.crashesManager.viewStackTrace('${crash.id}')">Stack</button>
                        <button class="btn btn-sm" onclick="window.crashesManager.downloadCrashInput('${crash.id}')">Input</button>
                    </td>
                </tr>
            `;
        }).join('');
    }

    showCrashGroups() {
        // Show groups, hide crashes
        document.getElementById('crash-groups-table').style.display = 'table';
        document.getElementById('individual-crashes-section').style.display = 'none';
        this.currentGroup = null;
    }

    async viewStackTrace(crashId) {
        if (!crashId) {
            this.showError('No crash ID provided');
            return;
        }
        
        try {
            this.currentCrash = { id: crashId };
            const stackTrace = await this.api.getStackTrace(crashId);
            this.showStackModal(crashId, stackTrace);
        } catch (error) {
            console.error('Failed to load stack trace:', error);
            this.showError('Failed to load stack trace');
        }
    }

    showStackModal(crashId, stackTrace) {
        // Update modal info
        document.getElementById('modal-crash-id').textContent = crashId.substring(0, 8);
        document.getElementById('modal-crash-type').textContent = 'Unknown'; // Would need crash details
        document.getElementById('modal-crash-signal').textContent = 'Unknown';
        document.getElementById('modal-stack-hash').textContent = stackTrace.top_n_hash || '-';
        
        // Render stack frames
        const framesContainer = document.getElementById('stack-frames-list');
        if (stackTrace.frames && stackTrace.frames.length > 0) {
            framesContainer.innerHTML = stackTrace.frames.map((frame, index) => {
                const isTopFrames = index < 5;
                return `
                    <div class="stack-frame ${isTopFrames ? 'top-frame' : ''}">
                        <span class="frame-number">#${index}</span>
                        <span class="frame-function">${this.escapeHtml(frame.function || 'Unknown')}</span>
                        <span class="frame-location">${this.escapeHtml(frame.file || '')}:${frame.line || 0}</span>
                        ${frame.offset ? `<span class="frame-offset">+0x${frame.offset.toString(16)}</span>` : ''}
                    </div>
                `;
            }).join('');
        } else {
            framesContainer.innerHTML = '<p class="empty-state">No stack frames available</p>';
        }
        
        // Show raw trace
        document.getElementById('raw-stack-trace').textContent = stackTrace.raw_trace || 'No raw trace available';
        
        // Show modal
        document.getElementById('stack-trace-modal').style.display = 'block';
    }

    closeStackModal() {
        document.getElementById('stack-trace-modal').style.display = 'none';
        this.currentCrash = null;
    }

    async downloadCrashInput(crashId) {
        try {
            const blob = await this.api.getCrashInput(crashId);
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `crash_input_${crashId.substring(0, 8)}.bin`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            
            this.showToast('Crash input downloaded');
        } catch (error) {
            console.error('Failed to download crash input:', error);
            this.showError('Failed to download crash input');
        }
    }

    copyStackTrace() {
        const rawTrace = document.getElementById('raw-stack-trace')?.textContent;
        if (!rawTrace) {
            this.showError('No stack trace to copy');
            return;
        }
        
        navigator.clipboard.writeText(rawTrace).then(() => {
            this.showToast('Stack trace copied to clipboard');
        }).catch(err => {
            console.error('Failed to copy:', err);
            this.showError('Failed to copy stack trace');
        });
    }

    async exportCrashes() {
        try {
            const data = {
                exported_at: new Date().toISOString(),
                total_crashes: parseInt(document.getElementById('total-crashes').textContent),
                unique_stacks: parseInt(document.getElementById('unique-stacks').textContent),
                crash_groups: this.crashGroups.map(group => ({
                    id: group.id,
                    campaign: group.campaign_name,
                    stack_hash: group.stack_hash,
                    count: group.count,
                    severity: group.severity,
                    first_seen: group.first_seen,
                    last_seen: group.last_seen,
                    top_frames: group.stack_frames?.slice(0, 5)
                }))
            };
            
            const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `pandafuzz_crashes_${new Date().toISOString().split('T')[0]}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            
            this.showToast('Crashes exported successfully');
        } catch (error) {
            console.error('Failed to export crashes:', error);
            this.showError('Failed to export crashes');
        }
    }

    handleNewCrash(data) {
        // Reload crash groups when new crash is found
        if (!this.filters.campaign || data.campaign_id === this.filters.campaign) {
            this.loadCrashGroups();
            this.showToast('New crash detected!', 'warning');
        }
    }

    inferCrashType(group) {
        // Try to infer crash type from stack frames
        if (!group.stack_frames || group.stack_frames.length === 0) {
            return 'generic';
        }
        
        const topFrame = group.stack_frames[0].function || '';
        const lowerFrame = topFrame.toLowerCase();
        
        if (lowerFrame.includes('malloc') || lowerFrame.includes('free')) {
            return 'heap_overflow';
        } else if (lowerFrame.includes('stack')) {
            return 'stack_overflow';
        } else if (lowerFrame.includes('use_after_free')) {
            return 'use_after_free';
        } else if (lowerFrame.includes('double_free')) {
            return 'double_free';
        } else if (lowerFrame.includes('null') || lowerFrame.includes('nullptr')) {
            return 'null_deref';
        } else if (lowerFrame.includes('assert')) {
            return 'assertion';
        }
        
        return 'generic';
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
window.CrashesManager = CrashesManager;