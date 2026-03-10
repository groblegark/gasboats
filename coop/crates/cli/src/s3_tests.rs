// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Alfred Jean LLC

use std::collections::HashMap;
use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use tokio::sync::broadcast;
use tokio_util::sync::CancellationToken;

use super::*;

// ---------------------------------------------------------------------------
// MockS3 — in-memory S3Storage for testing
// ---------------------------------------------------------------------------

struct MockS3 {
    objects: parking_lot::Mutex<HashMap<String, Vec<u8>>>,
}

impl MockS3 {
    fn new() -> Self {
        Self { objects: parking_lot::Mutex::new(HashMap::new()) }
    }

    /// Pre-load an object into the mock store.
    fn put(&self, session_id: &str, s3_path: &str, data: Vec<u8>) {
        let key = Self::mock_key(session_id, s3_path);
        self.objects.lock().insert(key, data);
    }

    /// Read an object from the mock store (for assertions).
    fn get(&self, session_id: &str, s3_path: &str) -> Option<Vec<u8>> {
        let key = Self::mock_key(session_id, s3_path);
        self.objects.lock().get(&key).cloned()
    }

    /// Number of stored objects.
    fn object_count(&self) -> usize {
        self.objects.lock().len()
    }

    /// All stored keys.
    fn keys(&self) -> Vec<String> {
        self.objects.lock().keys().cloned().collect()
    }

    fn mock_key(session_id: &str, s3_path: &str) -> String {
        format!("{session_id}/{s3_path}")
    }
}

impl S3Storage for MockS3 {
    async fn upload_file(
        &self,
        session_id: &str,
        s3_path: &str,
        local_path: &Path,
    ) -> anyhow::Result<()> {
        let data = tokio::fs::read(local_path).await?;
        let key = Self::mock_key(session_id, s3_path);
        self.objects.lock().insert(key, data);
        Ok(())
    }

    async fn upload_bytes(
        &self,
        session_id: &str,
        s3_path: &str,
        data: Vec<u8>,
    ) -> anyhow::Result<()> {
        let key = Self::mock_key(session_id, s3_path);
        self.objects.lock().insert(key, data);
        Ok(())
    }

    async fn download_file(
        &self,
        session_id: &str,
        s3_path: &str,
        local_path: &Path,
    ) -> anyhow::Result<()> {
        let key = Self::mock_key(session_id, s3_path);
        let data = {
            let objects = self.objects.lock();
            objects.get(&key).cloned().ok_or_else(|| anyhow::anyhow!("object not found: {key}"))?
        };
        if let Some(parent) = local_path.parent() {
            tokio::fs::create_dir_all(parent).await?;
        }
        tokio::fs::write(local_path, data).await?;
        Ok(())
    }

    async fn list_transcripts(&self, session_id: &str) -> anyhow::Result<Vec<u32>> {
        let objects = self.objects.lock();
        let prefix = format!("{session_id}/transcripts/");
        let mut numbers = Vec::new();
        for key in objects.keys() {
            if let Some(rest) = key.strip_prefix(&prefix) {
                if let Some(num_str) = rest.strip_suffix(".jsonl") {
                    if let Ok(n) = num_str.parse::<u32>() {
                        numbers.push(n);
                    }
                }
            }
        }
        numbers.sort();
        Ok(numbers)
    }

    async fn upload_meta(&self, session_id: &str, meta: &SessionMeta) -> anyhow::Result<()> {
        let json = serde_json::to_vec_pretty(meta)?;
        self.upload_bytes(session_id, "meta.json", json).await
    }

    async fn download_meta(&self, session_id: &str) -> anyhow::Result<Option<SessionMeta>> {
        let key = Self::mock_key(session_id, "meta.json");
        let objects = self.objects.lock();
        match objects.get(&key) {
            Some(data) => {
                let meta: SessionMeta = serde_json::from_slice(data)?;
                Ok(Some(meta))
            }
            None => Ok(None),
        }
    }

    async fn exists(&self, session_id: &str, s3_path: &str) -> bool {
        let key = Self::mock_key(session_id, s3_path);
        self.objects.lock().contains_key(&key)
    }

    async fn list_sessions(&self) -> anyhow::Result<Vec<String>> {
        let objects = self.objects.lock();
        let mut session_ids = std::collections::BTreeSet::new();
        for key in objects.keys() {
            // Keys are "session_id/path" — extract session_id.
            if let Some(sid) = key.split('/').next() {
                session_ids.insert(sid.to_owned());
            }
        }
        Ok(session_ids.into_iter().collect())
    }

