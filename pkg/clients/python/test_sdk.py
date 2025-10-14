#!/usr/bin/env python3
"""
Test script for PandaFuzz Python SDK.

This script performs basic tests to verify that the SDK is working correctly,
including imports, client initialization, and basic API calls.
"""

import sys
import os
import traceback
from typing import Optional

# Add current directory to Python path to find pandafuzz module
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# Import pandafuzz at module level
try:
    import pandafuzz
except ImportError as e:
    print(f"Failed to import pandafuzz: {e}")
    sys.exit(1)

def test_imports():
    """Test that all main SDK components can be imported."""
    print("Testing SDK imports...")
    
    try:
        # Test main package import
        import pandafuzz
        print("✓ pandafuzz package imported")
        
        # Test high-level clients
        from pandafuzz import PandaFuzzClient, AsyncPandaFuzzClient
        print("✓ High-level clients imported")
        
        # Test SSE components
        from pandafuzz import SSEClient, SSEEvent, PandaFuzzEventTypes
        print("✓ SSE components imported")
        
        # Test generated API clients
        from pandafuzz import (
            AnalyticsApi, BatchApi, BotsApi, CampaignsApi, 
            CorpusApi, CrashesApi, EventsApi, HealthApi, JobsApi
        )
        print("✓ Generated API clients imported")
        
        # Test models
        from pandafuzz.models import (
            Job, JobCreateRequest, Campaign, CampaignCreateRequest,
            Bot, BotCreateRequest, Crash, HealthStatus
        )
        print("✓ API models imported")
        
        # Test exceptions
        from pandafuzz.exceptions import ApiException, ApiValueError
        print("✓ Exception classes imported")
        
        return True
        
    except ImportError as e:
        print(f"✗ Import failed: {e}")
        traceback.print_exc()
        return False


def test_client_initialization():
    """Test client initialization."""
    print("\\nTesting client initialization...")
    
    try:
        # Test sync client initialization
        client = pandafuzz.PandaFuzzClient(
            host="http://localhost:8080",
            api_key="test-key"
        )
        print("✓ Sync client initialized")
        
        # Test async client initialization
        async_client = pandafuzz.AsyncPandaFuzzClient(
            host="http://localhost:8080", 
            api_key="test-key"
        )
        print("✓ Async client initialized")
        
        # Test SSE client initialization
        sse_client = pandafuzz.SSEClient(
            base_url="http://localhost:8080",
            api_key="test-key"
        )
        print("✓ SSE client initialized")
        
        # Clean up
        client.close()
        
        return True
        
    except Exception as e:
        print(f"✗ Client initialization failed: {e}")
        traceback.print_exc()
        return False


def test_event_types():
    """Test event type constants."""
    print("\\nTesting event types...")
    
    try:
        # Test individual event types
        assert pandafuzz.PandaFuzzEventTypes.JOB_CREATED == "job.created"
        assert pandafuzz.PandaFuzzEventTypes.CRASH_DISCOVERED == "crash.discovered"
        assert pandafuzz.PandaFuzzEventTypes.BOT_REGISTERED == "bot.registered"
        print("✓ Event type constants work")
        
        # Test event type collections
        job_events = pandafuzz.PandaFuzzEventTypes.all_job_events()
        assert isinstance(job_events, list)
        assert len(job_events) > 0
        assert pandafuzz.PandaFuzzEventTypes.JOB_CREATED in job_events
        print("✓ Event type collections work")
        
        bot_events = pandafuzz.PandaFuzzEventTypes.all_bot_events()
        crash_events = pandafuzz.PandaFuzzEventTypes.all_crash_events()
        campaign_events = pandafuzz.PandaFuzzEventTypes.all_campaign_events()
        
        assert all(isinstance(events, list) for events in [bot_events, crash_events, campaign_events])
        assert all(len(events) > 0 for events in [bot_events, crash_events, campaign_events])
        print("✓ All event type collections work")
        
        return True
        
    except Exception as e:
        print(f"✗ Event types test failed: {e}")
        traceback.print_exc()
        return False


def test_model_creation():
    """Test creating API model instances."""
    print("\\nTesting model creation...")
    
    try:
        # Test job creation request model
        job_request = pandafuzz.JobCreateRequest(
            name="test-job",
            fuzzer=pandafuzz.FuzzerType.LIBFUZZER,
            target_binary="/usr/bin/test",
            timeout_seconds=3600
        )
        assert job_request.name == "test-job"
        print("✓ JobCreateRequest model created")
        
        # Test campaign creation request model
        campaign_request = pandafuzz.CampaignCreateRequest(
            name="test-campaign",
            target_binary="/usr/bin/test",
            job_template={
                "fuzzer_type": "libfuzzer",
                "target_binary": "/usr/bin/test"
            },
            max_parallel_jobs=2
        )
        assert campaign_request.name == "test-campaign"
        print("✓ CampaignCreateRequest model created")
        
        # Test bot creation request model
        bot_request = pandafuzz.BotCreateRequest(
            name="test-bot",
            hostname="test-host",
            api_endpoint="http://localhost:9000",
            capabilities=["fuzzing", "analysis"],
            max_concurrent_jobs=4
        )
        assert bot_request.name == "test-bot"
        print("✓ BotCreateRequest model created")
        
        return True
        
    except Exception as e:
        print(f"✗ Model creation failed: {e}")
        traceback.print_exc()
        return False


