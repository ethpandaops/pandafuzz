#!/usr/bin/env python3
"""
Real-time event streaming examples for PandaFuzz Python SDK.

This script demonstrates various event streaming patterns including:
- Basic event streaming
- Event filtering and subscription
- Async event handling
- Event callbacks and processing
"""

import asyncio
import os
import sys
import time
import signal
from typing import Dict, Any, List, Callable

import pandafuzz


class EventProcessor:
    """Event processor with statistics and filtering capabilities."""
    
    def __init__(self):
        self.event_counts = {}
        self.total_events = 0
        self.start_time = time.time()
        self.last_event_time = None
        
    def process_event(self, event: pandafuzz.SSEEvent):
        """Process incoming event and update statistics."""
        self.total_events += 1
        self.last_event_time = time.time()
        
        # Count events by type
        self.event_counts[event.event_type] = self.event_counts.get(event.event_type, 0) + 1
        
        # Print event with timestamp
        elapsed = time.time() - self.start_time
        print(f"[{elapsed:6.1f}s] {event.event_type}: {self.format_event_data(event.data)}")
    
    def format_event_data(self, data: Any) -> str:
        """Format event data for display."""
        if isinstance(data, dict):
            # Extract key information based on event type
            if 'job_id' in data:
                job_info = f"job_id={data['job_id']}"
                if 'status' in data:
                    job_info += f", status={data['status']}"
                if 'progress' in data:
                    job_info += f", progress={data['progress']}%"
                return job_info
            elif 'bot_id' in data:
                bot_info = f"bot_id={data['bot_id']}"
                if 'status' in data:
                    bot_info += f", status={data['status']}"
                return bot_info
            elif 'campaign_id' in data:
                campaign_info = f"campaign_id={data['campaign_id']}"
                if 'status' in data:
                    campaign_info += f", status={data['status']}"
                return campaign_info
            elif 'crash_id' in data:
                crash_info = f"crash_id={data['crash_id']}"
                if 'severity' in data:
                    crash_info += f", severity={data['severity']}"
                return crash_info
            else:
                # Generic dict formatting
                items = []
                for key, value in list(data.items())[:3]:  # Show first 3 items
                    items.append(f"{key}={value}")
                return ', '.join(items)
        else:
            return str(data)[:100]  # Limit string length
    
    def print_statistics(self):
        """Print event processing statistics."""
        elapsed = time.time() - self.start_time
        print(f"\\n--- Event Statistics ({elapsed:.1f}s) ---")
        print(f"Total events: {self.total_events}")
        
        if self.event_counts:
            print("Events by type:")
            for event_type, count in sorted(self.event_counts.items()):
                print(f"  {event_type}: {count}")
        
        if self.total_events > 0:
            rate = self.total_events / elapsed
            print(f"Average rate: {rate:.2f} events/second")


def setup_client():
    """Setup PandaFuzz client for event streaming."""
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
    
    print(f"Initialized PandaFuzz client for event streaming: {host}")
    return client


def basic_event_streaming_example(client: pandafuzz.PandaFuzzClient, duration: int = 30):
    """Demonstrate basic event streaming."""
    print(f"\\n=== Basic Event Streaming ({duration}s) ===")
    
    processor = EventProcessor()
    start_time = time.time()
    
    try:
        print("Starting event stream...")
        for event in client.stream_all_events():
            processor.process_event(event)
            
            # Stop after specified duration
            if time.time() - start_time > duration:
                break
                
    except KeyboardInterrupt:
        print("\\nEvent streaming interrupted by user")
    except Exception as e:
        print(f"Event streaming failed: {e}")
    
    processor.print_statistics()