    async fn list_recording_chunks(&self, session_id: &str) -> anyhow::Result<Vec<String>> {
        let objects = self.objects.lock();
        let prefix = format!("{session_id}/recording/");
        let mut chunks = Vec::new();
        for key in objects.keys() {
            if let Some(rest) = key.strip_prefix(&prefix) {
                if rest.ends_with(".jsonl") {
                    chunks.push(rest.to_owned());
                }
            }
        }
        chunks.sort();
        Ok(chunks)
    }

    async fn download_bytes(&self, session_id: &str, s3_path: &str) -> anyhow::Result<Vec<u8>> {
        let key = Self::mock_key(session_id, s3_path);
        let objects = self.objects.lock();
        objects.get(&key).cloned().ok_or_else(|| anyhow::anyhow!("object not found: {key}"))
    }
}

// ---------------------------------------------------------------------------
// FailingS3 — always returns errors, for error-handling tests
// ---------------------------------------------------------------------------

struct FailingS3;

impl S3Storage for FailingS3 {
    async fn upload_file(&self, _: &str, _: &str, _: &Path) -> anyhow::Result<()> {
        anyhow::bail!("mock failure: upload_file")
    }
    async fn upload_bytes(&self, _: &str, _: &str, _: Vec<u8>) -> anyhow::Result<()> {
        anyhow::bail!("mock failure: upload_bytes")
    }
    async fn download_file(&self, _: &str, _: &str, _: &Path) -> anyhow::Result<()> {
        anyhow::bail!("mock failure: download_file")
    }
    async fn list_transcripts(&self, _: &str) -> anyhow::Result<Vec<u32>> {
        anyhow::bail!("mock failure: list_transcripts")
    }
    async fn upload_meta(&self, _: &str, _: &SessionMeta) -> anyhow::Result<()> {
        anyhow::bail!("mock failure: upload_meta")
    }
    async fn download_meta(&self, _: &str) -> anyhow::Result<Option<SessionMeta>> {
        anyhow::bail!("mock failure: download_meta")
    }
    async fn exists(&self, _: &str, _: &str) -> bool {
        false
    }
    async fn list_sessions(&self) -> anyhow::Result<Vec<String>> {
        anyhow::bail!("mock failure: list_sessions")
    }
    async fn list_recording_chunks(&self, _: &str) -> anyhow::Result<Vec<String>> {
        anyhow::bail!("mock failure: list_recording_chunks")
    }
    async fn download_bytes(&self, _: &str, _: &str) -> anyhow::Result<Vec<u8>> {
        anyhow::bail!("mock failure: download_bytes")
    }
}

// ---------------------------------------------------------------------------
// Helper: build a test RecordingEntry
// ---------------------------------------------------------------------------

fn test_recording_entry(seq: u64) -> RecordingEntry {
    use crate::screen::{CursorPosition, ScreenSnapshot};
    RecordingEntry {
        ts: 1700000000 + seq,
        seq,
        kind: "state".into(),
        detail: serde_json::json!({"prev": "Starting", "next": "Working"}),
        screen: ScreenSnapshot {
            lines: vec!["hello".into()],
            ansi: vec!["hello".into()],
            cols: 80,
            rows: 24,
            alt_screen: false,
            cursor: CursorPosition { row: 0, col: 5 },
            sequence: seq,
        },
    }
}

// ===========================================================================
// Unit tests — pure logic, no I/O
// ===========================================================================

#[test]
fn session_meta_serializes() -> anyhow::Result<()> {
    let meta = SessionMeta {
        session_id: "test-123".into(),
        agent_type: "claude".into(),
        started_at: 1700000000,
        ended_at: Some(1700003600),
        exit_code: Some(0),
        labels: vec!["project:gasboat".into()],
    };
    let json = serde_json::to_string(&meta)?;
    let parsed: SessionMeta = serde_json::from_str(&json)?;
    assert_eq!(parsed.session_id, "test-123");
    assert_eq!(parsed.exit_code, Some(0));
    assert_eq!(parsed.labels.len(), 1);
    Ok(())
}

