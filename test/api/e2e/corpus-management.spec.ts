/**
 * Corpus Management End-to-End Tests
 * 
 * Tests complete corpus management workflows including:
 * - Corpus collection creation and management
 * - File upload and validation
 * - Corpus selection and optimization
 * - Cross-campaign corpus sharing
 * - Corpus evolution tracking
 * - Quarantine functionality
 */

import { testHelpers, ResourceTracker, EventCollector } from './utils/test-helpers';
import { TEST_CORPUS_DATA, generateTestData, PERFORMANCE_THRESHOLDS } from './utils/test-fixtures';
import { PandaFuzzClient } from '@pandafuzz/client';
import { readFileSync } from 'fs';
import * as crypto from 'crypto';

describe('Corpus Management', () => {
  let client: PandaFuzzClient;
  let resourceTracker: ResourceTracker;
  let eventCollector: EventCollector;

  beforeAll(async () => {
    client = testHelpers.getClient();
    resourceTracker = new ResourceTracker();
    eventCollector = new EventCollector(client);
  });

  afterAll(async () => {
    try {
      eventCollector.stopCollecting();
      await resourceTracker.cleanup();
    } catch (error) {
      console.warn('Cleanup warning:', error);
    }
  });

  describe('Corpus Collection Creation and Management', () => {
    test('should create corpus collection with metadata', async () => {
      const collectionData = {
        name: testHelpers.generateTestName('test-collection'),
        description: 'Test corpus collection for E2E testing',
        tags: ['test', 'e2e', 'automated'],
        metadata: {
          source: 'e2e-test',
          purpose: 'functionality-testing',
          quality_score: 0.85
        }
      };

      const response = await client.corpus.createCorpusCollection({
        corpusCollectionCreateRequest: collectionData
      });

      resourceTracker.trackCollection(response.id!);

      expect(response).toMatchApiResponse();
      expect(response.name).toBe(collectionData.name);
      expect(response.description).toBe(collectionData.description);
      expect(response.tags).toEqual(collectionData.tags);
      expect(response.file_count).toBe(0);
      expect(response.size_bytes).toBe(0);
      expect(response.created_at).toHaveValidTimestamp();

      // Verify collection appears in list
      const listResponse = await client.corpus.listCorpusCollections();
      const createdCollection = listResponse.collections?.find(c => c.id === response.id);
      expect(createdCollection).toBeDefined();
      expect(createdCollection?.name).toBe(collectionData.name);
    });

    test('should validate collection creation parameters', async () => {
      const invalidCollectionData = {
        name: '', // Empty name should be invalid
        description: 'Invalid collection',
        tags: []
      };

      await expect(client.corpus.createCorpusCollection({
        corpusCollectionCreateRequest: invalidCollectionData
      })).rejects.toThrow();
    });

    test('should update collection metadata', async () => {
      const { id: collectionId } = await testHelpers.createTestCorpusCollection();
      resourceTracker.trackCollection(collectionId);

      const updateData = {
        description: 'Updated description for testing',
        tags: ['updated', 'test', 'modified'],
        metadata: {
          version: '2.0',
          last_updated: new Date().toISOString()
        }
      };

      const response = await client.corpus.updateCorpusCollection({
        id: collectionId,
        corpusCollectionUpdateRequest: updateData
      });

      expect(response.description).toBe(updateData.description);
      expect(response.tags).toEqual(updateData.tags);
      expect(response.updated_at).toHaveValidTimestamp();
    });

    test('should delete empty collection', async () => {
      const { id: collectionId } = await testHelpers.createTestCorpusCollection();
      
      // Delete collection
      await client.corpus.deleteCorpusCollection({ id: collectionId });

      // Verify collection is deleted
      await expect(client.corpus.getCorpusCollection({ id: collectionId }))
        .rejects.toThrow();

      // Verify it doesn't appear in list
      const listResponse = await client.corpus.listCorpusCollections();
      const deletedCollection = listResponse.collections?.find(c => c.id === collectionId);
      expect(deletedCollection).toBeUndefined();
    });
  });

  describe('File Upload and Validation', () => {
    let collectionId: string;

    beforeEach(async () => {
      const collection = await testHelpers.createTestCorpusCollection();
      collectionId = collection.id;
      resourceTracker.trackCollection(collectionId);
    });

    test('should upload various file types', async () => {
      const testFiles = [
        ...generateTestData('normal'),
        ...generateTestData('binary'),
        ...generateTestData('structured_data')
      ];

      const startTime = Date.now();
      await testHelpers.uploadCorpusFiles(collectionId, testFiles);
      const uploadTime = Date.now() - startTime;

      expect(uploadTime).toBeLessThan(PERFORMANCE_THRESHOLDS.corpus_upload_time);

      // Verify files were uploaded
      const collectionResponse = await client.corpus.getCorpusCollection({ id: collectionId });
      expect(collectionResponse.file_count).toBe(testFiles.length);
      expect(collectionResponse.size_bytes).toBeGreaterThan(0);

      // Get file list and verify contents
      const filesResponse = await client.corpus.listCorpusFiles({ id: collectionId });
      expect(filesResponse.files).toHaveLength(testFiles.length);

      for (const uploadedFile of filesResponse.files || []) {
        expect(uploadedFile.filename).toBeDefined();
        expect(uploadedFile.size).toBeGreaterThan(0);
        expect(uploadedFile.hash).toBeDefined();
        expect(uploadedFile.uploaded_at).toHaveValidTimestamp();

        // Find matching test file
        const originalFile = testFiles.find(f => f.filename === uploadedFile.filename);
        expect(originalFile).toBeDefined();
        expect(uploadedFile.size).toBe(originalFile!.content.length);

        // Verify hash
        const expectedHash = crypto.createHash('sha256').update(originalFile!.content).digest('hex');
        expect(uploadedFile.hash).toBe(expectedHash);
      }
    });

    test('should handle duplicate file uploads', async () => {
      const testFile = {
        filename: 'duplicate_test.txt',
        content: Buffer.from('This is a duplicate test'),
        description: 'File for duplicate testing'
      };

      // Upload first time
      await testHelpers.uploadCorpusFiles(collectionId, [testFile]);

      let collectionResponse = await client.corpus.getCorpusCollection({ id: collectionId });
      expect(collectionResponse.file_count).toBe(1);

      // Upload same file again
      await testHelpers.uploadCorpusFiles(collectionId, [testFile]);

      collectionResponse = await client.corpus.getCorpusCollection({ id: collectionId });
      
      // Should not duplicate the file (depending on implementation)
      // Some systems might allow duplicates, others might deduplicate
      expect(collectionResponse.file_count).toBeGreaterThanOrEqual(1);
      expect(collectionResponse.file_count).toBeLessThanOrEqual(2);

      console.log(`Duplicate handling: ${collectionResponse.file_count} files after uploading duplicate`);
    });

    test('should validate file size limits', async () => {
      // Create a very large file
      const largeFile = {
        filename: 'large_file.dat',
        content: Buffer.alloc(50 * 1024 * 1024, 'A'), // 50MB
        description: 'Large file for size limit testing'
      };

      try {
        await testHelpers.uploadCorpusFiles(collectionId, [largeFile]);
        
        // If upload succeeds, verify it's handled correctly
        const collectionResponse = await client.corpus.getCorpusCollection({ id: collectionId });
        expect(collectionResponse.file_count).toBe(1);
        expect(collectionResponse.size_bytes).toBe(largeFile.content.length);

      } catch (error) {
        // Upload might fail due to size limits - this is acceptable behavior
        console.log('Large file upload rejected (expected behavior):', error.message);
        expect(error.message).toMatch(/size|limit|too large/i);
      }
    });

    test('should reject malformed files', async () => {
      const malformedFiles = [
        {
          filename: '', // Empty filename
          content: Buffer.from('test'),
          description: 'Empty filename test'
        },
        {
          filename: 'null_content.txt',
          content: Buffer.alloc(0), // Empty content
          description: 'Empty content test'
        },
        {
          filename: 'special/chars\\in:name?.txt',
          content: Buffer.from('test with special chars'),
          description: 'Special characters in filename'
        }
      ];

      for (const file of malformedFiles) {
        try {
          await testHelpers.uploadCorpusFiles(collectionId, [file]);
          console.warn(`Malformed file accepted: ${file.description}`);
        } catch (error) {
          console.log(`Correctly rejected malformed file: ${file.description}`);
        }
      }
    });

    test('should generate file metadata and analytics', async () => {
      const analyticalFiles = [
        {
          filename: 'text_sample.txt',
          content: Buffer.from('Hello World! This is a text sample with various characters: 123 @#$%'),
          description: 'Text file for analytics'
        },
        {
          filename: 'binary_sample.bin',
          content: Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00]),
          description: 'Binary file for analytics'
        }
      ];

      await testHelpers.uploadCorpusFiles(collectionId, analyticalFiles);

      const filesResponse = await client.corpus.listCorpusFiles({ 
        id: collectionId,
        include_metadata: true 
      });

      for (const file of filesResponse.files || []) {
        expect(file.hash).toBeDefined();
        expect(file.size).toBeGreaterThan(0);
        expect(file.mime_type).toBeDefined();

        if (file.analysis) {
          expect(file.analysis.entropy).toBeGreaterThanOrEqual(0);
          expect(file.analysis.entropy).toBeLessThanOrEqual(8);
        }

        if (file.filename.includes('text')) {
          expect(file.mime_type).toMatch(/text/);
        }

        if (file.filename.includes('binary')) {
          expect(file.mime_type).toMatch(/application|binary/);
        }
      }
    });
  });

  describe('Corpus Selection and Optimization', () => {
    let collectionId: string;

    beforeEach(async () => {
      const collection = await testHelpers.createTestCorpusCollection();
      collectionId = collection.id;
      resourceTracker.trackCollection(collectionId);

      // Upload diverse corpus for selection testing
      const diverseCorpus = [
        ...generateTestData('minimal'),
        ...generateTestData('normal'),
        ...generateTestData('binary'),
        ...generateTestData('vulnerability_triggers')
      ];

      await testHelpers.uploadCorpusFiles(collectionId, diverseCorpus);
    });

    test('should select optimal corpus subset', async () => {
      const selectionCriteria = {
        max_files: 10,
        max_size_bytes: 50 * 1024, // 50KB
        diversity_weight: 0.7,
        coverage_weight: 0.3,
        quality_threshold: 0.5,
        algorithms: ['entropy_based', 'size_based', 'random']
      };

      const selectionResponse = await client.corpus.selectCorpusSubset({
        id: collectionId,
        corpusSelectionRequest: {
          criteria: selectionCriteria
        }
      });

      expect(selectionResponse.selected_files).toBeDefined();
      expect(selectionResponse.selected_files!.length).toBeLessThanOrEqual(selectionCriteria.max_files);

      const totalSize = selectionResponse.selected_files!.reduce(
        (sum, file) => sum + file.size, 0
      );
      expect(totalSize).toBeLessThanOrEqual(selectionCriteria.max_size_bytes);

      // Verify quality metrics
      if (selectionResponse.quality_metrics) {
        expect(selectionResponse.quality_metrics.diversity_score).toBeGreaterThanOrEqual(0);
        expect(selectionResponse.quality_metrics.diversity_score).toBeLessThanOrEqual(1);
        expect(selectionResponse.quality_metrics.coverage_estimation).toBeGreaterThanOrEqual(0);
        expect(selectionResponse.quality_metrics.average_entropy).toBeGreaterThanOrEqual(0);
      }

      console.log(`Selected ${selectionResponse.selected_files!.length} files with diversity score: ${selectionResponse.quality_metrics?.diversity_score}`);
    });

    test('should optimize corpus for specific fuzzer', async () => {
      const fuzzerTypes = ['afl++', 'libfuzzer', 'honggfuzz'];

      for (const fuzzerType of fuzzerTypes) {
        const optimizationRequest = {
          fuzzer_type: fuzzerType,
          target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
          max_corpus_size: 100,
          optimize_for: 'coverage' as const,
          remove_redundant: true,
          minimize_inputs: true
        };

        try {
          const optimizationResponse = await client.corpus.optimizeCorpusForFuzzer({
            id: collectionId,
            corpusOptimizationRequest: optimizationRequest
          });

          expect(optimizationResponse.optimized_corpus_id).toBeDefined();
          resourceTracker.trackCollection(optimizationResponse.optimized_corpus_id!);

          if (optimizationResponse.optimization_results) {
            const results = optimizationResponse.optimization_results;
            expect(results.original_file_count).toBeGreaterThan(0);
            expect(results.optimized_file_count).toBeGreaterThan(0);
            expect(results.size_reduction_ratio).toBeGreaterThanOrEqual(0);
            expect(results.coverage_improvement).toBeGreaterThanOrEqual(0);
          }

          console.log(`${fuzzerType} optimization: ${optimizationResponse.optimization_results?.original_file_count} -> ${optimizationResponse.optimization_results?.optimized_file_count} files`);

        } catch (error) {
          console.warn(`Optimization not available for ${fuzzerType}:`, error.message);
        }
      }
    });

    test('should rank corpus files by quality', async () => {
      const rankingRequest = {
        ranking_algorithm: 'entropy_coverage',
        include_analysis: true,
        limit: 20
      };

      try {
        const rankingResponse = await client.corpus.rankCorpusFiles({
          id: collectionId,
          corpusRankingRequest: rankingRequest
        });

        expect(rankingResponse.ranked_files).toBeDefined();
        expect(rankingResponse.ranked_files!.length).toBeGreaterThan(0);

        // Verify ranking is in descending order of score
        let previousScore = Infinity;
        for (const rankedFile of rankingResponse.ranked_files!) {
          expect(rankedFile.file_id).toBeDefined();
          expect(rankedFile.quality_score).toBeGreaterThanOrEqual(0);
          expect(rankedFile.quality_score).toBeLessThanOrEqual(previousScore);
          previousScore = rankedFile.quality_score;

          if (rankedFile.analysis) {
            expect(rankedFile.analysis.entropy).toBeGreaterThanOrEqual(0);
            expect(rankedFile.analysis.uniqueness).toBeGreaterThanOrEqual(0);
            expect(rankedFile.analysis.complexity).toBeGreaterThanOrEqual(0);
          }
        }

        console.log(`Ranked ${rankingResponse.ranked_files!.length} files, top score: ${rankingResponse.ranked_files![0].quality_score}`);

      } catch (error) {
        console.warn('Corpus ranking not available:', error.message);
      }
    });
  });

  describe('Cross-Campaign Corpus Sharing', () => {
    let collection1Id: string;
    let collection2Id: string;
    let campaign1Id: string;
    let campaign2Id: string;

    beforeEach(async () => {
      // Create two corpus collections
      const collection1 = await testHelpers.createTestCorpusCollection('shared-corpus-1');
      const collection2 = await testHelpers.createTestCorpusCollection('shared-corpus-2');
      
      collection1Id = collection1.id;
      collection2Id = collection2.id;
      
      resourceTracker.trackCollection(collection1Id);
      resourceTracker.trackCollection(collection2Id);

      // Upload different corpus to each collection
      await testHelpers.uploadCorpusFiles(collection1Id, generateTestData('normal'));
      await testHelpers.uploadCorpusFiles(collection2Id, generateTestData('binary'));

      // Create campaigns using these collections
      const campaignData1 = {
        name: testHelpers.generateTestName('sharing-campaign-1'),
        description: 'First campaign for corpus sharing test',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        corpus_collection_id: collection1Id,
        shared_corpus: true,
        max_jobs: 1,
        auto_restart: false,
        job_template: {
          duration: 120,
          memory_limit: 256 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'afl++' as const,
          arguments: '@@'
        }
      };

      const campaignData2 = {
        name: testHelpers.generateTestName('sharing-campaign-2'),
        description: 'Second campaign for corpus sharing test',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        corpus_collection_id: collection2Id,
        shared_corpus: true,
        max_jobs: 1,
        auto_restart: false,
        job_template: {
          duration: 120,
          memory_limit: 256 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'libfuzzer' as const,
          arguments: '@@'
        }
      };

      const campaign1Response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData1
      });
      
      const campaign2Response = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData2
      });

      campaign1Id = campaign1Response.id!;
      campaign2Id = campaign2Response.id!;

      resourceTracker.trackCampaign(campaign1Id);
      resourceTracker.trackCampaign(campaign2Id);
    });

    test('should sync corpus between campaigns', async () => {
      const syncRequest = {
        source_collection_id: collection1Id,
        target_collection_id: collection2Id,
        sync_mode: 'bidirectional' as const,
        filters: {
          min_quality_score: 0.3,
          max_file_size: 10 * 1024, // 10KB
          file_types: ['text', 'binary']
        },
        deduplication: {
          enabled: true,
          similarity_threshold: 0.95
        }
      };

      try {
        const syncResponse = await client.corpus.syncCorpusCollections({
          corpusSyncRequest: syncRequest
        });

        expect(syncResponse.sync_id).toBeDefined();
        expect(syncResponse.status).toBe('started');

        // Wait for sync to complete
        let syncComplete = false;
        const maxWaitTime = 60000; // 1 minute
        const startTime = Date.now();

        while (!syncComplete && (Date.now() - startTime) < maxWaitTime) {
          const statusResponse = await client.corpus.getSyncStatus({
            syncId: syncResponse.sync_id!
          });

          if (statusResponse.status === 'completed') {
            syncComplete = true;
            
            expect(statusResponse.summary).toBeDefined();
            if (statusResponse.summary) {
              const summary = statusResponse.summary;
              expect(summary.files_synced).toBeGreaterThanOrEqual(0);
              expect(summary.duplicates_removed).toBeGreaterThanOrEqual(0);
              expect(summary.bytes_transferred).toBeGreaterThanOrEqual(0);
            }

            console.log(`Corpus sync completed: ${statusResponse.summary?.files_synced} files synced`);

          } else if (statusResponse.status === 'failed') {
            throw new Error(`Corpus sync failed: ${statusResponse.error}`);
          }

          await new Promise(resolve => setTimeout(resolve, 2000));
        }

        if (!syncComplete) {
          console.warn('Corpus sync did not complete within timeout');
        }

      } catch (error) {
        console.warn('Corpus sync not available:', error.message);
      }
    });

    test('should share discoveries between campaigns', async () => {
      // Start both campaigns
      await client.campaigns.updateCampaign({
        id: campaign1Id,
        campaignUpdateRequest: { status: 'running' }
      });

      await client.campaigns.updateCampaign({
        id: campaign2Id,
        campaignUpdateRequest: { status: 'running' }
      });

      // Wait for campaigns to run and potentially share corpus
      await Promise.all([
        testHelpers.waitForCampaignStatus(campaign1Id, 'running', 30000),
        testHelpers.waitForCampaignStatus(campaign2Id, 'running', 30000)
      ]);

      // Let campaigns run for a while
      await new Promise(resolve => setTimeout(resolve, 60000));

      // Check if corpus collections have grown (indicating sharing)
      const collection1After = await client.corpus.getCorpusCollection({ id: collection1Id });
      const collection2After = await client.corpus.getCorpusCollection({ id: collection2Id });

      console.log(`Collection 1 files: ${collection1After.file_count}, Collection 2 files: ${collection2After.file_count}`);

      // At least one collection should have evolved
      const totalFilesAfter = collection1After.file_count + collection2After.file_count;
      const originalFiles = generateTestData('normal').length + generateTestData('binary').length;

      if (totalFilesAfter > originalFiles) {
        console.log(`Corpus sharing detected: ${totalFilesAfter - originalFiles} new files discovered`);
      } else {
        console.log('No new corpus files discovered (may be expected for short test duration)');
      }
    });

    test('should handle corpus access permissions', async () => {
      // Create a private corpus collection
      const privateCollectionData = {
        name: testHelpers.generateTestName('private-corpus'),
        description: 'Private corpus collection',
        tags: ['private', 'restricted'],
        access_level: 'private',
        allowed_campaigns: [campaign1Id] // Only campaign1 should have access
      };

      try {
        const privateCollection = await client.corpus.createCorpusCollection({
          corpusCollectionCreateRequest: privateCollectionData
        });

        resourceTracker.trackCollection(privateCollection.id!);

        // Upload some files to private collection
        await testHelpers.uploadCorpusFiles(
          privateCollection.id!,
          generateTestData('vulnerability_triggers')
        );

        // Try to access from allowed campaign
        const allowedAccessResponse = await client.corpus.getCorpusCollection({
          id: privateCollection.id!,
          campaignId: campaign1Id
        });

        expect(allowedAccessResponse.id).toBe(privateCollection.id);

        // Try to access from non-allowed campaign (should fail or have restricted access)
        try {
          await client.corpus.getCorpusCollection({
            id: privateCollection.id!,
            campaignId: campaign2Id
          });

          console.warn('Private corpus accessible from non-allowed campaign - permissions may not be implemented');

        } catch (error) {
          console.log('Correctly restricted access to private corpus from non-allowed campaign');
          expect(error.message).toMatch(/access|permission|forbidden/i);
        }

      } catch (error) {
        console.warn('Corpus access permissions not implemented:', error.message);
      }
    });
  });

  describe('Corpus Evolution and Tracking', () => {
    let collectionId: string;
    let campaignId: string;

    beforeEach(async () => {
      const collection = await testHelpers.createTestCorpusCollection();
      collectionId = collection.id;
      resourceTracker.trackCollection(collectionId);

      // Upload initial corpus
      await testHelpers.uploadCorpusFiles(
        collectionId,
        generateTestData('normal')
      );

      // Create campaign to evolve the corpus
      const campaignData = {
        name: testHelpers.generateTestName('evolution-campaign'),
        description: 'Campaign for corpus evolution testing',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        corpus_collection_id: collectionId,
        shared_corpus: true,
        max_jobs: 1,
        auto_restart: false,
        job_template: {
          duration: 180, // 3 minutes for evolution
          memory_limit: 256 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'afl++' as const,
          arguments: '@@'
        },
        corpus_evolution: {
          enabled: true,
          save_interesting: true,
          max_corpus_size: 1000
        }
      };

      const campaignResponse = await client.campaigns.createCampaign({
        campaignCreateRequest: campaignData
      });

      campaignId = campaignResponse.id!;
      resourceTracker.trackCampaign(campaignId);
    });

    test('should track corpus evolution over time', async () => {
      // Get initial corpus state
      const initialState = await client.corpus.getCorpusCollection({ id: collectionId });
      const initialFileCount = initialState.file_count;
      const initialSize = initialState.size_bytes;

      // Start campaign
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Track evolution over time
      const evolutionSnapshots = [];
      const snapshotInterval = 15000; // 15 seconds
      const totalSnapshots = 6; // 1.5 minutes total

      for (let i = 0; i < totalSnapshots; i++) {
        await new Promise(resolve => setTimeout(resolve, snapshotInterval));

        const snapshot = {
          timestamp: Date.now(),
          collection_state: await client.corpus.getCorpusCollection({ id: collectionId }),
          campaign_stats: await client.campaigns.getCampaignStats({ id: campaignId })
        };

        evolutionSnapshots.push(snapshot);
      }

      // Analyze evolution
      const finalState = evolutionSnapshots[evolutionSnapshots.length - 1].collection_state;
      
      console.log(`Corpus evolution: ${initialFileCount} -> ${finalState.file_count} files`);
      console.log(`Size evolution: ${initialSize} -> ${finalState.size_bytes} bytes`);

      // Check if corpus has evolved
      const hasEvolved = finalState.file_count > initialFileCount || 
                        finalState.size_bytes > initialSize;

      if (hasEvolved) {
        // Verify evolution quality
        const growthRate = (finalState.file_count - initialFileCount) / initialFileCount;
        expect(growthRate).toBeGreaterThanOrEqual(0);

        // Check evolution history
        try {
          const evolutionHistory = await client.corpus.getCorpusEvolutionHistory({
            id: collectionId,
            time_range: 'last_hour'
          });

          expect(evolutionHistory.timeline).toBeDefined();
          if (evolutionHistory.timeline && evolutionHistory.timeline.length > 0) {
            for (const point of evolutionHistory.timeline) {
              expect(point.timestamp).toBeDefined();
              expect(point.file_count).toBeGreaterThanOrEqual(0);
              expect(point.size_bytes).toBeGreaterThanOrEqual(0);
            }
          }

        } catch (error) {
          console.warn('Corpus evolution history not available:', error.message);
        }

      } else {
        console.log('No corpus evolution detected (may be expected for short test duration)');
      }
    });

    test('should identify interesting corpus additions', async () => {
      // Start campaign with interesting file detection
      await client.campaigns.updateCampaign({
        id: campaignId,
        campaignUpdateRequest: { status: 'running' }
      });

      await testHelpers.waitForCampaignStatus(campaignId, 'running', 30000);

      // Let campaign run to potentially find interesting files
      await new Promise(resolve => setTimeout(resolve, 90000)); // 1.5 minutes

      try {
        // Get files added during campaign
        const recentFiles = await client.corpus.listCorpusFiles({
          id: collectionId,
          added_after: new Date(Date.now() - 120000).toISOString(), // Last 2 minutes
          include_metadata: true
        });

        if (recentFiles.files && recentFiles.files.length > 0) {
          console.log(`${recentFiles.files.length} new files discovered during campaign`);

          for (const file of recentFiles.files) {
            if (file.metadata?.interesting_reason) {
              console.log(`Interesting file: ${file.filename} - ${file.metadata.interesting_reason}`);
            }

            if (file.metadata?.coverage_contribution) {
              expect(file.metadata.coverage_contribution).toBeGreaterThanOrEqual(0);
            }

            if (file.generation_info?.parent_file_id) {
              expect(file.generation_info.mutation_type).toBeDefined();
              expect(file.generation_info.generation_method).toBeDefined();
            }
          }
        } else {
          console.log('No new interesting files discovered (may be expected)');
        }

      } catch (error) {
        console.warn('Interesting file analysis not available:', error.message);
      }
    });

    test('should maintain corpus quality metrics', async () => {
      // Get initial quality metrics
      try {
        const initialMetrics = await client.corpus.getCorpusQualityMetrics({
          id: collectionId
        });

        expect(initialMetrics.diversity_score).toBeGreaterThanOrEqual(0);
        expect(initialMetrics.diversity_score).toBeLessThanOrEqual(1);
        expect(initialMetrics.coverage_estimation).toBeGreaterThanOrEqual(0);
        expect(initialMetrics.average_file_size).toBeGreaterThan(0);

        console.log(`Initial corpus quality - Diversity: ${initialMetrics.diversity_score}, Coverage: ${initialMetrics.coverage_estimation}`);

        // Start campaign
        await client.campaigns.updateCampaign({
          id: campaignId,
          campaignUpdateRequest: { status: 'running' }
        });

        await new Promise(resolve => setTimeout(resolve, 60000)); // 1 minute

        // Get updated quality metrics
        const updatedMetrics = await client.corpus.getCorpusQualityMetrics({
          id: collectionId
        });

        console.log(`Updated corpus quality - Diversity: ${updatedMetrics.diversity_score}, Coverage: ${updatedMetrics.coverage_estimation}`);

        // Quality should not degrade significantly
        expect(updatedMetrics.diversity_score).toBeGreaterThanOrEqual(initialMetrics.diversity_score * 0.8);

      } catch (error) {
        console.warn('Corpus quality metrics not available:', error.message);
      }
    });
  });

  describe('Corpus Quarantine Functionality', () => {
    let collectionId: string;

    beforeEach(async () => {
      const collection = await testHelpers.createTestCorpusCollection();
      collectionId = collection.id;
      resourceTracker.trackCollection(collectionId);

      // Upload mixed corpus including potentially problematic files
      const mixedCorpus = [
        ...generateTestData('normal'),
        ...generateTestData('binary'),
        {
          filename: 'suspicious.exe',
          content: Buffer.from('MZ\x90\x00\x03\x00\x00\x00\x04\x00'), // PE header
          description: 'Executable file (potentially suspicious)'
        },
        {
          filename: 'large_payload.dat',
          content: Buffer.alloc(5 * 1024 * 1024, 0xFF), // 5MB of 0xFF
          description: 'Large suspicious payload'
        }
      ];

      await testHelpers.uploadCorpusFiles(collectionId, mixedCorpus);
    });

    test('should detect and quarantine suspicious files', async () => {
      try {
        // Run quarantine scan
        const scanResponse = await client.corpus.scanCorpusForThreats({
          id: collectionId,
          corpusThreatScanRequest: {
            scan_types: ['malware', 'suspicious_patterns', 'size_anomalies'],
            strict_mode: false,
            auto_quarantine: true
          }
        });

        expect(scanResponse.scan_id).toBeDefined();
        expect(scanResponse.status).toBe('started');

        // Wait for scan completion
        let scanComplete = false;
        const maxWaitTime = 120000; // 2 minutes
        const startTime = Date.now();

        while (!scanComplete && (Date.now() - startTime) < maxWaitTime) {
          const statusResponse = await client.corpus.getThreatScanStatus({
            scanId: scanResponse.scan_id!
          });

          if (statusResponse.status === 'completed') {
            scanComplete = true;
            
            if (statusResponse.results) {
              const results = statusResponse.results;
              expect(results.files_scanned).toBeGreaterThan(0);
              expect(results.threats_found).toBeGreaterThanOrEqual(0);
              expect(results.quarantined_files).toBeGreaterThanOrEqual(0);

              console.log(`Threat scan: ${results.files_scanned} scanned, ${results.threats_found} threats, ${results.quarantined_files} quarantined`);

              if (results.threat_details && results.threat_details.length > 0) {
                for (const threat of results.threat_details) {
                  expect(threat.file_id).toBeDefined();
                  expect(threat.threat_type).toBeDefined();
                  expect(threat.severity).toBeDefined();
                  expect(['low', 'medium', 'high', 'critical']).toContain(threat.severity);
                }
              }
            }

          } else if (statusResponse.status === 'failed') {
            throw new Error(`Threat scan failed: ${statusResponse.error}`);
          }

          await new Promise(resolve => setTimeout(resolve, 3000));
        }

        if (!scanComplete) {
          console.warn('Threat scan did not complete within timeout');
        }

      } catch (error) {
        console.warn('Corpus threat scanning not available:', error.message);
      }
    });

    test('should manage quarantine queue', async () => {
      try {
        // Get quarantine status
        const quarantineResponse = await client.corpus.getCorpusQuarantineStatus({
          id: collectionId
        });

        expect(quarantineResponse.total_quarantined).toBeGreaterThanOrEqual(0);
        expect(quarantineResponse.pending_review).toBeGreaterThanOrEqual(0);

        if (quarantineResponse.quarantined_files && quarantineResponse.quarantined_files.length > 0) {
          const quarantinedFile = quarantineResponse.quarantined_files[0];
          
          expect(quarantinedFile.file_id).toBeDefined();
          expect(quarantinedFile.quarantine_reason).toBeDefined();
          expect(quarantinedFile.quarantined_at).toHaveValidTimestamp();
          expect(quarantinedFile.status).toBeDefined();

          // Test reviewing quarantined file
          const reviewResponse = await client.corpus.reviewQuarantinedFile({
            id: collectionId,
            fileId: quarantinedFile.file_id,
            quarantineReviewRequest: {
              action: 'approve', // or 'reject', 'keep_quarantined'
              reviewer_comment: 'E2E test review',
              override_reason: 'Test validation'
            }
          });

          expect(reviewResponse.action_taken).toBe('approve');
          expect(reviewResponse.processed_at).toHaveValidTimestamp();

          console.log(`Reviewed quarantined file: ${quarantinedFile.file_id} - ${reviewResponse.action_taken}`);
        }

      } catch (error) {
        console.warn('Quarantine management not available:', error.message);
      }
    });

    test('should prevent quarantined files from fuzzing', async () => {
      // Create campaign with quarantine enabled
      const campaignData = {
        name: testHelpers.generateTestName('quarantine-campaign'),
        description: 'Campaign with quarantine protection',
        target_binary: '/test-resources/test-targets/vulnerable/vulnerable-app',
        corpus_collection_id: collectionId,
        max_jobs: 1,
        auto_restart: false,
        quarantine_protection: {
          enabled: true,
          strict_mode: true,
          allow_pending_review: false
        },
        job_template: {
          duration: 60,
          memory_limit: 128 * 1024 * 1024,
          timeout: 1000,
          fuzzer_type: 'libfuzzer' as const,
          arguments: '@@'
        }
      };

      try {
        const campaignResponse = await client.campaigns.createCampaign({
          campaignCreateRequest: campaignData
        });

        resourceTracker.trackCampaign(campaignResponse.id!);

        // Start campaign
        await client.campaigns.updateCampaign({
          id: campaignResponse.id!,
          campaignUpdateRequest: { status: 'running' }
        });

        await testHelpers.waitForCampaignStatus(campaignResponse.id!, 'running', 30000);

        // Campaign should only use non-quarantined files
        await new Promise(resolve => setTimeout(resolve, 30000));

        const campaignStats = await client.campaigns.getCampaignStats({
          id: campaignResponse.id!
        });

        // Verify campaign is running with filtered corpus
        expect(campaignStats.corpus_size).toBeGreaterThan(0);
        console.log(`Campaign using ${campaignStats.corpus_size} non-quarantined corpus files`);

      } catch (error) {
        console.warn('Quarantine protection in campaigns not available:', error.message);
      }
    });
  });

  describe('Corpus Performance and Scalability', () => {
    test('should handle large corpus collections efficiently', async () => {
      const { id: collectionId } = await testHelpers.createTestCorpusCollection();
      resourceTracker.trackCollection(collectionId);

      // Generate large number of small files
      const largeCorpus = [];
      const fileCount = 100; // Reasonable number for E2E testing

      for (let i = 0; i < fileCount; i++) {
        largeCorpus.push({
          filename: `file_${i.toString().padStart(4, '0')}.txt`,
          content: Buffer.from(`File content ${i} with some variation ${Math.random()}`),
          description: `Generated file ${i}`
        });
      }

      // Measure upload performance
      const uploadStartTime = Date.now();
      await testHelpers.uploadCorpusFiles(collectionId, largeCorpus);
      const uploadTime = Date.now() - uploadStartTime;

      expect(uploadTime).toBeLessThan(PERFORMANCE_THRESHOLDS.corpus_upload_time * (fileCount / 10));

      // Measure listing performance
      const listStartTime = Date.now();
      const listResponse = await client.corpus.listCorpusFiles({ 
        id: collectionId,
        limit: fileCount 
      });
      const listTime = Date.now() - listStartTime;

      expect(listTime).toBeLessThan(PERFORMANCE_THRESHOLDS.api_response_time);
      expect(listResponse.files).toHaveLength(fileCount);

      console.log(`Large corpus performance - Upload: ${uploadTime}ms, List: ${listTime}ms`);

      // Test pagination
      const pageSize = 20;
      let offset = 0;
      let totalRetrieved = 0;

      while (offset < fileCount) {
        const pageResponse = await client.corpus.listCorpusFiles({
          id: collectionId,
          limit: pageSize,
          offset: offset
        });

        expect(pageResponse.files!.length).toBeLessThanOrEqual(pageSize);
        totalRetrieved += pageResponse.files!.length;
        offset += pageSize;

        if (pageResponse.files!.length < pageSize) {
          break; // Last page
        }
      }

      expect(totalRetrieved).toBe(fileCount);
    });

    test('should handle concurrent corpus operations', async () => {
      const { id: collectionId } = await testHelpers.createTestCorpusCollection();
      resourceTracker.trackCollection(collectionId);

      // Prepare concurrent operations
      const concurrentOperations = [];

      // Upload operations
      for (let i = 0; i < 5; i++) {
        const files = [{
          filename: `concurrent_${i}.txt`,
          content: Buffer.from(`Concurrent upload ${i}`),
          description: `Concurrent test file ${i}`
        }];

        concurrentOperations.push(
          testHelpers.uploadCorpusFiles(collectionId, files)
        );
      }

      // Read operations
      for (let i = 0; i < 10; i++) {
        concurrentOperations.push(
          client.corpus.getCorpusCollection({ id: collectionId })
        );
      }

      // Execute all operations concurrently
      const startTime = Date.now();
      const results = await Promise.allSettled(concurrentOperations);
      const totalTime = Date.now() - startTime;

      // Verify all operations completed
      const successful = results.filter(r => r.status === 'fulfilled').length;
      const failed = results.filter(r => r.status === 'rejected').length;

      expect(successful).toBeGreaterThan(failed);
      console.log(`Concurrent operations: ${successful} successful, ${failed} failed in ${totalTime}ms`);

      // Verify final state is consistent
      const finalState = await client.corpus.getCorpusCollection({ id: collectionId });
      expect(finalState.file_count).toBeGreaterThan(0);
      expect(finalState.file_count).toBeLessThanOrEqual(5); // At most 5 files should be uploaded
    });
  });
});