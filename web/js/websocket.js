// PandaFuzz WebSocket Client
class PandaFuzzWebSocket {
    constructor(url = null) {
        // Auto-detect WebSocket URL based on current location
        if (!url) {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const host = window.location.host;
            url = `${protocol}//${host}/api/v2/ws`;
        }
        
        this.url = url;
        this.ws = null;
        this.handlers = {};
        this.reconnectInterval = 5000;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 10;
        this.isConnected = false;
        this.subscriptions = new Set();
        this.messageQueue = [];
        this.connectionStatusCallbacks = [];
    }

    // Connect to WebSocket server
    connect() {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            console.log('WebSocket already connected');
            return;
        }

        console.log('Connecting to WebSocket:', this.url);
        
        try {
            this.ws = new WebSocket(this.url);
            
            this.ws.onopen = () => this.handleOpen();
            this.ws.onmessage = (event) => this.handleMessage(event);
            this.ws.onclose = () => this.handleClose();
            this.ws.onerror = (error) => this.handleError(error);
        } catch (error) {
            console.error('WebSocket connection failed:', error);
            this.scheduleReconnect();
        }
    }

    // Handle connection open
    handleOpen() {
        console.log('WebSocket connected');
        this.isConnected = true;
        this.reconnectAttempts = 0;
        
        // Notify connection status callbacks
        this.notifyConnectionStatus(true);
        
        // Re-subscribe to topics
        if (this.subscriptions.size > 0) {
            this.send({
                type: 'subscribe',
                data: {
                    topics: Array.from(this.subscriptions)
                }
            });
        }
        
        // Send queued messages
        while (this.messageQueue.length > 0) {
            const message = this.messageQueue.shift();
            this.send(message);
        }
        
        // Call connected handlers
        this.trigger('connected', { timestamp: new Date() });
    }

    // Handle incoming messages
    handleMessage(event) {
        try {
            const message = JSON.parse(event.data);
            console.debug('WebSocket message received:', message.type, message);
            
            // Handle special message types
            switch (message.type) {
                case 'welcome':
                    this.handleWelcome(message);
                    break;
                case 'ping':
                    this.send({ type: 'pong', data: {} });
                    break;
                default:
                    // Trigger event handlers
                    this.trigger(message.type, message.data);
                    this.trigger('message', message);
            }
        } catch (error) {
            console.error('Failed to parse WebSocket message:', error, event.data);
        }
    }

    // Handle welcome message
    handleWelcome(message) {
        console.log('WebSocket welcome received:', message.data);
        this.clientId = message.data.client_id;
    }

    // Handle connection close
    handleClose() {
        console.log('WebSocket disconnected');
        this.isConnected = false;
        this.ws = null;
        
        // Notify connection status callbacks
        this.notifyConnectionStatus(false);
        
        // Trigger disconnected event
        this.trigger('disconnected', { timestamp: new Date() });
        
        // Schedule reconnection
        this.scheduleReconnect();
    }

    // Handle connection error
    handleError(error) {
        console.error('WebSocket error:', error);
        this.trigger('error', { error: error });
    }

    // Schedule reconnection attempt
    scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            this.trigger('max_reconnect_failed', {});
            return;
        }
        
        this.reconnectAttempts++;
        const delay = Math.min(this.reconnectInterval * this.reconnectAttempts, 30000);
        
        console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
        
        setTimeout(() => {
            this.connect();
        }, delay);
    }

    // Send message
    send(message) {
        if (!message.timestamp) {
            message.timestamp = new Date().toISOString();
        }
        
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        } else {
            // Queue message for sending when connected
            this.messageQueue.push(message);
            console.debug('Message queued (not connected):', message);
        }
    }

    // Subscribe to topics
    subscribe(topics) {
        if (!Array.isArray(topics)) {
            topics = [topics];
        }
        
        topics.forEach(topic => this.subscriptions.add(topic));
        
        if (this.isConnected) {
            this.send({
                type: 'subscribe',
                data: { topics: topics }
            });
        }
    }

    // Unsubscribe from topics
    unsubscribe(topics) {
        if (!Array.isArray(topics)) {
            topics = [topics];
        }
        
        topics.forEach(topic => this.subscriptions.delete(topic));
        
        if (this.isConnected) {
            this.send({
                type: 'unsubscribe',
                data: { topics: topics }
            });
        }
    }

    // Register event handler
    on(event, handler) {
        if (!this.handlers[event]) {
            this.handlers[event] = [];
        }
        this.handlers[event].push(handler);
    }

    // Remove event handler
    off(event, handler) {
        if (this.handlers[event]) {
            this.handlers[event] = this.handlers[event].filter(h => h !== handler);
        }
    }

    // Trigger event handlers
    trigger(event, data) {
        if (this.handlers[event]) {
            this.handlers[event].forEach(handler => {
                try {
                    handler(data);
                } catch (error) {
                    console.error(`Error in ${event} handler:`, error);
                }
            });
        }
    }

    // Register connection status callback
    onConnectionStatusChange(callback) {
        this.connectionStatusCallbacks.push(callback);
        // Immediately call with current status
        callback(this.isConnected);
    }

    // Notify connection status callbacks
    notifyConnectionStatus(connected) {
        this.connectionStatusCallbacks.forEach(callback => {
            try {
                callback(connected);
            } catch (error) {
                console.error('Error in connection status callback:', error);
            }
        });
    }

    // Disconnect WebSocket
    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.isConnected = false;
        this.messageQueue = [];
        this.reconnectAttempts = this.maxReconnectAttempts; // Prevent auto-reconnect
    }

    // Helper method to wait for connection
    async waitForConnection(timeout = 5000) {
        if (this.isConnected) {
            return true;
        }

        return new Promise((resolve) => {
            const startTime = Date.now();
            
            const checkConnection = () => {
                if (this.isConnected) {
                    resolve(true);
                } else if (Date.now() - startTime > timeout) {
                    resolve(false);
                } else {
                    setTimeout(checkConnection, 100);
                }
            };
            
            checkConnection();
        });
    }
}

// Event type constants
PandaFuzzWebSocket.Events = {
    CAMPAIGN_CREATED: 'campaign_created',
    CAMPAIGN_UPDATED: 'campaign_updated',
    CAMPAIGN_COMPLETED: 'campaign_completed',
    CRASH_FOUND: 'crash_found',
    CORPUS_UPDATE: 'corpus_update',
    BOT_STATUS: 'bot_status',
    JOB_PROGRESS: 'job_progress',
    SYSTEM_ALERT: 'system_alert',
    CAMPAIGN_METRICS_UPDATE: 'campaign_metrics_update',
};

// Export for use in other scripts
window.PandaFuzzWebSocket = PandaFuzzWebSocket;