#[test]
fn session_meta_skips_none_fields() -> anyhow::Result<()> {
    let meta = SessionMeta {
        session_id: "test-456".into(),
        agent_type: "claude".into(),
        started_at: 1700000000,
        ended_at: None,
        exit_code: None,
        labels: vec![],
    };
    let json = serde_json::to_string(&meta)?;
    assert!(!json.contains("ended_at"));
    assert!(!json.contains("exit_code"));
    Ok(())
}

#[test]
fn s3_key_format() {
    // Verify the key format logic without needing a real S3 client.
    let prefix = "coop/sessions";
    let session_id = "abc-123";
    let path = "transcripts/1.jsonl";
    let key = format!("{prefix}/{session_id}/{path}");
    assert_eq!(key, "coop/sessions/abc-123/transcripts/1.jsonl");
}

#[test]
fn session_meta_default_labels() -> anyhow::Result<()> {
    let json = r#"{"session_id":"x","agent_type":"claude","started_at":0}"#;
    let meta: SessionMeta = serde_json::from_str(json)?;
    assert!(meta.labels.is_empty());
    assert!(meta.ended_at.is_none());
    assert!(meta.exit_code.is_none());
    Ok(())
}

#[test]
fn recording_chunk_name_format() {
    let first_seq = 10u64;
    let last_seq = 59u64;
    let name = format!("recording/{first_seq}-{last_seq}.jsonl");
    assert_eq!(name, "recording/10-59.jsonl");
}

#[test]
fn recording_entry_serializes_to_jsonl() -> anyhow::Result<()> {
    let entry = test_recording_entry(1);
    let line = serde_json::to_string(&entry)?;
    assert!(line.contains("\"seq\":1"));
    assert!(line.contains("\"kind\":\"state\""));
    // Verify it's valid JSON
    let _: serde_json::Value = serde_json::from_str(&line)?;
    Ok(())
}

// ===========================================================================
// MockS3 basic operations
// ===========================================================================

#[tokio::test]
async fn mock_upload_download_bytes_roundtrip() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let data = b"hello world".to_vec();
    mock.upload_bytes("sess-1", "test.txt", data.clone()).await?;

    assert!(mock.exists("sess-1", "test.txt").await);
    assert!(!mock.exists("sess-1", "other.txt").await);

    let stored = mock.get("sess-1", "test.txt");
    assert_eq!(stored, Some(data));
    Ok(())
}

#[tokio::test]
async fn mock_upload_download_file_roundtrip() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = MockS3::new();

    // Write a local file and upload it.
    let src = tmp.path().join("source.txt");
    tokio::fs::write(&src, b"file content").await?;
    mock.upload_file("sess-1", "uploaded.txt", &src).await?;

    // Download to a different path.
    let dest = tmp.path().join("downloaded.txt");
    mock.download_file("sess-1", "uploaded.txt", &dest).await?;

    let content = tokio::fs::read_to_string(&dest).await?;
    assert_eq!(content, "file content");
    Ok(())
}

#[tokio::test]
async fn mock_download_missing_object_errors() {
    let tmp = tempfile::tempdir().ok();
    let Some(tmp) = tmp else { return };
    let mock = MockS3::new();
    let dest = tmp.path().join("missing.txt");
    let result = mock.download_file("sess-1", "nope.txt", &dest).await;
    assert!(result.is_err());
}

#[tokio::test]
async fn mock_list_transcripts() -> anyhow::Result<()> {
    let mock = MockS3::new();

    // No transcripts initially.
    let nums = mock.list_transcripts("sess-1").await?;
    assert!(nums.is_empty());

    // Add some transcripts.
    mock.put("sess-1", "transcripts/1.jsonl", b"t1".to_vec());
    mock.put("sess-1", "transcripts/3.jsonl", b"t3".to_vec());
    mock.put("sess-1", "transcripts/2.jsonl", b"t2".to_vec());
    // Non-transcript files should be ignored.
    mock.put("sess-1", "meta.json", b"{}".to_vec());
    mock.put("sess-1", "transcripts/readme.txt", b"x".to_vec());

    let nums = mock.list_transcripts("sess-1").await?;
    assert_eq!(nums, vec![1, 2, 3]);

    // Different session should be isolated.
    let nums = mock.list_transcripts("sess-2").await?;
    assert!(nums.is_empty());
    Ok(())
}

