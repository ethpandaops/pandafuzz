import React, { useEffect, useState, useCallback } from 'react';
import {
  Grid,
  Card,
  CardContent,
  Typography,
  Box,
  LinearProgress,
  Chip,
  Skeleton,
  Fade,
  Zoom,
  useTheme,
  alpha,
  Tooltip,
  IconButton,
} from '@mui/material';
import {
  Computer as BotsIcon,
  Work as JobsIcon,
  BugReport as CrashesIcon,
  Assessment as CoverageIcon,
  TrendingUp as TrendingUpIcon,
  TrendingDown as TrendingDownIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import api from '../api/client';
import { DashboardStats } from '../types';

interface StatCardProps {
  title: string;
  value: number;
  subtitle?: string;
  icon: React.ReactNode;
  color: string;
  trend?: number;
  loading?: boolean;
  delay?: number;
}

function StatCard({ title, value, subtitle, icon, color, trend, loading, delay = 0 }: StatCardProps) {
  const theme = useTheme();
  
  if (loading) {
    return (
      <Card>
        <CardContent>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Box flex={1}>
              <Skeleton animation="wave" width="60%" height={20} />
              <Skeleton animation="wave" width="40%" height={40} sx={{ my: 1 }} />
              <Skeleton animation="wave" width="80%" height={16} />
            </Box>
            <Skeleton animation="wave" variant="circular" width={48} height={48} />
          </Box>
        </CardContent>
      </Card>
    );
  }
  
  return (
    <Zoom in={true} style={{ transitionDelay: `${delay}ms` }}>
      <Card
        sx={{
          transition: 'all 0.3s ease-in-out',
          '&:hover': {
            transform: 'translateY(-4px)',
            boxShadow: '0 8px 24px rgba(0,0,0,0.12)',
          },
        }}
      >
        <CardContent>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Box>
              <Typography color="textSecondary" gutterBottom variant="subtitle2">
                {title}
              </Typography>
              <Box display="flex" alignItems="baseline" gap={1}>
                <Typography variant="h4" sx={{ fontWeight: 600 }}>
                  {value.toLocaleString()}
                </Typography>
                {trend !== undefined && (
                  <Chip
                    size="small"
                    icon={trend > 0 ? <TrendingUpIcon /> : <TrendingDownIcon />}
                    label={`${trend > 0 ? '+' : ''}${trend}%`}
                    color={trend > 0 ? 'success' : 'error'}
                    sx={{ 
                      height: 20,
                      '& .MuiChip-icon': { fontSize: 16 },
                    }}
                  />
                )}
              </Box>
              {subtitle && (
                <Typography variant="body2" color="textSecondary" sx={{ mt: 0.5 }}>
                  {subtitle}
                </Typography>
              )}
            </Box>
            <Box 
              sx={{ 
                color,
                backgroundColor: alpha(color, 0.1),
                borderRadius: 2,
                p: 1.5,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              {React.cloneElement(icon as React.ReactElement, { sx: { fontSize: 32 } })}
            </Box>
          </Box>
        </CardContent>
      </Card>
    </Zoom>
  );
}

function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const theme = useTheme();

  const fetchStats = useCallback(async (showLoading = true) => {
    try {
      if (showLoading) {
        setLoading(true);
      } else {
        setIsRefreshing(true);
      }
      const data = await api.getDashboardStats();
      setStats(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch stats');
    } finally {
      if (showLoading) {
        setLoading(false);
      } else {
        setIsRefreshing(false);
      }
    }
  }, []);

  useEffect(() => {
    fetchStats(true);
    const interval = setInterval(() => fetchStats(false), 10000); // Refresh every 10 seconds
    return () => clearInterval(interval);
  }, [fetchStats]);

  const renderLoadingCards = () => (
    <Grid container spacing={3}>
      {[0, 1, 2, 3].map((index) => (
        <Grid item xs={12} sm={6} md={3} key={index}>
          <StatCard
            title=""
            value={0}
            icon={<BotsIcon />}
            color=""
            loading={true}
          />
        </Grid>
      ))}
    </Grid>
  );

  if (error) {
    return (
      <Box p={2}>
        <Typography color="error">Error: {error}</Typography>
      </Box>
    );
  }

  if (!stats) {
    return null;
  }

  return (
    <Fade in={true} timeout={500}>
      <Box>
        <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
          <Typography variant="h4" sx={{ fontWeight: 600 }}>
            System Dashboard
          </Typography>
          <Box display="flex" alignItems="center" gap={1}>
            {isRefreshing && (
              <Typography variant="caption" color="text.secondary">
                Updating...
              </Typography>
            )}
            <Tooltip title="Refresh stats">
              <IconButton 
                onClick={() => fetchStats(false)}
                disabled={isRefreshing}
                sx={{
                  transition: 'transform 0.3s ease-in-out',
                  '&:hover': {
                    transform: 'rotate(180deg)',
                  },
                }}
              >
                <RefreshIcon />
              </IconButton>
            </Tooltip>
          </Box>
        </Box>
        
        {loading && !stats ? (
          renderLoadingCards()
        ) : error ? (
          <Card>
            <CardContent>
              <Typography color="error" align="center">
                {error}
              </Typography>
            </CardContent>
          </Card>
        ) : stats ? (
          <>
            <Grid container spacing={3}>
              <Grid item xs={12} sm={6} md={3}>
                <StatCard
                  title="Active Bots"
                  value={stats.activeBots}
                  subtitle={`${stats.totalBots} total`}
                  icon={<BotsIcon fontSize="large" />}
                  color="#4caf50"
                />
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <StatCard
                  title="Running Jobs"
                  value={stats.runningJobs}
                  subtitle={`${stats.totalJobs} total`}
                  icon={<JobsIcon fontSize="large" />}
                  color="#2196f3"
                />
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <StatCard
                  title="Unique Crashes"
                  value={stats.uniqueCrashes}
                  subtitle={`${stats.totalCrashes} total`}
                  icon={<CrashesIcon fontSize="large" />}
                  color="#f44336"
                />
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <StatCard
                  title="Average Coverage"
                  value={Math.round(stats.averageCoverage)}
                  subtitle="percent"
                  icon={<CoverageIcon fontSize="large" />}
                  color="#ff9800"
                />
              </Grid>
            </Grid>

            <Box mt={4}>
              <Card>
                <CardContent>
                  <Typography variant="h6" gutterBottom>
                    System Activity
                  </Typography>
                  <Box display="flex" gap={2} flexWrap="wrap">
                    <Chip 
                      label={`${stats.jobsPerHour} jobs/hour`}
                      color="primary"
                      variant="outlined"
                    />
                    <Chip 
                      label={`${stats.totalBots > 0 ? Math.round((stats.activeBots / stats.totalBots) * 100) : 0}% bots active`}
                      color="success"
                      variant="outlined"
                    />
                    <Chip 
                      label={`${stats.totalCrashes} crashes found`}
                      color="error"
                      variant="outlined"
                    />
                  </Box>
                </CardContent>
              </Card>
            </Box>
          </>
        ) : null}
      </Box>
    </Fade>
  );
}

export default Dashboard;