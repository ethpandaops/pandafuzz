# PandaFuzz Web Dashboard

The PandaFuzz Web Dashboard provides a modern, real-time interface for monitoring and managing your distributed fuzzing operations.

## Features

- **Real-time Monitoring**: Live updates of bot status, job progress, and crash discoveries
- **Bot Management**: View active bots, their capabilities, and current workload
- **Job Control**: Create, monitor, and manage fuzzing jobs with different priorities
- **Crash Analysis**: Browse and analyze discovered crashes with detailed information
- **Coverage Visualization**: Track code coverage progress with interactive charts
- **Dark Theme**: Modern dark UI optimized for extended monitoring sessions

## Development

### Prerequisites

- Node.js 18+ and npm
- Running PandaFuzz master server

### Quick Start

```bash
# Install dependencies
npm install

# Start development server
npm start

# Build for production
npm run build
```

The development server runs on http://localhost:3000 and proxies API requests to http://localhost:8080.

### Available Scripts

- `npm start` - Start development server with hot reloading
- `npm run build` - Build production-ready bundle
- `npm run test` - Run tests
- `npm run lint` - Run ESLint
- `npm run type-check` - Run TypeScript type checking

## Production Deployment

### With Master Server

The web UI is automatically served by the master server when built:

```bash
# Build web UI
make build-web

# Run master with UI
make run-master-with-ui
```

Access the dashboard at http://localhost:8080

### Docker

The Docker image includes the web UI:

```bash
# Build Docker image
docker build --target master -t pandafuzz-master .

# Run with docker-compose
docker-compose up -d
```

## Architecture

### Technology Stack

- **React 18** - UI framework
- **TypeScript** - Type safety
- **Material-UI** - Component library
- **React Router** - Client-side routing
- **Axios** - HTTP client
- **Recharts** - Data visualization

### Project Structure

```
web/
├── public/           # Static assets
├── src/
│   ├── api/         # API client
│   ├── components/  # Reusable components
│   ├── pages/       # Page components
│   ├── types/       # TypeScript types
│   └── utils/       # Utility functions
├── package.json
└── tsconfig.json
```

### API Integration

The dashboard communicates with the master server via RESTful API:

- `/api/v1/bots` - Bot management
- `/api/v1/jobs` - Job operations
- `/api/v1/results/crashes` - Crash results
- `/api/v1/results/coverage` - Coverage data
- `/api/v1/system/status` - System statistics

## Pages

### Dashboard

Overview of system status including:
- Active/total bots
- Running/completed jobs
- Unique crashes found
- Average code coverage

### Bots

Manage bot agents:
- View bot status and capabilities
- Monitor bot health and last seen time
- Remove inactive bots

### Jobs

Fuzzing job management:
- Create new fuzzing jobs
- Monitor job progress
- Cancel running jobs
- View job history

### Crashes

Analyze discovered crashes:
- Browse unique crashes
- View crash details and stack traces
- Download crash inputs
- Filter by job or crash type

### Coverage

Track fuzzing effectiveness:
- Coverage percentage over time
- Edge discovery rate
- Per-job statistics
- Performance metrics

## Customization

### Theme

Edit `src/App.tsx` to customize the Material-UI theme:

```typescript
const theme = createTheme({
  palette: {
    mode: 'dark',
    primary: {
      main: '#3f51b5',
    },
    // Customize colors
  },
});
```

### API Configuration

Update the API base URL in `src/api/client.ts`:

```typescript
constructor(baseURL: string = '') {
  this.client = axios.create({
    baseURL: baseURL || '/api/v1',
    // ...
  });
}
```

## Troubleshooting

### Connection Issues

If the dashboard cannot connect to the API:

1. Verify master server is running
2. Check CORS settings in master configuration
3. Ensure proxy configuration in package.json is correct
4. Check browser console for detailed errors

### Build Issues

For build problems:

1. Clear node_modules and reinstall: `rm -rf node_modules && npm install`
2. Clear build cache: `rm -rf build`
3. Check Node.js version: `node --version` (should be 18+)

## Contributing

1. Follow existing code style and TypeScript conventions
2. Add tests for new features
3. Update types when API changes
4. Test in both development and production modes
5. Ensure accessibility standards are met

## License

Part of the PandaFuzz project. See main repository for license details.