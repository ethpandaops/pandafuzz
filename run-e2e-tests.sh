#!/bin/bash

# PandaFuzz E2E Test Runner Script

set -e

echo "🐼 PandaFuzz E2E Test Suite 🐼"
echo "==============================="

# Check if npm is installed
if ! command -v npm &> /dev/null; then
    echo "❌ npm is not installed. Please install Node.js and npm first."
    exit 1
fi

# Check if docker compose is installed
if ! command -v docker &> /dev/null || ! docker compose version &> /dev/null; then
    echo "❌ Docker Compose is not installed or not accessible."
    exit 1
fi

# Install dependencies
echo "📦 Installing dependencies..."
npm install

# Clean up any existing containers
echo "🧹 Cleaning up existing containers..."
docker compose down -v || true

# Build and start services
echo "🚀 Starting PandaFuzz services..."
docker compose up -d --build

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
npx wait-on http://localhost:8088/health -t 60000 || {
    echo "❌ Services failed to start within 60 seconds"
    docker compose logs
    docker compose down
    exit 1
}

echo "✅ Services are ready!"

# Run the tests
echo "🧪 Running E2E tests..."
if npm test; then
    echo "✅ All tests passed!"
    TEST_RESULT=0
else
    echo "❌ Some tests failed!"
    TEST_RESULT=1
fi

# Show test report
echo "📊 Opening test report..."
npm run test:report || true

# Clean up
read -p "Do you want to keep the services running? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "🧹 Cleaning up services..."
    docker compose down
fi

exit $TEST_RESULT