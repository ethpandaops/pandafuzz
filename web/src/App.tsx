import React, { useState, useMemo } from 'react';
import {
  BrowserRouter as Router,
  Routes,
  Route,
  Navigate,
} from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import { Box, Fade } from '@mui/material';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Bots from './pages/Bots';
import Jobs from './pages/Jobs';
import Crashes from './pages/Crashes';
import Coverage from './pages/Coverage';
import { Corpus } from './pages/Corpus';
import { theme, darkTheme } from './theme';

function App() {
  const [darkMode, setDarkMode] = useState(() => {
    const saved = localStorage.getItem('darkMode');
    return saved === 'true' || (!saved && window.matchMedia('(prefers-color-scheme: dark)').matches);
  });

  const currentTheme = useMemo(() => darkMode ? darkTheme : theme, [darkMode]);

  const toggleTheme = () => {
    setDarkMode(!darkMode);
    localStorage.setItem('darkMode', (!darkMode).toString());
  };

  return (
    <ThemeProvider theme={currentTheme}>
      <CssBaseline />
      <Router>
        <Layout darkMode={darkMode} toggleTheme={toggleTheme}>
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={
              <PageTransition>
                <Dashboard />
              </PageTransition>
            } />
            <Route path="/bots" element={
              <PageTransition>
                <Bots />
              </PageTransition>
            } />
            <Route path="/jobs" element={
              <PageTransition>
                <Jobs />
              </PageTransition>
            } />
            <Route path="/crashes" element={
              <PageTransition>
                <Crashes />
              </PageTransition>
            } />
            <Route path="/coverage" element={
              <PageTransition>
                <Coverage />
              </PageTransition>
            } />
            <Route path="/corpus" element={
              <PageTransition>
                <Corpus />
              </PageTransition>
            } />
          </Routes>
        </Layout>
      </Router>
    </ThemeProvider>
  );
}

function PageTransition({ children }: { children: React.ReactNode }) {
  return (
    <Fade in={true} timeout={300}>
      <Box sx={{ width: '100%', height: '100%' }}>
        {children}
      </Box>
    </Fade>
  );
}

export default App;