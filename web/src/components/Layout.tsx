import React, { useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useGlobalKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';
import { useHealthCheck } from '../hooks/useHealthCheck';
import {
  AppBar,
  Box,
  Drawer,
  IconButton,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Toolbar,
  Typography,
  Divider,
  Tooltip,
  Badge,
  useTheme,
  alpha,
  Fade,
  Zoom,
} from '@mui/material';
import {
  Menu as MenuIcon,
  Dashboard as DashboardIcon,
  Computer as BotsIcon,
  Work as JobsIcon,
  BugReport as CrashesIcon,
  Assessment as CoverageIcon,
  Folder as CorpusIcon,
  Brightness4 as DarkModeIcon,
  Brightness7 as LightModeIcon,
  GitHub as GitHubIcon,
  Settings as SettingsIcon,
} from '@mui/icons-material';

const drawerWidth = 240;

interface LayoutProps {
  children: React.ReactNode;
  darkMode?: boolean;
  toggleTheme?: () => void;
}

const menuItems = [
  { text: 'Dashboard', icon: <DashboardIcon />, path: '/dashboard' },
  { text: 'Bots', icon: <BotsIcon />, path: '/bots' },
  { text: 'Jobs', icon: <JobsIcon />, path: '/jobs' },
  { text: 'Corpus', icon: <CorpusIcon />, path: '/corpus' },
  { text: 'Crashes', icon: <CrashesIcon />, path: '/crashes' },
  { text: 'Coverage', icon: <CoverageIcon />, path: '/coverage' },
];

function Layout({ children, darkMode, toggleTheme }: LayoutProps) {
  const [mobileOpen, setMobileOpen] = useState(false);
  const location = useLocation();
  const theme = useTheme();
  useGlobalKeyboardShortcuts();
  const { health } = useHealthCheck();

  const handleDrawerToggle = () => {
    setMobileOpen(!mobileOpen);
  };

  const drawer = (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Toolbar sx={{ 
        background: `linear-gradient(45deg, ${theme.palette.primary.main} 30%, ${theme.palette.primary.light} 90%)`,
        color: 'white'
      }}>
        <Typography variant="h6" noWrap component="div" sx={{ fontWeight: 700 }}>
          🐼 PandaFuzz
        </Typography>
      </Toolbar>
      <Divider />
      <List sx={{ flex: 1 }}>
        {menuItems.map((item, index) => (
          <Zoom in={true} style={{ transitionDelay: `${index * 50}ms` }} key={item.text}>
            <ListItem disablePadding>
              <Tooltip title={item.text} placement="right" arrow>
                <ListItemButton
                  component={Link}
                  to={item.path}
                  selected={location.pathname === item.path}
                  sx={{
                    borderRadius: 2,
                    mx: 1,
                    my: 0.5,
                    transition: 'all 0.2s ease-in-out',
                    '&.Mui-selected': {
                      backgroundColor: alpha(theme.palette.primary.main, 0.15),
                      borderLeft: `4px solid ${theme.palette.primary.main}`,
                      '&:hover': {
                        backgroundColor: alpha(theme.palette.primary.main, 0.25),
                      },
                    },
                    '&:hover': {
                      backgroundColor: alpha(theme.palette.primary.main, 0.08),
                      transform: 'translateX(4px)',
                    },
                  }}
                >
                  <ListItemIcon sx={{ 
                    color: location.pathname === item.path ? theme.palette.primary.main : 'inherit',
                    minWidth: 40,
                  }}>
                    {item.icon}
                  </ListItemIcon>
                  <ListItemText 
                    primary={item.text} 
                    primaryTypographyProps={{
                      fontWeight: location.pathname === item.path ? 600 : 400,
                    }}
                  />
                </ListItemButton>
              </Tooltip>
            </ListItem>
          </Zoom>
        ))}
      </List>
      <Divider />
      <Box sx={{ p: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <Tooltip title="GitHub Repository" placement="right">
            <IconButton
              component="a"
              href="https://github.com/ethpandaops/pandafuzz"
              target="_blank"
              sx={{ mr: 1 }}
            >
              <GitHubIcon />
            </IconButton>
          </Tooltip>
          <Tooltip title="Settings" placement="right">
            <IconButton sx={{ mr: 1 }}>
              <SettingsIcon />
            </IconButton>
          </Tooltip>
        </Box>
        {health && health.git_commit && (
          <Box sx={{ mt: 1 }}>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
              Version: {health.version || 'dev'}
            </Typography>
            <Tooltip title={`Build: ${health.build_time || 'unknown'}`} placement="right">
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', cursor: 'help' }}>
                Commit: {health.git_commit.substring(0, 7)}
              </Typography>
            </Tooltip>
          </Box>
        )}
      </Box>
    </Box>
  );

  return (
    <Box sx={{ display: 'flex' }}>
      <AppBar
        position="fixed"
        sx={{
          width: { sm: `calc(100% - ${drawerWidth}px)` },
          ml: { sm: `${drawerWidth}px` },
        }}
      >
        <Toolbar>
          <IconButton
            color="inherit"
            aria-label="open drawer"
            edge="start"
            onClick={handleDrawerToggle}
            sx={{ mr: 2, display: { sm: 'none' } }}
          >
            <MenuIcon />
          </IconButton>
          <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>
            {menuItems.find((item) => item.path === location.pathname)?.text ||
              'PandaFuzz'}
          </Typography>
          <Tooltip title={darkMode ? 'Light mode' : 'Dark mode'}>
            <IconButton
              onClick={toggleTheme}
              color="inherit"
              sx={{
                transition: 'transform 0.3s ease-in-out',
                '&:hover': {
                  transform: 'rotate(180deg)',
                },
              }}
            >
              {darkMode ? <LightModeIcon /> : <DarkModeIcon />}
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>
      <Box
        component="nav"
        sx={{ width: { sm: drawerWidth }, flexShrink: { sm: 0 } }}
      >
        <Drawer
          variant="temporary"
          open={mobileOpen}
          onClose={handleDrawerToggle}
          ModalProps={{
            keepMounted: true,
          }}
          sx={{
            display: { xs: 'block', sm: 'none' },
            '& .MuiDrawer-paper': {
              boxSizing: 'border-box',
              width: drawerWidth,
            },
          }}
        >
          {drawer}
        </Drawer>
        <Drawer
          variant="permanent"
          sx={{
            display: { xs: 'none', sm: 'block' },
            '& .MuiDrawer-paper': {
              boxSizing: 'border-box',
              width: drawerWidth,
            },
          }}
          open
        >
          {drawer}
        </Drawer>
      </Box>
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          p: 3,
          width: { sm: `calc(100% - ${drawerWidth}px)` },
          minHeight: '100vh',
          backgroundColor: theme.palette.background.default,
          transition: 'background-color 0.3s ease-in-out',
        }}
      >
        <Toolbar />
        <Fade in={true} timeout={500}>
          <Box>{children}</Box>
        </Fade>
      </Box>
    </Box>
  );
}

export default Layout;