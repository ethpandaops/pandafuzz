import { useState, useEffect } from 'react';
import api from '../api/client';
import { HealthCheck } from '../types';

export function useHealthCheck() {
  const [health, setHealth] = useState<HealthCheck | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchHealth = async () => {
      try {
        setLoading(true);
        const data = await api.getHealth();
        setHealth(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch health');
        console.error('Failed to fetch health:', err);
      } finally {
        setLoading(false);
      }
    };

    fetchHealth();
    
    // Refresh health check every 30 seconds
    const interval = setInterval(fetchHealth, 30000);
    
    return () => clearInterval(interval);
  }, []);

  return { health, loading, error };
}