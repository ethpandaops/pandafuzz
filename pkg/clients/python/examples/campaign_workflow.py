#!/usr/bin/env python3
"""
Complete campaign workflow example for PandaFuzz Python SDK.

This script demonstrates a complete fuzzing campaign lifecycle including:
- Campaign creation and configuration
- Job management and monitoring
- Corpus handling
- Crash analysis
- Results collection and reporting
"""

import asyncio
import os
import sys
import time
from typing import Dict, Any, List, Optional
from pathlib import Path

import pandafuzz


class CampaignManager:
    """Manages a complete fuzzing campaign workflow."""
    
    def __init__(self, client: pandafuzz.PandaFuzzClient):
        self.client = client
        self.campaign_id: Optional[str] = None
        self.job_ids: List[str] = []
        self.start_time: Optional[float] = None
        self.results: Dict[str, Any] = {}
        
    def create_campaign(
        self,
        name: str,
        target_binary: str,
        fuzzer_type: str = "libfuzzer",
        duration_hours: int = 1,
        max_jobs: int = 3
    ) -> Dict[str, Any]:
        """Create a new fuzzing campaign."""
        print(f"\\n=== Creating Campaign: {name} ===")
        
        try:
            campaign = self.client.create_simple_campaign(
                name=name,
                fuzzer_type=fuzzer_type,
                target_binary=target_binary,
                job_count=max_jobs,
                duration_hours=duration_hours
            )
            
            self.campaign_id = campaign['id']
            print(f"✓ Campaign created successfully!")
            print(f"  Campaign ID: {self.campaign_id}")
            print(f"  Name: {campaign.get('name', name)}")
            print(f"  Max parallel jobs: {max_jobs}")
            print(f"  Duration: {duration_hours} hours")
            print(f"  Fuzzer: {fuzzer_type}")
            
            return campaign
            
        except Exception as e:
            print(f"✗ Failed to create campaign: {e}")
            raise
    
    def start_campaign(self):
        """Start the campaign execution."""
        if not self.campaign_id:
            raise ValueError("No campaign created yet")
        
        print(f"\\n=== Starting Campaign {self.campaign_id} ===")
        
        try:
            self.client.campaigns.start_campaign(self.campaign_id)
            self.start_time = time.time()
            print("✓ Campaign started successfully!")
            
        except Exception as e:
            print(f"✗ Failed to start campaign: {e}")
            raise
    
    def monitor_campaign_progress(self, monitoring_duration: int = 60):
        """Monitor campaign progress for a specified duration."""
        if not self.campaign_id:
            raise ValueError("No campaign to monitor")
        
        print(f"\\n=== Monitoring Campaign Progress ({monitoring_duration}s) ===")
        
        try:
            monitor_start = time.time()
            last_update = 0
            
            for stats in self.client.monitor_campaign_progress(
                self.campaign_id, 
                poll_interval=5.0
            ):
                elapsed = time.time() - monitor_start
                
                # Update every 10 seconds or on significant changes
                if elapsed - last_update >= 10 or self._is_significant_change(stats):
                    self._print_progress_update(stats, elapsed)
                    last_update = elapsed
                
                # Store latest results
                self.results.update(stats)
                
                # Stop monitoring after specified duration
                if elapsed > monitoring_duration:
                    print(f"\\nMonitoring timeout reached ({monitoring_duration}s)")
                    break
                
                # Check if campaign is complete
                if stats.get('status') in ['completed', 'stopped', 'failed']:
                    print(f"\\n✓ Campaign {stats.get('status')}")
                    break
                    
        except KeyboardInterrupt:
            print("\\nMonitoring interrupted by user")
        except Exception as e:
            print(f"Monitoring failed: {e}")
    
    def _is_significant_change(self, stats: Dict[str, Any]) -> bool:
        """Check if there's a significant change worth reporting."""
        if not self.results:
            return True
        
        # Check for new crashes
        old_crashes = self.results.get('total_crashes', 0)
        new_crashes = stats.get('total_crashes', 0)
        if new_crashes > old_crashes:
            return True
        
        # Check for job status changes
        old_active = self.results.get('active_jobs', 0)
        new_active = stats.get('active_jobs', 0)
        if abs(new_active - old_active) > 0:
            return True
        
        return False
    
    def _print_progress_update(self, stats: Dict[str, Any], elapsed: float):
        """Print a formatted progress update."""
        print(f"\\n[{elapsed:6.1f}s] Campaign Progress:")
        print(f"  Status: {stats.get('status', 'unknown')}")
        print(f"  Active Jobs: {stats.get('active_jobs', 0)}/{stats.get('total_jobs', 0)}")
        print(f"  Completed Jobs: {stats.get('completed_jobs', 0)}")
        print(f"  Failed Jobs: {stats.get('failed_jobs', 0)}")
        print(f"  Total Crashes: {stats.get('total_crashes', 0)}")
        print(f"  Coverage: {stats.get('coverage_percentage', 0):.1f}%")
        
        # Show performance metrics if available
        if 'performance_metrics' in stats:
            perf = stats['performance_metrics']
            print(f"  Exec/sec: {perf.get('executions_per_second', 0):.1f}")
            print(f"  CPU Usage: {perf.get('cpu_usage_percent', 0):.1f}%")
    
    def collect_campaign_jobs(self):
        """Collect all jobs associated with the campaign."""
        if not self.campaign_id:
            return
        
        print(f"\\n=== Collecting Campaign Jobs ===")
        
        try:
            jobs = self.client.jobs.list_jobs(campaign_id=self.campaign_id)
            
            if hasattr(jobs, 'items'):
                self.job_ids = [job.id for job in jobs.items]
                print(f"Found {len(self.job_ids)} jobs in campaign:")
                
                for job in jobs.items:
                    print(f"  - {job.name} ({job.id}): {job.status}")
                    print(f"    Created: {job.created_at}")
                    print(f"    Fuzzer: {job.fuzzer_type}")
                    if hasattr(job, 'progress') and job.progress:
                        print(f"    Progress: {job.progress.percentage}%")
            else:
                print("No jobs found for campaign")
                
        except Exception as e:
            print(f"Failed to collect campaign jobs: {e}")
    
    def analyze_crashes(self):
        """Analyze crashes discovered during the campaign."""
        print(f"\\n=== Crash Analysis ===")
        
        try:
            # Get campaign-specific crashes
            crashes = self.client.crashes.list_crashes(
                campaign_id=self.campaign_id,
                limit=50
            )
            
            if hasattr(crashes, 'items') and crashes.items:
                print(f"Found {len(crashes.items)} crashes:")
                
                # Analyze crash distribution
                by_severity = {}
                by_type = {}
                by_job = {}
                
                for crash in crashes.items:
                    # Count by severity
                    severity = getattr(crash, 'severity', 'unknown')
                    by_severity[severity] = by_severity.get(severity, 0) + 1
                    
                    # Count by type
                    crash_type = getattr(crash, 'crash_type', 'unknown')
                    by_type[crash_type] = by_type.get(crash_type, 0) + 1
                    
                    # Count by job
                    job_id = getattr(crash, 'job_id', 'unknown')
                    by_job[job_id] = by_job.get(job_id, 0) + 1
                
                # Print analysis
                print("\\nCrash distribution:")
                print("  By severity:")
                for severity, count in sorted(by_severity.items()):
                    print(f"    {severity}: {count}")
                
                print("  By type:")
                for crash_type, count in sorted(by_type.items()):
                    print(f"    {crash_type}: {count}")
                
                print("  By job:")
                for job_id, count in sorted(by_job.items()):
                    job_name = self._get_job_name(job_id)
                    print(f"    {job_name} ({job_id[:8]}...): {count}")
                
                # Show detailed info for high-severity crashes
                high_severity_crashes = [
                    c for c in crashes.items 
                    if getattr(c, 'severity', '').lower() in ['high', 'critical']
                ]
                
                if high_severity_crashes:
                    print(f"\\nHigh-severity crashes ({len(high_severity_crashes)}):")
                    for crash in high_severity_crashes[:3]:  # Show first 3
                        print(f"  - {crash.id}:")
                        print(f"    Type: {crash.crash_type}")
                        print(f"    Severity: {crash.severity}")
                        if hasattr(crash, 'crash_info') and crash.crash_info:
                            print(f"    Signal: {crash.crash_info.signal}")
                            if crash.crash_info.stack_trace:
                                # Show first line of stack trace
                                first_line = crash.crash_info.stack_trace.split('\\n')[0]
                                print(f"    Stack: {first_line[:80]}...")
                        print()
                
            else:
                print("No crashes found for this campaign")
                
        except Exception as e:
            print(f"Crash analysis failed: {e}")
    
    def _get_job_name(self, job_id: str) -> str:
        """Get job name by ID."""
        try:
            job = self.client.jobs.get_job(job_id)
            return getattr(job, 'name', 'unknown')
        except:
            return 'unknown'
    
    def collect_corpus_info(self):
        """Collect corpus information for the campaign."""
        print(f"\\n=== Corpus Information ===")
        
        try:
            corpus = self.client.corpus.list_corpus(
                campaign_id=self.campaign_id,
                limit=100
            )
            
            if hasattr(corpus, 'items') and corpus.items:
                print(f"Corpus entries: {len(corpus.items)}")
                
                # Analyze corpus statistics
                total_size = 0
                coverage_entries = 0
                generation_types = {}
                
                for entry in corpus.items:
                    if hasattr(entry, 'size'):
                        total_size += entry.size
                    
                    if hasattr(entry, 'coverage_info') and entry.coverage_info:
                        coverage_entries += 1
                    
                    if hasattr(entry, 'generation_info') and entry.generation_info:
                        gen_type = entry.generation_info.method
                        generation_types[gen_type] = generation_types.get(gen_type, 0) + 1
                
                print(f"Total corpus size: {total_size / 1024 / 1024:.2f} MB")
                print(f"Entries with coverage info: {coverage_entries}")
                
                if generation_types:
                    print("Generation methods:")
                    for method, count in sorted(generation_types.items()):
                        print(f"  {method}: {count}")
                
            else:
                print("No corpus entries found for this campaign")
                
        except Exception as e:
            print(f"Corpus collection failed: {e}")
    
    def generate_final_report(self):
        """Generate a final campaign report."""
        print(f"\\n=== Final Campaign Report ===")
        
        if not self.campaign_id:
            print("No campaign to report on")
            return
        
        try:
            # Get final campaign stats
            final_stats = self.client.campaigns.get_campaign_stats(self.campaign_id)
            campaign_info = self.client.campaigns.get_campaign(self.campaign_id)
            
            print(f"Campaign: {campaign_info.name} ({self.campaign_id})")
            print(f"Status: {campaign_info.status}")
            print(f"Duration: {self._format_duration()}")
            
            print(f"\\nExecution Summary:")
            print(f"  Total Jobs: {final_stats.get('total_jobs', 0)}")
            print(f"  Completed Jobs: {final_stats.get('completed_jobs', 0)}")
            print(f"  Failed Jobs: {final_stats.get('failed_jobs', 0)}")
            print(f"  Success Rate: {self._calculate_success_rate(final_stats):.1f}%")
            
            print(f"\\nSecurity Findings:")
            print(f"  Total Crashes: {final_stats.get('total_crashes', 0)}")
            print(f"  Unique Crashes: {final_stats.get('unique_crashes', 0)}")
            print(f"  High Severity: {final_stats.get('high_severity_crashes', 0)}")
            
            print(f"\\nCoverage Metrics:")
            print(f"  Final Coverage: {final_stats.get('coverage_percentage', 0):.2f}%")
            print(f"  New Edges: {final_stats.get('new_edges', 0)}")
            
            if 'performance_metrics' in final_stats:
                perf = final_stats['performance_metrics']
                print(f"\\nPerformance Metrics:")
                print(f"  Avg Exec/sec: {perf.get('avg_executions_per_second', 0):.1f}")
                print(f"  Total Executions: {perf.get('total_executions', 0):,}")
                print(f"  Peak CPU Usage: {perf.get('peak_cpu_usage_percent', 0):.1f}%")
                print(f"  Peak Memory Usage: {perf.get('peak_memory_usage_mb', 0):.1f} MB")
            
            print(f"\\nRecommendations:")
            self._generate_recommendations(final_stats)
            
        except Exception as e:
            print(f"Report generation failed: {e}")
    
    def _format_duration(self) -> str:
        """Format campaign duration."""
        if not self.start_time:
            return "unknown"
        
        duration = time.time() - self.start_time
        hours = int(duration // 3600)
        minutes = int((duration % 3600) // 60)
        seconds = int(duration % 60)
        
        if hours > 0:
            return f"{hours}h {minutes}m {seconds}s"
        elif minutes > 0:
            return f"{minutes}m {seconds}s"
        else:
            return f"{seconds}s"
    
    def _calculate_success_rate(self, stats: Dict[str, Any]) -> float:
        """Calculate job success rate."""
        total = stats.get('total_jobs', 0)
        completed = stats.get('completed_jobs', 0)
        
        if total == 0:
            return 0.0
        
        return (completed / total) * 100
    
    def _generate_recommendations(self, stats: Dict[str, Any]):
        """Generate actionable recommendations based on results."""
        recommendations = []
        
        # Coverage recommendations
        coverage = stats.get('coverage_percentage', 0)
        if coverage < 50:
            recommendations.append("Consider increasing campaign duration or adding more diverse corpus")
        elif coverage > 90:
            recommendations.append("Excellent coverage achieved - consider fuzzing different targets")
        
        # Crash recommendations
        total_crashes = stats.get('total_crashes', 0)
        if total_crashes == 0:
            recommendations.append("No crashes found - target may be robust or needs different fuzzing approach")
        elif total_crashes > 100:
            recommendations.append("High crash rate detected - prioritize crash analysis and fixing")
        
        # Performance recommendations
        if 'performance_metrics' in stats:
            perf = stats['performance_metrics']
            exec_rate = perf.get('avg_executions_per_second', 0)
            if exec_rate < 100:
                recommendations.append("Low execution rate - consider optimizing target or fuzzer config")
        
        # Job failure recommendations
        failed_jobs = stats.get('failed_jobs', 0)
        total_jobs = stats.get('total_jobs', 1)
        if failed_jobs / total_jobs > 0.2:
            recommendations.append("High job failure rate - check bot health and resource availability")
        
        if recommendations:
            for i, rec in enumerate(recommendations, 1):
                print(f"  {i}. {rec}")
        else:
            print("  Campaign executed successfully with no specific recommendations")
    
    def cleanup_campaign(self):
        """Clean up campaign resources."""
        print(f"\\n=== Campaign Cleanup ===")
        
        if not self.campaign_id:
            print("No campaign to clean up")
            return
        
        try:
            # Stop campaign if still running
            campaign = self.client.campaigns.get_campaign(self.campaign_id)
            if campaign.status in ['running', 'active']:
                print("Stopping active campaign...")
                self.client.campaigns.stop_campaign(
                    self.campaign_id, 
                    reason="Example completed"
                )
            
            print("✓ Campaign cleanup completed")
            
        except Exception as e:
            print(f"Cleanup failed: {e}")


def main():
    """Main campaign workflow example."""
    print("PandaFuzz Python SDK - Complete Campaign Workflow")
    print("=" * 60)
    
    # Setup client
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
    
    # Initialize campaign manager
    campaign_manager = CampaignManager(client)
    
    try:
        # Check system health before starting
        if not client.quick_health_check():
            print("⚠️ System health check failed - proceeding anyway for demo")
        else:
            print("✓ System health check passed")
        
        # Example campaign configuration
        campaign_config = {
            'name': 'SDK-Example-Campaign',
            'target_binary': '/usr/bin/example-target',  # Replace with actual target
            'fuzzer_type': 'libfuzzer',
            'duration_hours': 1,  # Short duration for demo
            'max_jobs': 2
        }
        
        print(f"\\nConfiguring campaign with:")
        for key, value in campaign_config.items():
            print(f"  {key}: {value}")
        
        # Note: This example shows the workflow but doesn't create actual campaigns
        # to avoid resource usage. Uncomment the following sections to run actual campaigns.
        
        print("\\n" + "="*60)
        print("DEMO MODE: Actual campaign creation is commented out")
        print("Uncomment the sections below to run a real campaign")
        print("="*60)
        
        """
        # Execute campaign workflow
        campaign_manager.create_campaign(**campaign_config)
        campaign_manager.start_campaign()
        
        # Monitor progress (shorter duration for demo)
        campaign_manager.monitor_campaign_progress(monitoring_duration=30)
        
        # Collect results
        campaign_manager.collect_campaign_jobs()
        campaign_manager.analyze_crashes()
        campaign_manager.collect_corpus_info()
        
        # Generate final report
        campaign_manager.generate_final_report()
        """
        
        # Show what a real workflow would look like
        print("\\nA complete workflow would include:")
        print("1. Campaign creation and configuration")
        print("2. Campaign execution and monitoring")
        print("3. Job status tracking and management") 
        print("4. Real-time crash discovery and analysis")
        print("5. Corpus growth and quality metrics")
        print("6. Performance monitoring and optimization")
        print("7. Final reporting and recommendations")
        
        print("\\n=== Workflow example completed! ===")
        
    except KeyboardInterrupt:
        print("\\nWorkflow interrupted by user")
        campaign_manager.cleanup_campaign()
    except Exception as e:
        print(f"\\nWorkflow failed with error: {e}")
        campaign_manager.cleanup_campaign()
    finally:
        # Clean up client connections
        client.close()


if __name__ == "__main__":
    main()