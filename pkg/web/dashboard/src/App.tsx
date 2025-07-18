import React from 'react';
import { BrowserRouter as Router, Routes, Route, Link } from 'react-router-dom';
import Dashboard from './components/Dashboard';
import CampaignDetails from './components/CampaignDetails';
import FuzzerPerformance from './components/FuzzerPerformance';
import BotStatus from './components/BotStatus';
import './App.css';

function App() {
  return (
    <Router>
      <div className="App">
        <nav className="navbar">
          <div className="nav-brand">
            <h1>PandaFuzz Dashboard</h1>
          </div>
          <ul className="nav-links">
            <li><Link to="/">Overview</Link></li>
            <li><Link to="/fuzzers">Fuzzers</Link></li>
            <li><Link to="/bots">Bots</Link></li>
          </ul>
        </nav>

        <main className="main-content">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/campaign/:id" element={<CampaignDetails />} />
            <Route path="/fuzzers" element={<FuzzerPerformance />} />
            <Route path="/bots" element={<BotStatus />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;