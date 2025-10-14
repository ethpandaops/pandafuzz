#!/usr/bin/env python3
"""
Basic PandaFuzz Python SDK usage examples.

This script demonstrates fundamental operations using the PandaFuzz Python client
including health checks, job management, and basic campaign operations.
"""

import os
import sys
import time
from typing import Optional

import pandafuzz


def setup_client() -> pandafuzz.PandaFuzzClient:
    """Setup PandaFuzz client with environment-based configuration."""
    host = os.getenv("PANDAFUZZ_HOST", "http://localhost:8080")
    api_key = os.getenv("PANDAFUZZ_API_KEY")
    bearer_token = os.getenv("PANDAFUZZ_BEARER_TOKEN")
    
    if not (api_key or bearer_token):
        print("Please set either PANDAFUZZ_API_KEY or PANDAFUZZ_BEARER_TOKEN environment variable")
        sys.exit(1)
    
    client = pandafuzz.PandaFuzzClient(
        host=host,
        api_key=api_key,
        bearer_token=bearer_token
    )
    
    print(f"Initialized PandaFuzz client for: {host}")
    return client


def health_check_example(client: pandafuzz.PandaFuzzClient):
    """Demonstrate health checking."""
    print("\\n=== Health Check Example ===")
    
    # Quick health check
    is_healthy = client.quick_health_check()
    print(f"Quick health check: {'✓ Healthy' if is_healthy else '✗ Unhealthy'}")
    
    # Detailed health information
    try:
        health = client.health.get_health()
        print(f"Detailed health status: {health.status}")
        print(f"System version: {health.version}")
        print(f"Uptime: {health.uptime}")
        
        # Check individual components
        for component, details in health.checks.items():
            print(f"  {component}: {details.status}")
    except Exception as e:
        print(f"Health check failed: {e}")
    
    # Readiness check
    try:
        readiness = client.health.get_readiness()
        print(f"Readiness status: {readiness.status}")
    except Exception as e:
        print(f"Readiness check failed: {e}")


def bot_management_example(client: pandafuzz.PandaFuzzClient):
    """Demonstrate bot management operations."""
    print("\\n=== Bot Management Example ===")
    
    # List all bots
    try:
        bots = client.bots.list_bots()
        print(f"Total registered bots: {len(bots.items) if hasattr(bots, 'items') else 0}")
        
        if hasattr(bots, 'items') and bots.items:
            print("Active bots:")
            for bot in bots.items[:5]:  # Show first 5 bots
                print(f"  - {bot.name} ({bot.id}): {bot.status}")
                print(f"    Capabilities: {', '.join(bot.capabilities) if bot.capabilities else 'None'}")
                print(f"    Active jobs: {bot.active_jobs}/{bot.max_concurrent_jobs}")
    except Exception as e:
        print(f"Failed to list bots: {e}")
    
    # Create a test bot (optional - uncomment if you want to test bot creation)
    """
    try:
        new_bot = client.bots.create_bot({
            "name": "example-bot",
            "capabilities": ["libfuzzer", "afl++"],
            "max_concurrent_jobs": 2
        })
        print(f"Created bot: {new_bot.id}")
        
        # Clean up - delete the test bot
        client.bots.delete_bot(new_bot.id)
        print(f"Deleted test bot: {new_bot.id}")
    except Exception as e:
        print(f"Bot creation/deletion failed: {e}")
    """


def job_management_example(client: pandafuzz.PandaFuzzClient):
    """Demonstrate job management operations."""
    print("\\n=== Job Management Example ===")
    
    # List existing jobs
    try:
        jobs = client.jobs.list_jobs(limit=10)
        print(f"Recent jobs: {len(jobs.items) if hasattr(jobs, 'items') else 0}")
        
        if hasattr(jobs, 'items') and jobs.items:
            print("Job status summary:")
            for job in jobs.items:
                print(f"  - {job.name} ({job.id}): {job.status}")
                print(f"    Fuzzer: {job.fuzzer_type}, Started: {job.created_at}")
    except Exception as e:
        print(f"Failed to list jobs: {e}")
    
    # Create a simple job (commented out to avoid creating actual jobs)
    """
    try:
        job = client.create_simple_job(
            name="sdk-example-job",
            fuzzer_type="libfuzzer",
            target_binary="/usr/bin/test-target",
            duration_minutes=5  # Short duration for testing
        )
        print(f"Created job: {job['id']}")
        
        # Monitor job for a short time
        print("Monitoring job progress...")
        timeout = time.time() + 30  # Monitor for 30 seconds max
        
        while time.time() < timeout:
            current_job = client.jobs.get_job(job['id'])
            print(f"Job status: {current_job.status}")
            
            if current_job.status in ['completed', 'failed', 'cancelled']:
                break
            
            time.sleep(2)
        
        # Get job logs
        logs = client.jobs.get_job_logs(job['id'], limit=20)
        print(f"Recent logs ({len(logs.logs) if hasattr(logs, 'logs') else 0} entries):")
        if hasattr(logs, 'logs'):
            for log in logs.logs[-5:]:  # Show last 5 logs
                print(f"  [{log.timestamp}] {log.level}: {log.message[:100]}...")
        
    except Exception as e:
        print(f"Job management failed: {e}")
    """