#[tokio::test]
async fn mock_upload_download_meta() -> anyhow::Result<()> {
    let mock = MockS3::new();

    // No meta initially.
    let meta = mock.download_meta("sess-1").await?;
    assert!(meta.is_none());

    // Upload meta.
    let meta = SessionMeta {
        session_id: "sess-1".into(),
        agent_type: "claude".into(),
        started_at: 1700000000,
        ended_at: None,
        exit_code: None,
        labels: vec!["project:test".into()],
    };
    mock.upload_meta("sess-1", &meta).await?;

    // Download and verify.
    let fetched = mock.download_meta("sess-1").await?;
    let fetched = fetched.ok_or_else(|| anyhow::anyhow!("expected meta"))?;
    assert_eq!(fetched.session_id, "sess-1");
    assert_eq!(fetched.agent_type, "claude");
    assert_eq!(fetched.labels, vec!["project:test"]);
    Ok(())
}

// ===========================================================================
// restore_transcripts tests
// ===========================================================================

#[tokio::test]
async fn restore_transcripts_downloads_all() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = Arc::new(MockS3::new());

    // Pre-load transcripts.
    mock.put("sess-1", "transcripts/1.jsonl", b"line1\nline2\n".to_vec());
    mock.put("sess-1", "transcripts/2.jsonl", b"line3\n".to_vec());
    mock.put("sess-1", "transcripts/5.jsonl", b"line4\n".to_vec());

    let dir = tmp.path().join("transcripts");
    let count = restore_transcripts(mock.as_ref(), "sess-1", &dir).await?;

    assert_eq!(count, 3);
    assert_eq!(tokio::fs::read_to_string(dir.join("1.jsonl")).await?, "line1\nline2\n");
    assert_eq!(tokio::fs::read_to_string(dir.join("2.jsonl")).await?, "line3\n");
    assert_eq!(tokio::fs::read_to_string(dir.join("5.jsonl")).await?, "line4\n");
    Ok(())
}

#[tokio::test]
async fn restore_transcripts_skips_existing() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = Arc::new(MockS3::new());

    mock.put("sess-1", "transcripts/1.jsonl", b"from-s3\n".to_vec());
    mock.put("sess-1", "transcripts/2.jsonl", b"from-s3\n".to_vec());

    let dir = tmp.path().join("transcripts");
    tokio::fs::create_dir_all(&dir).await?;
    // Pre-create transcript 1 locally.
    tokio::fs::write(dir.join("1.jsonl"), b"local-data\n").await?;

    let count = restore_transcripts(mock.as_ref(), "sess-1", &dir).await?;
    assert_eq!(count, 2); // Both counted (1 skipped, 1 downloaded).

    // Local file should NOT be overwritten.
    assert_eq!(tokio::fs::read_to_string(dir.join("1.jsonl")).await?, "local-data\n");
    // S3 file should be downloaded.
    assert_eq!(tokio::fs::read_to_string(dir.join("2.jsonl")).await?, "from-s3\n");
    Ok(())
}

#[tokio::test]
async fn restore_transcripts_empty_session() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = Arc::new(MockS3::new());

    let dir = tmp.path().join("transcripts");
    let count = restore_transcripts(mock.as_ref(), "no-such-session", &dir).await?;
    assert_eq!(count, 0);
    // Directory should still be created.
    assert!(dir.exists());
    Ok(())
}

#[tokio::test]
async fn restore_transcripts_creates_parent_dirs() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = Arc::new(MockS3::new());
    mock.put("s", "transcripts/1.jsonl", b"data".to_vec());

    let dir = tmp.path().join("deep").join("nested").join("transcripts");
    let count = restore_transcripts(mock.as_ref(), "s", &dir).await?;
    assert_eq!(count, 1);
    assert!(dir.join("1.jsonl").exists());
    Ok(())
}

// ===========================================================================
// restore_session_log tests
// ===========================================================================

#[tokio::test]
async fn restore_session_log_downloads() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = Arc::new(MockS3::new());
    mock.put("sess-1", "session.jsonl", b"{\"ts\":1}\n{\"ts\":2}\n".to_vec());

    let dest = tmp.path().join("restored-session.jsonl");
    restore_session_log(mock.as_ref(), "sess-1", &dest).await?;

    let content = tokio::fs::read_to_string(&dest).await?;
    assert_eq!(content, "{\"ts\":1}\n{\"ts\":2}\n");
    Ok(())
}

