# Transcription Benchmarks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Go benchmarks measuring transcription speed (RTF) and accuracy (WER) for both Whisper and Parakeet backends.

**Architecture:** WER computation as an exported utility in the transcribe package, benchmark functions using `testing.B` with sub-benchmarks per audio sample, LibriSpeech test audio committed to testdata/.

**Tech Stack:** Go testing/benchmarking, go-audio/wav, LibriSpeech (public domain audio)

---

### Task 1: WER Implementation + Tests

**Files:**
- Create: `internal/transcribe/wer.go`
- Create: `internal/transcribe/wer_test.go`

**Step 1:** Write table-driven tests for ComputeWER covering: identical strings, one substitution, one insertion, one deletion, case insensitivity, punctuation stripping, empty reference, empty hypothesis, completely different strings.

**Step 2:** Run tests to verify they fail (ComputeWER not defined).

**Step 3:** Implement ComputeWER with normalize + min edit distance DP + backtrace.

**Step 4:** Run tests to verify they pass.

**Step 5:** Commit.

---

### Task 2: Prepare Test Audio & References

**Files:**
- Create: `internal/transcribe/testdata/references.json`
- Download: LibriSpeech WAV clips to `internal/transcribe/testdata/`

**Step 1:** Create testdata directory.

**Step 2:** Download 3 LibriSpeech clips (~3s, ~8s, ~14s) from test-clean dataset. Convert to 16kHz mono WAV if needed.

**Step 3:** Create references.json with ground truth transcripts for all samples (including JFK).

**Step 4:** Commit.

---

### Task 3: Audio Loading Helper

**Files:**
- Modify: `internal/transcribe/whisper_test.go`

**Step 1:** Add test for generic loadWAVSamples function.

**Step 2:** Run test to verify it fails.

**Step 3:** Extract generic loadWAVSamples helper from existing jfkSamples, refactor jfkSamples to use it.

**Step 4:** Run all existing tests to verify nothing broke.

**Step 5:** Commit.

---

### Task 4: Benchmark Functions

**Files:**
- Create: `internal/transcribe/benchmark_test.go`

**Step 1:** Write benchmark file with BenchmarkWhisperProcess and BenchmarkParakeetProcess, each with sub-benchmarks per audio sample, reporting RTF, WER, and audio-ms metrics.

**Step 2:** Run benchmarks to verify they work.

**Step 3:** Commit.

---

### Task 5: Taskfile Integration

**Files:**
- Modify: `Taskfile.yml`

**Step 1:** Add `bench` task.

**Step 2:** Verify `task bench` works.

**Step 3:** Commit.

---

### Task 6: Verify & Clean Up

**Step 1:** Run full test suite (`task test`).

**Step 2:** Run benchmarks end-to-end (`task bench`).

**Step 3:** Final commit if cleanup needed.
