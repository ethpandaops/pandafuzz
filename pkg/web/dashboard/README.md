# PandaFuzz Analytics Dashboard

A React TypeScript dashboard for visualizing and monitoring PandaFuzz fuzzing campaigns.

## Features

### Dashboard Overview
- **Campaign Summary**: View all active campaigns with key metrics
- **Global Statistics**: Total jobs, crashes, coverage, and execution speed
- **Quick Insights**: Top performing campaigns by crashes and coverage
- **Real-time Updates**: Live metrics with WebSocket support

### Campaign Details
- **Coverage Trends**: Interactive line charts showing coverage growth over time
- **Crash Analysis**: Crash rate metrics with trend detection
- **Job Management**: View and monitor jobs within a campaign
- **Performance Metrics**: Execution speed, corpus size, and resource usage

### Fuzzer Performance
- **Comparative Analysis**: Compare performance across different fuzzer types
- **Multi-dimensional View**: Radar charts for capability comparison
- **Resource Utilization**: CPU, memory, and disk usage metrics
- **Efficiency Scoring**: Composite metrics for fuzzer effectiveness

### Bot Fleet Management
- **Fleet Status**: Real-time view of bot availability and utilization
- **Capability Matrix**: Overview of bot capabilities and distribution
- **Auto-refresh**: Automatic updates every 5 seconds
- **Job Assignment**: Track which bots are running which jobs

## Installation

```bash
cd pkg/web/dashboard
npm install
```

## Development

```bash
# Start development server
npm start

# The dashboard will be available at http://localhost:3000
# API requests are proxied to http://localhost:8080
```

## Building

```bash
# Create production build
npm run build

# Build output will be in the build/ directory
```

## Configuration

### API Configuration
Edit `src/services/api.ts` to configure:
- API base URL
- API key authentication
- Request/response interceptors

### Environment Variables
Create a `.env` file for environment-specific settings:
```
REACT_APP_API_URL=http://localhost:8080/api/v1
REACT_APP_API_KEY=your-api-key
```

## Integration with Master Server

### Option 1: Standalone Development
Run the dashboard separately during development:
```bash
npm start
```

### Option 2: Static Build Integration
Build and serve from the master server:
```bash
npm run build
# Copy build files to master server static directory
```

### Option 3: Embedded Server
Use Go's embed package to include the built dashboard:
```go
//go:embed dashboard/build/*
var dashboardFiles embed.FS

// Serve from master server
router.PathPrefix("/dashboard").Handler(
    http.FileServer(http.FS(dashboardFiles))
)
```

## Charts and Visualizations

The dashboard uses Chart.js with React Chart.js 2 for visualizations:

- **Line Charts**: Coverage trends, execution speed over time
- **Bar Charts**: Job status distribution, fuzzer comparison
- **Doughnut Charts**: Bot status distribution
- **Radar Charts**: Multi-dimensional fuzzer performance

## Component Structure

```
src/
├── components/
│   ├── Dashboard.tsx       # Main overview page
│   ├── CampaignDetails.tsx # Detailed campaign view
│   ├── FuzzerPerformance.tsx # Fuzzer comparison
│   └── BotStatus.tsx       # Bot fleet management
├── services/
│   └── api.ts             # API client service
├── types/
│   └── index.ts           # TypeScript type definitions
├── App.tsx                # Main app component with routing
└── index.tsx              # Entry point
```

## Customization

### Adding New Metrics
1. Update types in `src/types/index.ts`
2. Add API endpoint in `src/services/api.ts`
3. Create visualization component
4. Add route in `App.tsx`

### Styling
- Global styles: `src/App.css`
- Component-specific styles can be added as CSS modules
- Uses CSS Grid and Flexbox for responsive layouts

### Theme Customization
Modify CSS variables in `App.css`:
```css
:root {
  --primary-color: #1976d2;
  --success-color: #4caf50;
  --warning-color: #ff9800;
  --error-color: #f44336;
}
```

## Performance Considerations

- **Data Caching**: API responses are cached client-side
- **Pagination**: Large datasets are paginated
- **Lazy Loading**: Charts are loaded on-demand
- **Debouncing**: API calls are debounced for user inputs

## Browser Support

- Chrome/Edge (latest)
- Firefox (latest)
- Safari (latest)

## Troubleshooting

### CORS Issues
If experiencing CORS issues during development:
1. Ensure the proxy setting in `package.json` is correct
2. Or configure CORS headers on the API server

### Build Errors
```bash
# Clear cache and reinstall
rm -rf node_modules package-lock.json
npm install
```

### API Connection Issues
Check:
1. API server is running on the expected port
2. API key is configured correctly
3. Network connectivity between dashboard and API