#[tokio::test]
async fn restore_session_log_creates_parent_dirs() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = Arc::new(MockS3::new());
    mock.put("s", "session.jsonl", b"log-data".to_vec());

    let dest = tmp.path().join("a").join("b").join("session.jsonl");
    restore_session_log(mock.as_ref(), "s", &dest).await?;

    assert_eq!(tokio::fs::read_to_string(&dest).await?, "log-data");
    Ok(())
}

#[tokio::test]
async fn restore_session_log_missing_errors() {
    let tmp = tempfile::tempdir().ok();
    let Some(tmp) = tmp else { return };
    let mock = Arc::new(MockS3::new());
    let dest = tmp.path().join("session.jsonl");
    let result = restore_session_log(mock.as_ref(), "no-session", &dest).await;
    assert!(result.is_err());
}

// ===========================================================================
// spawn_subscriber tests
// ===========================================================================

#[tokio::test]
async fn subscriber_uploads_metadata_on_start() -> anyhow::Result<()> {
    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "test-session".into(),
        None,
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec!["project:gasboat".into()],
        shutdown.clone(),
    );

    // Wait for the metadata upload task.
    tokio::time::sleep(Duration::from_millis(100)).await;
    shutdown.cancel();

    let meta = mock.download_meta("test-session").await?;
    let meta = meta.ok_or_else(|| anyhow::anyhow!("expected metadata"))?;
    assert_eq!(meta.session_id, "test-session");
    assert_eq!(meta.agent_type, "claude");
    assert!(meta.ended_at.is_none());
    assert_eq!(meta.labels, vec!["project:gasboat"]);
    Ok(())
}

#[tokio::test]
async fn subscriber_uploads_transcript_on_event() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let session_dir = tmp.path().to_path_buf();
    let transcripts_dir = session_dir.join("transcripts");
    tokio::fs::create_dir_all(&transcripts_dir).await?;

    // Write a transcript file that the subscriber will upload.
    tokio::fs::write(transcripts_dir.join("1.jsonl"), b"transcript-data\n").await?;

    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-t".into(),
        Some(session_dir),
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    // Wait for metadata upload.
    tokio::time::sleep(Duration::from_millis(50)).await;

    // Send a transcript event.
    let _ = transcript_tx.send(TranscriptEvent {
        number: 1,
        timestamp: "2026-01-01T00:00:00Z".into(),
        line_count: 10,
        seq: 1,
    });

    // Wait for the upload.
    tokio::time::sleep(Duration::from_millis(100)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(50)).await;

    // Verify transcript was uploaded.
    let data = mock.get("sess-t", "transcripts/1.jsonl");
    assert_eq!(data, Some(b"transcript-data\n".to_vec()));
    Ok(())
}

#[tokio::test]
async fn subscriber_batches_recording_entries() -> anyhow::Result<()> {
    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(256);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-r".into(),
        None,
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60), // Long interval so periodic doesn't trigger.
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;

    // Send 50 recording entries to trigger a batch flush (batch_size = 50).
    for i in 1..=50 {
        let _ = record_tx.send(test_recording_entry(i));
    }

    // Wait for processing.
    tokio::time::sleep(Duration::from_millis(200)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(50)).await;

    // Should have a recording chunk named "recording/1-50.jsonl".
    let data = mock.get("sess-r", "recording/1-50.jsonl");
    assert!(data.is_some(), "expected recording chunk 1-50");
    let data = data.ok_or_else(|| anyhow::anyhow!("missing chunk"))?;
    let lines: Vec<&str> = std::str::from_utf8(&data)?.lines().collect();
    assert_eq!(lines.len(), 50);
    Ok(())
}

#[tokio::test]
async fn subscriber_flushes_remaining_on_shutdown() -> anyhow::Result<()> {
    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(256);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-flush".into(),
        None,
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;

    // Send fewer than batch_size (50) entries — they should NOT be flushed yet.
    for i in 1..=5 {
        let _ = record_tx.send(test_recording_entry(i));
    }
    tokio::time::sleep(Duration::from_millis(100)).await;

    // Verify nothing flushed yet (except metadata).
    assert!(mock.get("sess-flush", "recording/1-5.jsonl").is_none());

    // Trigger shutdown — should flush remaining.
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(100)).await;

    let data = mock.get("sess-flush", "recording/1-5.jsonl");
    assert!(data.is_some(), "expected recording chunk flushed on shutdown");
    let data = data.ok_or_else(|| anyhow::anyhow!("missing chunk"))?;
    let lines: Vec<&str> = std::str::from_utf8(&data)?.lines().collect();
    assert_eq!(lines.len(), 5);
    Ok(())
}