def filtered_event_streaming_example(client: pandafuzz.PandaFuzzClient, duration: int = 20):
    """Demonstrate event filtering and specific subscriptions."""
    print(f"\\n=== Filtered Event Streaming ({duration}s) ===")
    
    # Job-specific event processor
    job_processor = EventProcessor()
    
    def job_event_callback(event: pandafuzz.SSEEvent):
        job_processor.process_event(event)
    
    try:
        print("Subscribing to job-related events...")
        
        # Stream only job events
        start_time = time.time()
        for event in client.stream_job_events():
            job_event_callback(event)
            
            if time.time() - start_time > duration:
                break
                
    except KeyboardInterrupt:
        print("\\nFiltered streaming interrupted by user")
    except Exception as e:
        print(f"Filtered streaming failed: {e}")
    
    print("Job events:")
    job_processor.print_statistics()


def crash_event_monitoring_example(client: pandafuzz.PandaFuzzClient, duration: int = 15):
    """Demonstrate crash event monitoring with detailed processing."""
    print(f"\\n=== Crash Event Monitoring ({duration}s) ===")
    
    crash_events = []
    
    def crash_callback(event: pandafuzz.SSEEvent):
        crash_events.append(event)
        crash_data = event.data
        
        print(f"🚨 CRASH DETECTED!")
        print(f"   Event: {event.event_type}")
        print(f"   Crash ID: {crash_data.get('crash_id', 'unknown')}")
        print(f"   Job ID: {crash_data.get('job_id', 'unknown')}")
        print(f"   Severity: {crash_data.get('severity', 'unknown')}")
        print(f"   Type: {crash_data.get('crash_type', 'unknown')}")
        print()
    
    try:
        print("Monitoring for crash discovery events...")
        
        start_time = time.time()
        for event in client.stream_crash_events():
            crash_callback(event)
            
            if time.time() - start_time > duration:
                break
                
    except KeyboardInterrupt:
        print("\\nCrash monitoring interrupted by user")
    except Exception as e:
        print(f"Crash monitoring failed: {e}")
    
    print(f"Total crash events detected: {len(crash_events)}")


async def async_event_streaming_example(client: pandafuzz.PandaFuzzClient, duration: int = 25):
    """Demonstrate async event streaming with concurrent processing."""
    print(f"\\n=== Async Event Streaming ({duration}s) ===")
    
    processor = EventProcessor()
    
    async def event_callback(event: pandafuzz.SSEEvent):
        """Async event callback with simulated processing delay."""
        processor.process_event(event)
        
        # Simulate some async processing
        if event.event_type in [pandafuzz.PandaFuzzEventTypes.CRASH_DISCOVERED]:
            await asyncio.sleep(0.1)  # Simulate crash analysis delay
    
    try:
        print("Starting async event streaming...")
        
        # Use async client for event streaming
        async_client = client.async_client
        
        start_time = time.time()
        
        async for event in async_client.stream_events():
            # Process event asynchronously
            asyncio.create_task(event_callback(event))
            
            if time.time() - start_time > duration:
                break
                
        # Give callbacks time to complete
        await asyncio.sleep(0.5)
        
    except KeyboardInterrupt:
        print("\\nAsync streaming interrupted by user")
    except Exception as e:
        print(f"Async streaming failed: {e}")
    
    print("Async event processing:")
    processor.print_statistics()


def multi_subscription_example(client: pandafuzz.PandaFuzzClient, duration: int = 20):
    """Demonstrate multiple concurrent event subscriptions."""
    print(f"\\n=== Multiple Event Subscriptions ({duration}s) ===")
    
    processors = {
        'job': EventProcessor(),
        'bot': EventProcessor(),
        'crash': EventProcessor(),
        'campaign': EventProcessor()
    }
    
    def create_callback(category: str):
        def callback(event: pandafuzz.SSEEvent):
            processors[category].process_event(event)
        return callback
    
    try:
        print("Starting multiple event subscriptions...")
        print("- Job events")
        print("- Bot events") 
        print("- Crash events")
        print("- Campaign events")
        
        # This example shows the concept - in practice you'd need to implement
        # actual concurrent subscription handling
        start_time = time.time()
        
        # Use the general event stream and filter by type
        for event in client.stream_all_events():
            event_type = event.event_type
            
            if event_type in pandafuzz.PandaFuzzEventTypes.all_job_events():
                create_callback('job')(event)
            elif event_type in pandafuzz.PandaFuzzEventTypes.all_bot_events():
                create_callback('bot')(event)
            elif event_type in pandafuzz.PandaFuzzEventTypes.all_crash_events():
                create_callback('crash')(event)
            elif event_type in pandafuzz.PandaFuzzEventTypes.all_campaign_events():
                create_callback('campaign')(event)
            
            if time.time() - start_time > duration:
                break
                
    except KeyboardInterrupt:
        print("\\nMultiple subscriptions interrupted by user")
    except Exception as e:
        print(f"Multiple subscriptions failed: {e}")
    
    # Print statistics for each category
    for category, processor in processors.items():
        if processor.total_events > 0:
            print(f"\\n{category.title()} events:")
            processor.print_statistics()