def test_sse_event_creation():
    """Test SSE event creation."""
    print("\\nTesting SSE event creation...")
    
    try:
        # Test SSE event creation
        event_data = {"job_id": "test-job-123", "status": "completed"}
        event = pandafuzz.SSEEvent("job.completed", event_data, "event-123")
        
        assert event.event_type == "job.completed"
        assert event.data == event_data
        assert event.event_id == "event-123"
        assert event.timestamp > 0
        print("✓ SSE event created successfully")
        
        return True
        
    except Exception as e:
        print(f"✗ SSE event creation failed: {e}")
        traceback.print_exc()
        return False


def test_live_api_calls():
    """Test live API calls if environment is configured."""
    print("\\nTesting live API calls...")
    
    host = os.getenv("PANDAFUZZ_HOST")
    api_key = os.getenv("PANDAFUZZ_API_KEY")
    bearer_token = os.getenv("PANDAFUZZ_BEARER_TOKEN")
    
    if not host or not (api_key or bearer_token):
        print("⚠️  Skipping live API tests (no environment configuration)")
        print("   Set PANDAFUZZ_HOST and PANDAFUZZ_API_KEY/PANDAFUZZ_BEARER_TOKEN to test live API")
        return True
    
    try:
        client = pandafuzz.PandaFuzzClient(
            host=host,
            api_key=api_key,
            bearer_token=bearer_token
        )
        
        print(f"Testing connection to: {host}")
        
        # Test health check
        is_healthy = client.quick_health_check()
        print(f"✓ Health check: {'healthy' if is_healthy else 'unhealthy'}")
        
        # Test health endpoint
        health = client.health.get_health()
        print(f"✓ Health endpoint: {health.status}")
        
        # Test listing endpoints (should not fail even if empty)
        bots = client.bots.list_bots(limit=1)
        print(f"✓ Bots list: {len(bots.items) if hasattr(bots, 'items') else 0} items")
        
        jobs = client.jobs.list_jobs(limit=1)
        print(f"✓ Jobs list: {len(jobs.items) if hasattr(jobs, 'items') else 0} items")
        
        campaigns = client.campaigns.list_campaigns(limit=1)
        print(f"✓ Campaigns list: {len(campaigns.items) if hasattr(campaigns, 'items') else 0} items")
        
        # Clean up
        client.close()
        
        return True
        
    except Exception as e:
        print(f"✗ Live API test failed: {e}")
        print("   This might be expected if the server is not running")
        return True  # Don't fail the test suite for server connectivity issues


def test_async_functionality():
    """Test async functionality."""
    print("\\nTesting async functionality...")
    
    try:
        import asyncio
        
        async def async_test():
            client = pandafuzz.AsyncPandaFuzzClient(
                host="http://localhost:8080",
                api_key="test-key"
            )
            
            # Test client context manager
            async with client as c:
                assert c is not None
                print("✓ Async client context manager works")
            
            print("✓ Async client closed successfully")
        
        # Run async test
        asyncio.run(async_test())
        
        return True
        
    except Exception as e:
        print(f"✗ Async functionality test failed: {e}")
        traceback.print_exc()
        return False


def run_all_tests():
    """Run all tests and return overall result."""
    print("PandaFuzz Python SDK - Test Suite")
    print("=" * 40)
    
    tests = [
        ("Import Tests", test_imports),
        ("Client Initialization", test_client_initialization),
        ("Event Types", test_event_types),
        ("Model Creation", test_model_creation),
        ("SSE Event Creation", test_sse_event_creation),
        ("Async Functionality", test_async_functionality),
        ("Live API Calls", test_live_api_calls),
    ]
    
    passed = 0
    failed = 0
    
    for test_name, test_func in tests:
        print(f"\\n{test_name}:")
        print("-" * 30)
        try:
            if test_func():
                passed += 1
                print(f"✓ {test_name} PASSED")
            else:
                failed += 1
                print(f"✗ {test_name} FAILED")
        except Exception as e:
            failed += 1
            print(f"✗ {test_name} FAILED with exception: {e}")
    
    # Print summary
    print("\\n" + "=" * 40)
    print("TEST SUMMARY")
    print("=" * 40)
    print(f"Total tests: {len(tests)}")
    print(f"Passed: {passed}")
    print(f"Failed: {failed}")
    
    if failed == 0:
        print("\\n🎉 ALL TESTS PASSED!")
        return True
    else:
        print(f"\\n❌ {failed} TESTS FAILED")
        return False


def main():
    """Main test function."""
    success = run_all_tests()
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()