#[tokio::test]
async fn subscriber_uploads_session_log_with_transcript() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let session_dir = tmp.path().to_path_buf();
    let transcripts_dir = session_dir.join("transcripts");
    tokio::fs::create_dir_all(&transcripts_dir).await?;

    // Write session log and transcript files.
    let session_log = session_dir.join("session.jsonl");
    tokio::fs::write(&session_log, b"session-log-data\n").await?;
    tokio::fs::write(transcripts_dir.join("1.jsonl"), b"t1\n").await?;

    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-log".into(),
        Some(session_dir),
        &transcript_tx,
        &record_tx,
        Some(session_log),
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;

    // Send transcript event — should also trigger session log upload.
    let _ = transcript_tx.send(TranscriptEvent {
        number: 1,
        timestamp: "2026-01-01T00:00:00Z".into(),
        line_count: 5,
        seq: 1,
    });

    tokio::time::sleep(Duration::from_millis(200)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(50)).await;

    // Session log should have been uploaded.
    let log_data = mock.get("sess-log", "session.jsonl");
    assert_eq!(log_data, Some(b"session-log-data\n".to_vec()));
    Ok(())
}

#[tokio::test]
async fn subscriber_uploads_event_logs_on_shutdown() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let session_dir = tmp.path().to_path_buf();

    // Write event log files.
    tokio::fs::write(session_dir.join("state_events.jsonl"), b"state-events\n").await?;
    tokio::fs::write(session_dir.join("hook_events.jsonl"), b"hook-events\n").await?;

    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-ev".into(),
        Some(session_dir),
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(200)).await;

    assert_eq!(mock.get("sess-ev", "state_events.jsonl"), Some(b"state-events\n".to_vec()));
    assert_eq!(mock.get("sess-ev", "hook_events.jsonl"), Some(b"hook-events\n".to_vec()));
    Ok(())
}

// ===========================================================================
// Edge cases and error scenarios
// ===========================================================================

#[tokio::test]
async fn subscriber_handles_no_session_dir() -> anyhow::Result<()> {
    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    // No session_dir, no session_log_path.
    spawn_subscriber(
        Arc::clone(&mock),
        "sess-none".into(),
        None,
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(100)).await;

    // Only metadata should be uploaded.
    let meta = mock.download_meta("sess-none").await?;
    assert!(meta.is_some());
    // No event logs or session log.
    assert!(mock.get("sess-none", "state_events.jsonl").is_none());
    assert!(mock.get("sess-none", "session.jsonl").is_none());
    Ok(())
}

#[tokio::test]
async fn subscriber_survives_s3_upload_errors() -> anyhow::Result<()> {
    // FailingS3 returns errors for all operations, but the subscriber should
    // not crash — it logs warnings and continues.
    let mock = Arc::new(FailingS3);
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-fail".into(),
        None,
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;

    // Send events — should not crash despite S3 errors.
    for i in 1..=3 {
        let _ = record_tx.send(test_recording_entry(i));
    }
    tokio::time::sleep(Duration::from_millis(100)).await;

    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(100)).await;

    // Test passes if we get here without panic.
    Ok(())
}

#[tokio::test]
async fn restore_transcripts_with_failing_s3() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = FailingS3;
    let dir = tmp.path().join("transcripts");

    let result = restore_transcripts(&mock, "sess-1", &dir).await;
    // list_transcripts fails, so the whole function fails.
    assert!(result.is_err());
    Ok(())
}

#[tokio::test]
async fn flush_recording_buffer_empty_is_noop() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let initial_count = mock.object_count();
    let mut buffer = Vec::new();

    flush_recording_buffer(&mock, "sess-1", &mut buffer).await;

    // No new objects should have been created.
    assert_eq!(mock.object_count(), initial_count);
    Ok(())
}