def campaign_management_example(client: pandafuzz.PandaFuzzClient):
    """Demonstrate campaign management operations."""
    print("\\n=== Campaign Management Example ===")
    
    # List existing campaigns
    try:
        campaigns = client.campaigns.list_campaigns(limit=10)
        print(f"Active campaigns: {len(campaigns.items) if hasattr(campaigns, 'items') else 0}")
        
        if hasattr(campaigns, 'items') and campaigns.items:
            print("Campaign summary:")
            for campaign in campaigns.items:
                print(f"  - {campaign.name} ({campaign.id}): {campaign.status}")
                
                # Get campaign statistics
                try:
                    stats = client.campaigns.get_campaign_stats(campaign.id)
                    print(f"    Jobs: {stats.active_jobs}/{stats.total_jobs}")
                    print(f"    Crashes: {stats.total_crashes}")
                except Exception as e:
                    print(f"    Stats unavailable: {e}")
    except Exception as e:
        print(f"Failed to list campaigns: {e}")


def analytics_example(client: pandafuzz.PandaFuzzClient):
    """Demonstrate analytics and metrics retrieval."""
    print("\\n=== Analytics Example ===")
    
    # Get system metrics
    try:
        metrics = client.analytics.get_metrics()
        print("System metrics:")
        
        if hasattr(metrics, 'metrics'):
            if hasattr(metrics.metrics, 'system'):
                system = metrics.metrics.system
                print(f"  System CPU: {system.cpu_usage_percent}%")
                print(f"  System Memory: {system.memory_usage_percent}%")
                print(f"  Disk Usage: {system.disk_usage_percent}%")
            
            if hasattr(metrics.metrics, 'jobs'):
                jobs = metrics.metrics.jobs
                print(f"  Active Jobs: {jobs.active}")
                print(f"  Completed Jobs: {jobs.completed}")
                print(f"  Failed Jobs: {jobs.failed}")
            
            if hasattr(metrics.metrics, 'bots'):
                bots = metrics.metrics.bots
                print(f"  Active Bots: {bots.active}")
                print(f"  Idle Bots: {bots.idle}")
    except Exception as e:
        print(f"Failed to get metrics: {e}")
    
    # Get analytics data
    try:
        analytics = client.analytics.get_analytics()
        print("\\nSystem analytics:")
        
        if hasattr(analytics, 'system_overview'):
            overview = analytics.system_overview
            print(f"  Total Jobs: {overview.total_jobs}")
            print(f"  Total Campaigns: {overview.total_campaigns}")
            print(f"  Total Crashes: {overview.total_crashes}")
    except Exception as e:
        print(f"Failed to get analytics: {e}")


def crash_summary_example(client: pandafuzz.PandaFuzzClient):
    """Demonstrate crash analysis operations."""
    print("\\n=== Crash Analysis Example ===")
    
    try:
        # Get crash summary using convenience method
        summary = client.get_crash_summary(limit=20)
        print(f"Crash summary:")
        print(f"  Total crashes: {summary['total_crashes']}")
        
        if summary['by_severity']:
            print("  By severity:")
            for severity, count in summary['by_severity'].items():
                print(f"    {severity}: {count}")
        
        if summary['by_type']:
            print("  By type:")
            for crash_type, count in summary['by_type'].items():
                print(f"    {crash_type}: {count}")
                
        # List recent crashes
        if hasattr(summary['recent_crashes'], 'items'):
            print("\\n  Recent crashes:")
            for crash in summary['recent_crashes'].items[:5]:
                print(f"    - {crash.id}: {crash.crash_type} ({crash.severity})")
                print(f"      Discovered: {crash.discovered_at}")
    except Exception as e:
        print(f"Failed to get crash summary: {e}")


def main():
    """Main example function."""
    print("PandaFuzz Python SDK - Basic Usage Examples")
    print("=" * 50)
    
    # Setup client
    client = setup_client()
    
    try:
        # Run examples
        health_check_example(client)
        bot_management_example(client)
        job_management_example(client)
        campaign_management_example(client)
        analytics_example(client)
        crash_summary_example(client)
        
        print("\\n=== Examples completed successfully! ===")
        
    except KeyboardInterrupt:
        print("\\nExamples interrupted by user")
    except Exception as e:
        print(f"\\nExample failed with error: {e}")
    finally:
        # Clean up client connections
        client.close()


if __name__ == "__main__":
    main()