def event_filtering_demo(client: pandafuzz.PandaFuzzClient, duration: int = 15):
    """Demonstrate advanced event filtering."""
    print(f"\\n=== Advanced Event Filtering Demo ({duration}s) ===")
    
    high_priority_events = []
    
    def is_high_priority_event(event: pandafuzz.SSEEvent) -> bool:
        """Determine if an event is high priority."""
        # High priority events
        high_priority_types = [
            pandafuzz.PandaFuzzEventTypes.CRASH_DISCOVERED,
            pandafuzz.PandaFuzzEventTypes.JOB_FAILED,
            pandafuzz.PandaFuzzEventTypes.BOT_DISCONNECTED,
            pandafuzz.PandaFuzzEventTypes.CAMPAIGN_COMPLETED,
            pandafuzz.PandaFuzzEventTypes.SYSTEM_ALERT
        ]
        
        if event.event_type in high_priority_types:
            return True
        
        # Check for high severity crashes
        if hasattr(event.data, 'severity') and event.data.severity == 'high':
            return True
            
        return False
    
    try:
        print("Filtering for high-priority events only...")
        
        start_time = time.time()
        all_events = 0
        
        for event in client.stream_all_events():
            all_events += 1
            
            if is_high_priority_event(event):
                high_priority_events.append(event)
                print(f"🔥 HIGH PRIORITY: {event.event_type}")
                
                # Show additional details for high priority events
                if hasattr(event.data, 'job_id'):
                    print(f"   Job ID: {event.data.job_id}")
                if hasattr(event.data, 'campaign_id'):
                    print(f"   Campaign ID: {event.data.campaign_id}")
            
            if time.time() - start_time > duration:
                break
                
    except KeyboardInterrupt:
        print("\\nEvent filtering interrupted by user")
    except Exception as e:
        print(f"Event filtering failed: {e}")
    
    print(f"\\nFiltering results:")
    print(f"Total events processed: {all_events}")
    print(f"High priority events: {len(high_priority_events)}")
    if all_events > 0:
        priority_ratio = len(high_priority_events) / all_events * 100
        print(f"Priority ratio: {priority_ratio:.1f}%")


def main():
    """Main event streaming examples function."""
    print("PandaFuzz Python SDK - Event Streaming Examples")
    print("=" * 55)
    
    # Setup client
    client = setup_client()
    
    # Setup signal handler for graceful shutdown
    def signal_handler(signum, frame):
        print(f"\\nReceived signal {signum}, shutting down gracefully...")
        sys.exit(0)
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    try:
        # Run event streaming examples with shorter durations for demo
        basic_event_streaming_example(client, duration=10)
        filtered_event_streaming_example(client, duration=8)
        crash_event_monitoring_example(client, duration=5)
        
        # Async example
        print("\\nRunning async example...")
        asyncio.run(async_event_streaming_example(client, duration=8))
        
        multi_subscription_example(client, duration=8)
        event_filtering_demo(client, duration=5)
        
        print("\\n=== Event streaming examples completed! ===")
        
    except KeyboardInterrupt:
        print("\\nExamples interrupted by user")
    except Exception as e:
        print(f"\\nExample failed with error: {e}")
    finally:
        # Clean up client connections
        client.close()


if __name__ == "__main__":
    main()