#[tokio::test]
async fn flush_recording_buffer_builds_correct_jsonl() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let mut buffer =
        vec![test_recording_entry(10), test_recording_entry(11), test_recording_entry(12)];

    flush_recording_buffer(&mock, "sess-1", &mut buffer).await;

    // Buffer should be cleared.
    assert!(buffer.is_empty());

    // Check the uploaded chunk.
    let data = mock.get("sess-1", "recording/10-12.jsonl");
    assert!(data.is_some());
    let data = data.ok_or_else(|| anyhow::anyhow!("missing chunk"))?;
    let content = std::str::from_utf8(&data)?;
    let lines: Vec<&str> = content.lines().collect();
    assert_eq!(lines.len(), 3);

    // Each line should be valid JSON.
    for line in &lines {
        let entry: RecordingEntry = serde_json::from_str(line)?;
        assert_eq!(entry.kind, "state");
    }
    Ok(())
}

#[tokio::test]
async fn upload_session_log_missing_path_is_noop() -> anyhow::Result<()> {
    let mock = MockS3::new();
    // None path — should return immediately.
    upload_session_log(&mock, "sess-1", None).await;
    assert_eq!(mock.object_count(), 0);
    Ok(())
}

#[tokio::test]
async fn upload_session_log_missing_file_is_noop() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let nonexistent = Path::new("/tmp/coop-test-does-not-exist-session.jsonl");
    upload_session_log(&mock, "sess-1", Some(nonexistent)).await;
    assert_eq!(mock.object_count(), 0);
    Ok(())
}

#[tokio::test]
async fn upload_event_logs_missing_dir_is_noop() -> anyhow::Result<()> {
    let mock = MockS3::new();
    // None session_dir — should return immediately.
    upload_event_logs(&mock, "sess-1", None).await;
    assert_eq!(mock.object_count(), 0);
    Ok(())
}

#[tokio::test]
async fn upload_event_logs_partial_files() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let mock = MockS3::new();

    // Only create state_events, not hook_events.
    tokio::fs::write(tmp.path().join("state_events.jsonl"), b"events\n").await?;

    upload_event_logs(&mock, "sess-1", Some(tmp.path())).await;

    assert!(mock.get("sess-1", "state_events.jsonl").is_some());
    assert!(mock.get("sess-1", "hook_events.jsonl").is_none());
    Ok(())
}

#[tokio::test]
async fn subscriber_periodic_upload_triggers() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let session_dir = tmp.path().to_path_buf();
    let session_log = session_dir.join("session.jsonl");
    tokio::fs::write(&session_log, b"periodic-test\n").await?;

    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    // Short upload interval to trigger periodic upload quickly.
    spawn_subscriber(
        Arc::clone(&mock),
        "sess-periodic".into(),
        Some(session_dir),
        &transcript_tx,
        &record_tx,
        Some(session_log),
        Duration::from_millis(50), // 50ms interval
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    // Wait longer than the interval.
    tokio::time::sleep(Duration::from_millis(300)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(50)).await;

    // Session log should have been uploaded by the periodic timer.
    let log_data = mock.get("sess-periodic", "session.jsonl");
    assert_eq!(log_data, Some(b"periodic-test\n".to_vec()));
    Ok(())
}

#[tokio::test]
async fn subscriber_multiple_transcript_events() -> anyhow::Result<()> {
    let tmp = tempfile::tempdir()?;
    let session_dir = tmp.path().to_path_buf();
    let transcripts_dir = session_dir.join("transcripts");
    tokio::fs::create_dir_all(&transcripts_dir).await?;

    // Write multiple transcript files.
    tokio::fs::write(transcripts_dir.join("1.jsonl"), b"t1\n").await?;
    tokio::fs::write(transcripts_dir.join("2.jsonl"), b"t2\n").await?;
    tokio::fs::write(transcripts_dir.join("3.jsonl"), b"t3\n").await?;

    let mock = Arc::new(MockS3::new());
    let (transcript_tx, _) = broadcast::channel::<TranscriptEvent>(16);
    let (record_tx, _) = broadcast::channel::<RecordingEntry>(16);
    let shutdown = CancellationToken::new();

    spawn_subscriber(
        Arc::clone(&mock),
        "sess-multi".into(),
        Some(session_dir),
        &transcript_tx,
        &record_tx,
        None,
        Duration::from_secs(60),
        "claude".into(),
        vec![],
        shutdown.clone(),
    );

    tokio::time::sleep(Duration::from_millis(50)).await;

    // Send transcript events sequentially.
    for i in 1..=3 {
        let _ = transcript_tx.send(TranscriptEvent {
            number: i,
            timestamp: format!("2026-01-01T00:00:0{i}Z"),
            line_count: (i * 10).into(),
            seq: i.into(),
        });
        tokio::time::sleep(Duration::from_millis(50)).await;
    }

    tokio::time::sleep(Duration::from_millis(100)).await;
    shutdown.cancel();
    tokio::time::sleep(Duration::from_millis(50)).await;

    assert_eq!(mock.get("sess-multi", "transcripts/1.jsonl"), Some(b"t1\n".to_vec()));
    assert_eq!(mock.get("sess-multi", "transcripts/2.jsonl"), Some(b"t2\n".to_vec()));
    assert_eq!(mock.get("sess-multi", "transcripts/3.jsonl"), Some(b"t3\n".to_vec()));
    Ok(())
}

