#!/usr/bin/env python3
"""
Async PandaFuzz Python SDK usage examples.

This script demonstrates asynchronous operations using the PandaFuzz async client
for high-performance concurrent operations.
"""

import asyncio
import os
import sys
import time
from typing import List, Dict, Any

import pandafuzz


async def setup_async_client() -> pandafuzz.AsyncPandaFuzzClient:
    """Setup async PandaFuzz client with environment-based configuration."""
    host = os.getenv("PANDAFUZZ_HOST", "http://localhost:8080")
    api_key = os.getenv("PANDAFUZZ_API_KEY")
    bearer_token = os.getenv("PANDAFUZZ_BEARER_TOKEN")
    
    if not (api_key or bearer_token):
        print("Please set either PANDAFUZZ_API_KEY or PANDAFUZZ_BEARER_TOKEN environment variable")
        sys.exit(1)
    
    client = pandafuzz.AsyncPandaFuzzClient(
        host=host,
        api_key=api_key,
        bearer_token=bearer_token,
        timeout=30.0,
        max_retries=3
    )
    
    print(f"Initialized async PandaFuzz client for: {host}")
    return client


async def async_health_check_example(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate async health checking."""
    print("\\n=== Async Health Check Example ===")
    
    # Quick health check
    is_healthy = await client.async_quick_health_check()
    print(f"Quick async health check: {'✓ Healthy' if is_healthy else '✗ Unhealthy'}")
    
    # Concurrent health and readiness checks
    try:
        health_task = client.get_health()
        readiness_task = client.get_readiness()
        
        health, readiness = await asyncio.gather(health_task, readiness_task)
        
        print(f"Health status: {health.get('status', 'unknown')}")
        print(f"Readiness status: {readiness.get('status', 'unknown')}")
        print(f"System version: {health.get('version', 'unknown')}")
    except Exception as e:
        print(f"Async health check failed: {e}")


async def concurrent_bot_queries(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate concurrent bot queries."""
    print("\\n=== Concurrent Bot Queries Example ===")
    
    try:
        # Get bot list first
        bots_response = await client.list_bots(limit=20)
        bot_ids = [bot.get('id') for bot in bots_response.get('items', [])][:10]
        
        if bot_ids:
            print(f"Querying details for {len(bot_ids)} bots concurrently...")
            
            # Create tasks for concurrent bot detail queries
            tasks = [client.get_bot(bot_id) for bot_id in bot_ids]
            
            # Execute all queries concurrently
            start_time = time.time()
            bot_details = await asyncio.gather(*tasks, return_exceptions=True)
            elapsed = time.time() - start_time
            
            print(f"Completed {len(bot_details)} bot queries in {elapsed:.2f} seconds")
            
            # Process results
            successful = 0
            failed = 0
            for i, result in enumerate(bot_details):
                if isinstance(result, Exception):
                    print(f"  Bot {bot_ids[i]} query failed: {result}")
                    failed += 1
                else:
                    successful += 1
                    
            print(f"Results: {successful} successful, {failed} failed")
        else:
            print("No bots found to query")
            
    except Exception as e:
        print(f"Concurrent bot queries failed: {e}")


async def concurrent_job_operations(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate concurrent job operations."""
    print("\\n=== Concurrent Job Operations Example ===")
    
    try:
        # Get recent jobs
        jobs_response = await client.list_jobs(limit=10)
        job_ids = [job.get('id') for job in jobs_response.get('items', [])][:5]
        
        if job_ids:
            print(f"Querying details and logs for {len(job_ids)} jobs concurrently...")
            
            # Create concurrent tasks for job details and logs
            detail_tasks = [client.get_job(job_id) for job_id in job_ids]
            log_tasks = [client.get_job_logs(job_id, limit=10) for job_id in job_ids]
            
            # Execute all tasks concurrently
            start_time = time.time()
            all_tasks = detail_tasks + log_tasks
            results = await asyncio.gather(*all_tasks, return_exceptions=True)
            elapsed = time.time() - start_time
            
            print(f"Completed {len(all_tasks)} job operations in {elapsed:.2f} seconds")
            
            # Process job details
            job_details = results[:len(job_ids)]
            job_logs = results[len(job_ids):]
            
            for i, (detail, logs) in enumerate(zip(job_details, job_logs)):
                if not isinstance(detail, Exception):
                    job_name = detail.get('name', 'unknown')
                    job_status = detail.get('status', 'unknown')
                    log_count = len(logs.get('logs', [])) if not isinstance(logs, Exception) else 0
                    print(f"  Job {job_ids[i]} ({job_name}): {job_status}, {log_count} log entries")
                    
        else:
            print("No jobs found to query")
            
    except Exception as e:
        print(f"Concurrent job operations failed: {e}")


async def async_campaign_monitoring(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate async campaign monitoring."""
    print("\\n=== Async Campaign Monitoring Example ===")
    
    try:
        # Get active campaigns
        campaigns_response = await client.list_campaigns(status="active")
        campaigns = campaigns_response.get('items', [])
        
        if campaigns:
            campaign_id = campaigns[0].get('id')
            campaign_name = campaigns[0].get('name', 'unknown')
            print(f"Monitoring campaign: {campaign_name} ({campaign_id})")
            
            # Monitor campaign for a short duration
            monitoring_duration = 10  # seconds
            start_time = time.time()
            
            async def monitor_campaign():
                """Monitor single campaign stats."""
                async for stats in client.async_monitor_campaign_progress(
                    campaign_id, 
                    poll_interval=2.0
                ):
                    elapsed = time.time() - start_time
                    if elapsed > monitoring_duration:
                        break
                        
                    active_jobs = stats.get('active_jobs', 0)
                    total_jobs = stats.get('total_jobs', 0)
                    crashes = stats.get('total_crashes', 0)
                    coverage = stats.get('coverage_percentage', 0)
                    
                    print(f"  [{elapsed:.1f}s] Jobs: {active_jobs}/{total_jobs}, "
                          f"Crashes: {crashes}, Coverage: {coverage:.1f}%")
            
            await monitor_campaign()
        else:
            print("No active campaigns to monitor")
            
    except Exception as e:
        print(f"Async campaign monitoring failed: {e}")


async def concurrent_analytics_queries(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate concurrent analytics queries."""
    print("\\n=== Concurrent Analytics Queries Example ===")
    
    try:
        # Create multiple analytics queries
        tasks = [
            client.get_analytics(),
            client.get_metrics(),
        ]
        
        # Add campaign-specific analytics if campaigns exist
        campaigns_response = await client.list_campaigns(limit=3)
        campaigns = campaigns_response.get('items', [])[:2]
        
        for campaign in campaigns:
            campaign_id = campaign.get('id')
            if campaign_id:
                tasks.append(client.get_campaign_stats(campaign_id))
        
        print(f"Executing {len(tasks)} analytics queries concurrently...")
        
        # Execute all analytics queries concurrently
        start_time = time.time()
        results = await asyncio.gather(*tasks, return_exceptions=True)
        elapsed = time.time() - start_time
        
        print(f"Completed analytics queries in {elapsed:.2f} seconds")
        
        # Process results
        analytics_result = results[0] if len(results) > 0 else None
        metrics_result = results[1] if len(results) > 1 else None
        campaign_stats = results[2:] if len(results) > 2 else []
        
        if not isinstance(analytics_result, Exception) and analytics_result:
            system_overview = analytics_result.get('system_overview', {})
            print(f"  System overview: {system_overview.get('total_jobs', 0)} jobs, "
                  f"{system_overview.get('total_crashes', 0)} crashes")
        
        if not isinstance(metrics_result, Exception) and metrics_result:
            metrics = metrics_result.get('metrics', {})
            system_metrics = metrics.get('system', {})
            print(f"  System metrics: {system_metrics.get('cpu_usage_percent', 0)}% CPU, "
                  f"{system_metrics.get('memory_usage_percent', 0)}% memory")
        
        # Display campaign stats
        for i, stats in enumerate(campaign_stats):
            if not isinstance(stats, Exception):
                campaign_name = campaigns[i].get('name', f'campaign-{i}')
                active_jobs = stats.get('active_jobs', 0)
                total_crashes = stats.get('total_crashes', 0)
                print(f"  Campaign {campaign_name}: {active_jobs} active jobs, {total_crashes} crashes")
                
    except Exception as e:
        print(f"Concurrent analytics queries failed: {e}")


async def batch_job_creation_example(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate batch job creation (simulated)."""
    print("\\n=== Batch Job Creation Example (Simulated) ===")
    
    # This example shows how you would create multiple jobs concurrently
    # In practice, you'd uncomment this to actually create jobs
    
    job_templates = [
        {
            "name": f"batch-job-{i}",
            "fuzzer_type": "libfuzzer",
            "target_binary": f"/usr/bin/target-{i}",
            "timeout_seconds": 300
        }
        for i in range(5)
    ]
    
    print(f"Would create {len(job_templates)} jobs concurrently...")
    print("Job templates prepared:")
    for template in job_templates:
        print(f"  - {template['name']}: {template['fuzzer_type']}")
    
    # Simulate the concurrent creation (commented out to avoid creating actual jobs)
    """
    try:
        # Create all jobs concurrently
        start_time = time.time()
        tasks = [client.create_job(template) for template in job_templates]
        jobs = await asyncio.gather(*tasks, return_exceptions=True)
        elapsed = time.time() - start_time
        
        print(f"Created {len(jobs)} jobs in {elapsed:.2f} seconds")
        
        # Process results
        successful_jobs = []
        failed_jobs = []
        
        for i, job in enumerate(jobs):
            if isinstance(job, Exception):
                failed_jobs.append((job_templates[i]['name'], job))
            else:
                successful_jobs.append(job)
        
        print(f"Results: {len(successful_jobs)} successful, {len(failed_jobs)} failed")
        
        # Clean up - cancel created jobs
        if successful_jobs:
            cancel_tasks = [client.cancel_job(job['id']) for job in successful_jobs]
            await asyncio.gather(*cancel_tasks, return_exceptions=True)
            print(f"Cancelled {len(successful_jobs)} test jobs")
            
    except Exception as e:
        print(f"Batch job creation failed: {e}")
    """


async def async_event_streaming_demo(client: pandafuzz.AsyncPandaFuzzClient):
    """Demonstrate async event streaming (short demo)."""
    print("\\n=== Async Event Streaming Demo ===")
    
    try:
        print("Starting event stream (monitoring for 5 seconds)...")
        
        event_count = 0
        start_time = time.time()
        
        # Stream events with timeout
        async def stream_with_timeout():
            nonlocal event_count
            async for event in client.stream_events():
                event_count += 1
                print(f"  Event {event_count}: {event.event_type}")
                
                # Stop after 5 seconds or 10 events
                if time.time() - start_time > 5 or event_count >= 10:
                    break
        
        await asyncio.wait_for(stream_with_timeout(), timeout=6.0)
        
    except asyncio.TimeoutError:
        print("Event streaming timeout reached")
    except Exception as e:
        print(f"Event streaming failed: {e}")
    
    print(f"Received {event_count} events during monitoring period")


async def main():
    """Main async example function."""
    print("PandaFuzz Python SDK - Async Operations Examples")
    print("=" * 55)
    
    # Setup async client
    client = await setup_async_client()
    
    try:
        # Run async examples
        await async_health_check_example(client)
        await concurrent_bot_queries(client)
        await concurrent_job_operations(client)
        await async_campaign_monitoring(client)
        await concurrent_analytics_queries(client)
        await batch_job_creation_example(client)
        await async_event_streaming_demo(client)
        
        print("\\n=== Async examples completed successfully! ===")
        
    except KeyboardInterrupt:
        print("\\nAsync examples interrupted by user")
    except Exception as e:
        print(f"\\nAsync example failed with error: {e}")
    finally:
        # Clean up client connections
        await client.close()


if __name__ == "__main__":
    asyncio.run(main())