// ---------------------------------------------------------------------------
// Phase 5 — list_sessions, list_recording_chunks, download_bytes
// ---------------------------------------------------------------------------

#[tokio::test]
async fn mock_list_sessions_empty() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let sessions = mock.list_sessions().await?;
    assert!(sessions.is_empty());
    Ok(())
}

#[tokio::test]
async fn mock_list_sessions_multiple() -> anyhow::Result<()> {
    let mock = MockS3::new();
    mock.put("alpha", "meta.json", b"{}".to_vec());
    mock.put("beta", "transcripts/1.jsonl", b"t1".to_vec());
    mock.put("gamma", "recording/chunk-0000.jsonl", b"r1".to_vec());
    // Same session appears in multiple keys — should deduplicate.
    mock.put("alpha", "transcripts/1.jsonl", b"t1".to_vec());

    let mut sessions = mock.list_sessions().await?;
    sessions.sort();
    assert_eq!(sessions, vec!["alpha", "beta", "gamma"]);
    Ok(())
}

#[tokio::test]
async fn mock_list_recording_chunks_empty() -> anyhow::Result<()> {
    let mock = MockS3::new();
    // Session exists but has no recording/ objects.
    mock.put("sess-1", "meta.json", b"{}".to_vec());
    let chunks = mock.list_recording_chunks("sess-1").await?;
    assert!(chunks.is_empty());
    Ok(())
}

#[tokio::test]
async fn mock_list_recording_chunks_sorted() -> anyhow::Result<()> {
    let mock = MockS3::new();
    mock.put("sess-1", "recording/chunk-0002.jsonl", b"r2".to_vec());
    mock.put("sess-1", "recording/chunk-0000.jsonl", b"r0".to_vec());
    mock.put("sess-1", "recording/chunk-0001.jsonl", b"r1".to_vec());
    // Non-jsonl file under recording/ should be excluded.
    mock.put("sess-1", "recording/index.txt", b"idx".to_vec());
    // Object under a different session should not appear.
    mock.put("sess-2", "recording/chunk-0000.jsonl", b"other".to_vec());

    let chunks = mock.list_recording_chunks("sess-1").await?;
    assert_eq!(chunks, vec!["chunk-0000.jsonl", "chunk-0001.jsonl", "chunk-0002.jsonl"]);
    Ok(())
}

#[tokio::test]
async fn mock_download_bytes_roundtrip() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let payload = b"hello world".to_vec();
    mock.put("sess-1", "artifact.bin", payload.clone());

    let downloaded = mock.download_bytes("sess-1", "artifact.bin").await?;
    assert_eq!(downloaded, payload);
    Ok(())
}

#[tokio::test]
async fn mock_download_bytes_missing_errors() -> anyhow::Result<()> {
    let mock = MockS3::new();
    let result = mock.download_bytes("no-such-session", "missing.bin").await;
    assert!(result.is_err());
    assert!(result.unwrap_err().to_string().contains("object not found"));
    Ok(())
}

#[tokio::test]
async fn failing_s3_list_sessions_errors() -> anyhow::Result<()> {
    let fail = FailingS3;
    let result = fail.list_sessions().await;
    assert!(result.is_err());
    Ok(())
}

#[tokio::test]
async fn failing_s3_list_recording_chunks_errors() -> anyhow::Result<()> {
    let fail = FailingS3;
    let result = fail.list_recording_chunks("any").await;
    assert!(result.is_err());
    Ok(())
}

#[tokio::test]
async fn failing_s3_download_bytes_errors() -> anyhow::Result<()> {
    let fail = FailingS3;
    let result = fail.download_bytes("any", "any.bin").await;
    assert!(result.is_err());
    Ok(())
}
